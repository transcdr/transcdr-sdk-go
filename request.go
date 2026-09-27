package transcdr

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxBackoff = 8 * time.Second

// RequestOption tunes a single call.
type RequestOption func(*requestConfig)

type requestConfig struct {
	query          url.Values
	headers        http.Header
	idempotencyKey string
	maxRetries     *int
	timeout        *time.Duration
	noAuth         bool
}

// withoutAuth sends no Authorization header (for URLs that are credentials).
func withoutAuth() RequestOption { return func(r *requestConfig) { r.noAuth = true } }

// WithIdempotencyKey sends an Idempotency-Key header, which also makes a POST
// safe to retry. Job and upload creation set one automatically.
func WithIdempotencyKey(key string) RequestOption {
	return func(r *requestConfig) { r.idempotencyKey = key }
}

// WithQuery adds a query parameter.
func WithQuery(key, value string) RequestOption {
	return func(r *requestConfig) { r.query.Add(key, value) }
}

// WithRequestHeader adds a header to this request.
func WithRequestHeader(name, value string) RequestOption {
	return func(r *requestConfig) { r.headers.Add(name, value) }
}

// WithRequestMaxRetries overrides the client's retry count for this request.
func WithRequestMaxRetries(n int) RequestOption {
	return func(r *requestConfig) { r.maxRetries = &n }
}

// WithRequestTimeout overrides the client's per-attempt timeout for this
// request; 0 means none.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(r *requestConfig) { r.timeout = &d }
}

// NewIdempotencyKey returns a random key for [WithIdempotencyKey].
func NewIdempotencyKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// Do calls any endpoint: an escape hatch for routes newer than the SDK. path
// is an API path (/v1/…) or an absolute URL; body is encoded as JSON unless
// nil; a 2xx JSON response is decoded into out unless out is nil.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...RequestOption) error {
	return c.do(ctx, method, path, nil, body, out, opts)
}

// url resolves an API path or absolute URL.
func (c *Client) url(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// seg escapes one path segment.
func seg(s string) string { return url.PathEscape(s) }

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any, opts []RequestOption) error {
	cfg := requestConfig{query: url.Values{}, headers: http.Header{}}
	for k, vs := range query {
		for _, v := range vs {
			cfg.query.Add(k, v)
		}
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	target := c.url(path)
	if len(cfg.query) > 0 {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + cfg.query.Encode()
	}

	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("transcdr: encoding the request body: %w", err)
		}
	}

	method = strings.ToUpper(method)
	retries := 0
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		retries = c.maxRetries
	default:
		if cfg.idempotencyKey != "" {
			retries = c.maxRetries
		}
	}
	if cfg.maxRetries != nil {
		retries = *cfg.maxRetries
	}
	timeout := c.timeout
	if cfg.timeout != nil {
		timeout = *cfg.timeout
	}

	for attempt := 0; ; attempt++ {
		resp, data, err := c.send(ctx, method, target, payload, cfg, timeout)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < retries {
				if werr := sleep(ctx, c.backoff(attempt, "")); werr != nil {
					return werr
				}
				continue
			}
			return err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(data)) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return &Error{
					Type: ErrorTypeAPI, Status: resp.StatusCode, Message: "The API returned malformed JSON: " + err.Error(),
					RequestID: resp.Header.Get("X-Request-Id"), Header: resp.Header,
				}
			}
			return nil
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < retries {
			if werr := sleep(ctx, c.backoff(attempt, resp.Header.Get("Retry-After"))); werr != nil {
				return werr
			}
			continue
		}
		return errorFromResponse(resp.StatusCode, data, resp.Header)
	}
}

// send makes one attempt.
func (c *Client) send(ctx context.Context, method, target string, payload []byte, cfg requestConfig, timeout time.Duration) (*http.Response, []byte, error) {
	attemptCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(attemptCtx, method, target, reader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	for k, vs := range c.headers {
		req.Header[k] = append([]string(nil), vs...)
	}
	for k, vs := range cfg.headers {
		req.Header[k] = append([]string(nil), vs...)
	}
	if key := c.APIKey(); key != "" && !cfg.noAuth {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if cfg.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", cfg.idempotencyKey)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		timedOut := errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
		msg := "Could not reach the Transcdr API: " + err.Error()
		if timedOut {
			msg = fmt.Sprintf("Request timed out after %s.", timeout)
		}
		return nil, nil, &ConnectionError{Message: msg, Timeout: timedOut, Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &ConnectionError{Message: "Reading the response failed: " + err.Error(), Err: err}
	}
	return resp, data, nil
}

// backoff is the delay before retry attempt+1: Retry-After when the API sent
// one (capped at 60 s), else exponential with jitter in [d/2, d].
func (c *Client) backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil && seconds >= 0 {
			return min(time.Duration(seconds*float64(time.Second)), c.maxRetryWait)
		}
		if at, err := http.ParseTime(retryAfter); err == nil {
			return max(0, min(time.Until(at), c.maxRetryWait))
		}
	}
	d := time.Duration(math.Min(float64(c.retryDelay)*math.Pow(2, float64(attempt)), float64(maxBackoff)))
	if d <= 0 {
		return 0
	}
	return d/2 + time.Duration(mrand.Int64N(int64(d/2)+1))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
