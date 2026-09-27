package transcdr

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClientDefaultsFromEnvironment(t *testing.T) {
	f := newFakeAPI(t)
	t.Setenv("TRANSCDR_API_KEY", "tdk_test_from_env")
	t.Setenv("TRANSCDR_BASE_URL", f.server.URL+"/")
	c := NewClient()
	if c.BaseURL() != f.server.URL {
		t.Fatalf("BaseURL = %q", c.BaseURL())
	}
	if c.APIKey() != "tdk_test_from_env" {
		t.Fatalf("APIKey = %q", c.APIKey())
	}
	must(t, c.Do(context.Background(), "GET", "/v1/status", nil, nil))
	if got := f.last().Header.Get("Authorization"); got != "Bearer tdk_test_from_env" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestClientOptionsWin(t *testing.T) {
	f := newFakeAPI(t)
	t.Setenv("TRANSCDR_API_KEY", "tdk_test_from_env")
	t.Setenv("TRANSCDR_BASE_URL", "http://unused.invalid")
	c := NewClient(WithAPIKey("tdk_test_example"), WithBaseURL(f.server.URL))
	if c.BaseURL() != f.server.URL || c.APIKey() != "tdk_test_example" {
		t.Fatalf("options did not win: %q %q", c.BaseURL(), c.APIKey())
	}
}

func TestDefaultBaseURL(t *testing.T) {
	t.Setenv("TRANSCDR_BASE_URL", "")
	if got := NewClient().BaseURL(); got != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", got)
	}
}

func TestHeaders(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client(WithHeader("X-Extra", "1"), WithUserAgent("my-app/1.2"))
	must(t, c.Do(context.Background(), "POST", "/v1/things", map[string]int{"a": 1}, nil, WithRequestHeader("X-Per-Request", "yes")))
	r := f.last()
	checks := map[string]string{
		"Authorization": "Bearer tdk_test_example",
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"X-Extra":       "1",
		"X-Per-Request": "yes",
		"User-Agent":    "my-app/1.2 transcdr-sdk-go/" + Version,
	}
	for k, want := range checks {
		if got := r.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if string(r.Body) != `{"a":1}` {
		t.Errorf("body = %s", r.Body)
	}
}

func TestNoBodyNoContentType(t *testing.T) {
	f := newFakeAPI(t)
	must(t, f.client().Do(context.Background(), "GET", "/v1/status", nil, nil))
	r := f.last()
	if r.Header.Get("Content-Type") != "" || len(r.Body) != 0 {
		t.Fatalf("GET sent a body: %q %q", r.Header.Get("Content-Type"), r.Body)
	}
	if !strings.HasPrefix(r.Header.Get("User-Agent"), "transcdr-sdk-go/") {
		t.Fatalf("User-Agent = %q", r.Header.Get("User-Agent"))
	}
}

func TestSetAPIKey(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	c.SetAPIKey("tds_session_example")
	must(t, c.Do(context.Background(), "GET", "/v1/me", nil, nil))
	if got := f.last().Header.Get("Authorization"); got != "Bearer tds_session_example" {
		t.Fatalf("Authorization = %q", got)
	}
	c.SetAPIKey("")
	must(t, c.Do(context.Background(), "GET", "/v1/status", nil, nil))
	if got := f.last().Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization after clearing = %q", got)
	}
}

func TestDoEscapeHatch(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"id":"x_1","count":3}`)
	var out struct {
		ID    string `json:"id"`
		Count int    `json:"count"`
	}
	must(t, f.client().Do(context.Background(), "patch", "v1/new-thing", map[string]string{"k": "v"}, &out, WithQuery("a", "b")))
	r := f.expect("PATCH", "/v1/new-thing")
	if qv(r.Query, "a") != "b" || out.ID != "x_1" || out.Count != 3 {
		t.Fatalf("query %v, out %+v", r.Query, out)
	}
	// An absolute URL is used as is.
	must(t, f.client().Do(context.Background(), "GET", f.server.URL+"/elsewhere?x=1", nil, nil, WithQuery("y", "2")))
	r = f.expect("GET", "/elsewhere")
	if qv(r.Query, "x") != "1" || qv(r.Query, "y") != "2" {
		t.Fatalf("query = %v", r.Query)
	}
}

func TestNoContentAndEmptyBodies(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(204, "")
	var out map[string]any
	must(t, f.client().Do(context.Background(), "DELETE", "/v1/jobs/job_1", nil, &out))
	if out != nil {
		t.Fatalf("out = %v", out)
	}
}

func TestMalformedJSON(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"id":`, "X-Request-Id", "req_bad")
	var out map[string]any
	err := f.client().Do(context.Background(), "GET", "/v1/jobs/job_1", nil, &out)
	e, ok := AsError(err)
	if !ok || e.Type != ErrorTypeAPI || e.Status != 200 || e.RequestID != "req_bad" {
		t.Fatalf("err = %#v", err)
	}
}

func TestServicesAreWired(t *testing.T) {
	c := NewClient()
	if c.Auth == nil || c.Organization == nil || c.Organization.Members == nil || c.Organizations == nil || c.APIKeys == nil ||
		c.Uploads == nil || c.Assets == nil || c.Jobs == nil || c.Probe == nil || c.Presets == nil || c.Webhooks == nil ||
		c.Events == nil || c.Usage == nil || c.Billing == nil || c.Billing.Invoices == nil || c.Plans == nil ||
		c.Capabilities == nil || c.Status == nil || c.Stats == nil || c.Connections == nil || c.Automations == nil ||
		c.Deliveries == nil || c.Announcements == nil || c.Changelog == nil || c.Admin == nil ||
		c.Admin.Announcements == nil || c.Admin.Incidents == nil {
		t.Fatal("a service is nil")
	}
}

func TestNewIdempotencyKeyShape(t *testing.T) {
	a, b := NewIdempotencyKey(), NewIdempotencyKey()
	if !uuidShape(a) || a == b {
		t.Fatalf("keys %q %q", a, b)
	}
}

func uuidShape(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	for i, n := range []int{8, 4, 4, 4, 12} {
		if len(parts[i]) != n {
			return false
		}
		for _, r := range parts[i] {
			if !strings.ContainsRune("0123456789abcdef", r) {
				return false
			}
		}
	}
	return true
}

func TestNullable(t *testing.T) {
	type body struct {
		A Nullable[int64]  `json:"a,omitzero"`
		B Nullable[string] `json:"b,omitzero"`
		C Nullable[bool]   `json:"c,omitzero"`
	}
	got, err := json.Marshal(body{A: Null[int64](), B: Value("x")})
	must(t, err)
	if string(got) != `{"a":null,"b":"x"}` {
		t.Fatalf("marshal = %s", got)
	}
	var back body
	must(t, json.Unmarshal([]byte(`{"a":null,"b":"y"}`), &back))
	if !back.A.IsNull() || back.A.IsZero() {
		t.Fatal("a should be null")
	}
	if v, ok := back.B.Get(); !ok || v != "y" {
		t.Fatalf("b = %q %v", v, ok)
	}
	if !back.C.IsZero() || back.C.IsNull() {
		t.Fatal("c should be unset")
	}
	if _, ok := back.C.Get(); ok {
		t.Fatal("unset Get reported a value")
	}
	if String("s") == nil || *Int(2) != 2 || *Int64(3) != 3 || *Float64(1.5) != 1.5 || !*Bool(true) {
		t.Fatal("pointer helpers")
	}
}

func TestOutputSpecInput(t *testing.T) {
	// Fields: only those set are sent; Nullable fields can be null.
	spec := OutputSpecInput{Codec: "h264", Quality: &Quality{Target: String(QualityCBR), Bitrate: String("5M")}, Ladder: Null[Ladder](), Gop: Value(48)}
	got, err := json.Marshal(spec)
	must(t, err)
	var m map[string]any
	must(t, json.Unmarshal(got, &m))
	if m["codec"] != "h264" || m["ladder"] != nil || m["gop"] != float64(48) || len(m) != 4 {
		t.Fatalf("marshal = %s", got)
	}
	if _, ok := m["ladder"]; !ok {
		t.Fatal("ladder null was dropped")
	}

	// RawOutputSpec is sent verbatim, key order and all.
	raw := `{"zeta":1,"codec":"av1","future_field":{"x":true}}`
	got, err = json.Marshal(RawOutputSpec(json.RawMessage(raw)))
	must(t, err)
	if string(got) != raw {
		t.Fatalf("raw marshal = %s", got)
	}
	if string(RawOutputSpec(json.RawMessage(raw)).Raw()) != raw {
		t.Fatal("Raw of a RawOutputSpec")
	}

	// Decoding keeps the fields and the raw JSON.
	var dec OutputSpecInput
	in := `{"codec":"h265","renditions":[{"width":1280,"height":720}],"segment_seconds":6,"new":1}`
	must(t, json.Unmarshal([]byte(in), &dec))
	if dec.Codec != "h265" || len(dec.Renditions) != 1 || string(dec.Raw()) != in {
		t.Fatalf("decoded %+v raw %s", dec, dec.Raw())
	}
	if v, _ := dec.SegmentSeconds.Get(); v != 6 {
		t.Fatalf("segment_seconds = %v", v)
	}
	// A decoded spec marshals from its fields.
	got, err = json.Marshal(dec)
	must(t, err)
	if strings.Contains(string(got), "new") {
		t.Fatalf("decoded spec re-marshal = %s", got)
	}

	// In a request body, as a pointer.
	f := newFakeAPI(t)
	f.reply(200, `{"id":"job_1"}`)
	_, err = f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://example.com/a.mp4"), Output: RawOutputSpec(json.RawMessage(`{"codec":"h264"}`))})
	must(t, err)
	if !strings.Contains(string(f.last().Body), `"output":{"codec":"h264"}`) {
		t.Fatalf("body = %s", f.last().Body)
	}
}

func TestOutputSpecRaw(t *testing.T) {
	in := `{"mode":"hls","codec":"av1","renditions":[],"ladder":{"max_short_side":1080},"quality":{"target":"high"},"gop":null,"segment_seconds":4.0,"audio":{"mode":"auto"},"subtitles":"all","color":"sdr","bit_depth":"auto","max_fps":null,"filters":null,"trim":null,"future":true}`
	var s OutputSpec
	must(t, json.Unmarshal([]byte(in), &s))
	if s.Mode != "hls" || s.Ladder == nil || *s.Ladder.MaxShortSide != 1080 || *s.Quality.Target != QualityHigh || *s.SegmentSeconds != 4 {
		t.Fatalf("decoded %+v", s)
	}
	if string(s.Raw()) != in {
		t.Fatalf("Raw = %s", s.Raw())
	}
	if QualityVMAF(93) != "vmaf=93" {
		t.Fatal(QualityVMAF(93))
	}
}

func TestHTTPClientOption(t *testing.T) {
	f := newFakeAPI(t)
	var used bool
	hc := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	must(t, f.client(WithHTTPClient(hc)).Do(context.Background(), "GET", "/v1/status", nil, nil))
	if !used {
		t.Fatal("custom HTTP client not used")
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
