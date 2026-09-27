package transcdr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// recorded is one request the fake API received.
type recorded struct {
	Method string
	Path   string // escaped path, e.g. /v1/jobs/job_1
	Query  map[string][]string
	Header http.Header
	Body   []byte
}

// JSON decodes the request body.
func (r recorded) JSON(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("request body is not a JSON object: %s", r.Body)
	}
	return m
}

// fakeAPI answers every request with the next queued response (or 200 {})
// and records what it received.
type fakeAPI struct {
	t         *testing.T
	server    *httptest.Server
	mu        sync.Mutex
	requests  []recorded
	responses []fakeResponse
}

type fakeResponse struct {
	status int
	body   string
	header map[string]string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, recorded{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.Query(), Header: r.Header.Clone(), Body: body})
		resp := fakeResponse{status: 200, body: "{}"}
		if len(f.responses) > 0 {
			resp, f.responses = f.responses[0], f.responses[1:]
		}
		f.mu.Unlock()
		for k, v := range resp.header {
			w.Header().Set(k, v)
		}
		if resp.body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.status)
		_, _ = io.WriteString(w, resp.body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// reply queues a response.
func (f *fakeAPI) reply(status int, body string, header ...string) *fakeAPI {
	h := map[string]string{}
	for i := 0; i+1 < len(header); i += 2 {
		h[header[i]] = header[i+1]
	}
	f.mu.Lock()
	f.responses = append(f.responses, fakeResponse{status: status, body: body, header: h})
	f.mu.Unlock()
	return f
}

// client is a client for the fake API with fast retries.
func (f *fakeAPI) client(opts ...Option) *Client {
	base := []Option{WithBaseURL(f.server.URL), WithAPIKey("tdk_test_example"), WithRetryDelay(time.Millisecond)}
	return NewClient(append(base, opts...)...)
}

// last is the most recent request.
func (f *fakeAPI) last() recorded {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		f.t.Fatal("no request was made")
	}
	return f.requests[len(f.requests)-1]
}

// all returns every request so far.
func (f *fakeAPI) all() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.requests...)
}

// expect checks the most recent request's method and path.
func (f *fakeAPI) expect(method, path string) recorded {
	f.t.Helper()
	r := f.last()
	if r.Method != method || r.Path != path {
		f.t.Fatalf("request = %s %s, want %s %s", r.Method, r.Path, method, path)
	}
	return r
}

// fixture reads testdata/fixtures/<name>.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
