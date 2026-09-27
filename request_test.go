package transcdr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetRetriedOn5xxAnd429(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(503, `{"error":{"type":"api_error","message":"busy"}}`).
		reply(429, `{"error":{"type":"rate_limit_error","message":"slow down"}}`, "Retry-After", "0").
		reply(200, `{"status":"operational","queue_depth":1,"running_jobs":2}`)
	s, err := f.client().Status.Get(context.Background())
	must(t, err)
	if s.QueueDepth != 1 || len(f.all()) != 3 {
		t.Fatalf("status %+v after %d requests", s, len(f.all()))
	}
}

func TestPutAndDeleteRetried(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(500, "").reply(204, "")
	must(t, f.client().Jobs.Delete(context.Background(), "job_1"))
	f.reply(502, "").reply(200, `{}`)
	_, err := f.client().Billing.ChangePlan(context.Background(), "growth")
	must(t, err)
	if n := len(f.all()); n != 4 {
		t.Fatalf("requests = %d", n)
	}
}

func TestPostWithoutKeyNotRetried(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(500, `{"error":{"type":"api_error","message":"boom"}}`)
	_, err := f.client().Presets.Create(context.Background(), &PresetCreateParams{Name: "x", Output: &OutputSpecInput{}})
	if !IsAPIError(err) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.all()); n != 1 {
		t.Fatalf("a POST without an idempotency key was sent %d times", n)
	}
	// PATCH is not retried either.
	f.reply(503, "")
	_, err = f.client().Presets.Update(context.Background(), "pre_1", &PresetUpdateParams{Name: String("y")})
	if err == nil || len(f.all()) != 2 {
		t.Fatalf("PATCH: err %v, requests %d", err, len(f.all()))
	}
}

func TestPostWithKeyRetriedWithSameKey(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(500, "").reply(429, "", "Retry-After", "0").reply(200, `{"id":"job_1","status":"queued"}`)
	job, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://example.com/in.mp4")})
	must(t, err)
	reqs := f.all()
	if job.ID != "job_1" || len(reqs) != 3 {
		t.Fatalf("job %+v after %d requests", job, len(reqs))
	}
	key := reqs[0].Header.Get("Idempotency-Key")
	for _, r := range reqs {
		if got := r.Header.Get("Idempotency-Key"); got != key || key == "" {
			t.Fatalf("keys differ: %q vs %q", got, key)
		}
	}
	// A key passed explicitly makes any POST retryable.
	f2 := newFakeAPI(t)
	f2.reply(503, "").reply(200, `{}`)
	must(t, f2.client().Do(context.Background(), "POST", "/v1/anything", nil, nil, WithIdempotencyKey("abc")))
	if n := len(f2.all()); n != 2 || f2.last().Header.Get("Idempotency-Key") != "abc" {
		t.Fatalf("requests = %d", n)
	}
}

func TestMaxRetries(t *testing.T) {
	f := newFakeAPI(t)
	for i := 0; i < 10; i++ {
		f.reply(500, "")
	}
	err := f.client(WithMaxRetries(3)).Do(context.Background(), "GET", "/v1/x", nil, nil)
	if err == nil || len(f.all()) != 4 {
		t.Fatalf("err %v, requests %d", err, len(f.all()))
	}
	f2 := newFakeAPI(t)
	f2.reply(500, "").reply(500, "")
	err = f2.client().Do(context.Background(), "GET", "/v1/x", nil, nil, WithRequestMaxRetries(0))
	if err == nil || len(f2.all()) != 1 {
		t.Fatalf("per-request override: err %v, requests %d", err, len(f2.all()))
	}
	// Retries are also offered to a POST when asked for per request.
	f3 := newFakeAPI(t)
	f3.reply(500, "").reply(200, "{}")
	must(t, f3.client().Do(context.Background(), "POST", "/v1/x", nil, nil, WithRequestMaxRetries(1)))
	if len(f3.all()) != 2 {
		t.Fatalf("requests = %d", len(f3.all()))
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(422, `{"error":{"type":"invalid_request_error","code":"validation_failed","message":"bad"}}`)
	err := f.client().Do(context.Background(), "GET", "/v1/x", nil, nil)
	if !IsInvalidRequest(err) || len(f.all()) != 1 {
		t.Fatalf("err %v, requests %d", err, len(f.all()))
	}
}

func TestErrorEnvelope(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(422, `{"error":{"type":"invalid_request_error","code":"validation_failed","message":"The output.renditions field is required.",
		"param":"output.renditions","details":{"output.renditions":["The output.renditions field is required."],"name":["Too long."]},"request_id":"3f2a-1c"}}`,
		"X-Request-Id", "header-id")
	err := f.client().Do(context.Background(), "POST", "/v1/jobs", map[string]any{}, nil)
	e, ok := AsError(err)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	if e.Status != 422 || e.Type != ErrorTypeInvalidRequest || e.Code != "validation_failed" || e.Param != "output.renditions" ||
		e.RequestID != "3f2a-1c" || len(e.Details["name"]) != 1 || e.Header.Get("X-Request-Id") != "header-id" {
		t.Fatalf("parsed %+v", e)
	}
	want := "transcdr: 422 validation_failed: The output.renditions field is required. (name: Too long.; output.renditions: The output.renditions field is required.) [request 3f2a-1c]"
	if e.Error() != want {
		t.Fatalf("Error() = %q", e.Error())
	}
	if !IsInvalidRequest(err) || IsNotFound(err) || IsConflict(err) {
		t.Fatal("predicates")
	}
}

func TestErrorRequestIDFromHeader(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(409, `{"error":{"type":"invalid_request_error","code":"connection_in_use","message":"in use"}}`, "X-Request-Id", "rid-9")
	err := f.client().Connections.Delete(context.Background(), "con_1")
	e, _ := AsError(err)
	if !IsConflict(err) || e.RequestID != "rid-9" {
		t.Fatalf("err = %#v", err)
	}
}

func TestErrorWithoutEnvelope(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(404, "not here")
	err := f.client().Do(context.Background(), "GET", "/v1/jobs/job_x", nil, nil)
	e, ok := AsError(err)
	if !ok || !IsNotFound(err) || e.Message != "not here" || e.Type != ErrorTypeInvalidRequest {
		t.Fatalf("err = %#v", err)
	}
	f.reply(418, "")
	e, _ = AsError(f.client().Do(context.Background(), "GET", "/v1/x", nil, nil))
	if e.Message != "Request failed with status 418" {
		t.Fatalf("message = %q", e.Message)
	}
}

func TestDefaultErrorTypes(t *testing.T) {
	cases := []struct {
		status int
		typ    string
		is     func(error) bool
	}{
		{400, ErrorTypeInvalidRequest, IsInvalidRequest},
		{401, ErrorTypeAuthentication, IsAuthentication},
		{402, ErrorTypeQuota, IsQuota},
		{403, ErrorTypePermission, IsPermission},
		{429, ErrorTypeRateLimit, IsRateLimit},
		{500, ErrorTypeAPI, IsAPIError},
	}
	for _, c := range cases {
		f := newFakeAPI(t)
		f.reply(c.status, "")
		err := f.client(WithMaxRetries(0)).Do(context.Background(), "GET", "/v1/x", nil, nil)
		e, ok := AsError(err)
		if !ok || e.Type != c.typ || !c.is(err) {
			t.Errorf("%d: %#v", c.status, err)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(429, `{"error":{"type":"rate_limit_error","message":"slow"}}`, "Retry-After", "7")
	err := f.client(WithMaxRetries(0)).Do(context.Background(), "GET", "/v1/x", nil, nil)
	e, _ := AsError(err)
	d, ok := e.RetryAfter()
	if !ok || d != 7*time.Second {
		t.Fatalf("RetryAfter = %v %v", d, ok)
	}
	if _, ok := (&Error{}).RetryAfter(); ok {
		t.Fatal("no header, no Retry-After")
	}
	date := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	if d, ok := (&Error{Header: http.Header{"Retry-After": {date}}}).RetryAfter(); !ok || d <= 0 {
		t.Fatalf("date Retry-After = %v %v", d, ok)
	}
}

func TestBackoff(t *testing.T) {
	c := NewClient()
	if d := c.backoff(0, "2"); d != 2*time.Second {
		t.Errorf("Retry-After 2 = %v", d)
	}
	if d := c.backoff(0, "3600"); d != 60*time.Second {
		t.Errorf("Retry-After is capped at 60 s: %v", d)
	}
	for attempt := 0; attempt < 8; attempt++ {
		d := c.backoff(attempt, "")
		limit := min(500*time.Millisecond<<attempt, 8*time.Second)
		if d < limit/2 || d > limit {
			t.Errorf("attempt %d: %v outside [%v, %v]", attempt, d, limit/2, limit)
		}
	}
}

func TestConnectionErrorAndTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	c := NewClient(WithBaseURL(slow.URL), WithTimeout(30*time.Millisecond), WithMaxRetries(0))
	err := c.Do(context.Background(), "GET", "/v1/x", nil, nil)
	if !IsTimeout(err) || !IsConnection(err) {
		t.Fatalf("err = %v", err)
	}
	var ce *ConnectionError
	if !errors.As(err, &ce) || ce.Unwrap() == nil {
		t.Fatalf("ConnectionError = %#v", err)
	}

	// A per-request timeout overrides the client's.
	c = NewClient(WithBaseURL(slow.URL), WithMaxRetries(0))
	if err := c.Do(context.Background(), "GET", "/v1/x", nil, nil, WithRequestTimeout(20*time.Millisecond)); !IsTimeout(err) {
		t.Fatalf("per-request timeout: %v", err)
	}

	// Nothing listening.
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	err = NewClient(WithBaseURL(url), WithMaxRetries(1), WithRetryDelay(time.Millisecond)).Do(context.Background(), "GET", "/v1/x", nil, nil)
	if !IsConnection(err) || IsTimeout(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	f := newFakeAPI(t)
	for i := 0; i < 5; i++ {
		f.reply(500, "", "Retry-After", "30")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := f.client().Do(ctx, "GET", "/v1/x", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
