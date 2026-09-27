package transcdr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJobsCreate(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, `{"id":"job_1","status":"queued"}`)
	job, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{
		Input:        URLInput("https://example.com/in.mp4"),
		Preset:       String("hls-av1-abr"),
		Priority:     String(PriorityHigh),
		Metadata:     Metadata{"customer": "acme"},
		WebhookURL:   String("https://example.com/hook"),
		Destination:  &JobDestination{ConnectionID: "con_1", Prefix: "out/{job_id}/"},
		MaxCostCents: Int64(250),
		Output:       &OutputSpecInput{Codec: "h264"},
	})
	must(t, err)
	if job.ID != "job_1" || job.Status != JobQueued {
		t.Fatalf("job = %+v", job)
	}
	r := f.expect("POST", "/v1/jobs")
	b := r.JSON(t)
	in := b["input"].(map[string]any)
	if in["type"] != "url" || in["url"] != "https://example.com/in.mp4" || len(in) != 2 {
		t.Fatalf("input = %v", in)
	}
	if b["preset"] != "hls-av1-abr" || b["priority"] != "high" || b["max_cost_cents"] != float64(250) ||
		b["metadata"].(map[string]any)["customer"] != "acme" || b["webhook_url"] != "https://example.com/hook" ||
		b["destination"].(map[string]any)["prefix"] != "out/{job_id}/" || b["output"].(map[string]any)["codec"] != "h264" {
		t.Fatalf("body = %s", r.Body)
	}
	if key := r.Header.Get("Idempotency-Key"); !uuidShape(key) {
		t.Fatalf("Idempotency-Key = %q", key)
	}

	// Minimal body: nothing optional is sent.
	f.reply(201, `{"id":"job_2"}`)
	_, err = f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: AssetInput("ast_1")})
	must(t, err)
	if string(f.last().Body) != `{"input":{"type":"asset","asset_id":"ast_1"}}` {
		t.Fatalf("minimal body = %s", f.last().Body)
	}
}

func TestJobsCreateIdempotencyKeys(t *testing.T) {
	f := newFakeAPI(t)
	_, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://x")}, WithIdempotencyKey("mine"))
	must(t, err)
	if got := f.last().Header.Get("Idempotency-Key"); got != "mine" {
		t.Fatalf("caller key lost: %q", got)
	}
	// Without retries a key is still sent: a caller may retry by hand.
	_, err = f.client(WithMaxRetries(0)).Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://x")})
	must(t, err)
	if got := f.last().Header.Get("Idempotency-Key"); !uuidShape(got) {
		t.Fatalf("Idempotency-Key = %q", got)
	}
}

func TestJobInputConstructors(t *testing.T) {
	if in := ConnectionInput("con_1", "incoming/a.mov"); in.Type != "connection" || in.ConnectionID != "con_1" || in.Path != "incoming/a.mov" {
		t.Fatalf("%+v", in)
	}
	if in := AssetInput("ast_1"); in.Type != "asset" || in.AssetID != "ast_1" {
		t.Fatalf("%+v", in)
	}
	if in := URLInput("https://x"); in.Type != "url" || in.URL != "https://x" {
		t.Fatalf("%+v", in)
	}
}

func TestJobsList(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, fixture(t, "jobs.json"))
	after := time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("x", 3600))
	before := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	page, err := f.client().Jobs.List(context.Background(), &JobListParams{
		ListParams: ListParams{Limit: 10, Cursor: "job_9"}, Status: JobCompleted, Preset: "hls-av1-abr",
		CreatedAfter: &after, CreatedBefore: &before, Metadata: Metadata{"customer": "acme"},
	})
	must(t, err)
	r := f.expect("GET", "/v1/jobs")
	want := map[string]string{
		"limit": "10", "cursor": "job_9", "status": "completed", "preset": "hls-av1-abr",
		"created_after": "2026-08-31T23:00:00Z", "created_before": "2026-09-30T12:00:00Z", "metadata[customer]": "acme",
	}
	for k, v := range want {
		if got := r.Query[k]; len(got) != 1 || got[0] != v {
			t.Errorf("%s = %v, want %s", k, got, v)
		}
	}
	if len(page.Data) != 10 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page: %d items, more %v, cursor %q", len(page.Data), page.HasMore, page.NextCursor)
	}
	for _, j := range page.Data {
		if j.ID == "" || j.Status == "" || j.CreatedAt.IsZero() {
			t.Fatalf("job decoded badly: %+v", j)
		}
	}

	// No params: no query.
	_, err = f.client().Jobs.List(context.Background(), nil)
	must(t, err)
	if len(f.last().Query) != 0 {
		t.Fatalf("query = %v", f.last().Query)
	}
}

func TestJobsAll(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"job_1"}],"has_more":true,"next_cursor":"job_1"}`).
		reply(200, `{"data":[{"id":"job_2"}],"has_more":false,"next_cursor":null}`)
	jobs, err := Collect(f.client().Jobs.All(context.Background(), &JobListParams{Status: JobFailed}), 0)
	must(t, err)
	reqs := f.all()
	if len(jobs) != 2 || qv(reqs[1].Query, "cursor") != "job_1" || qv(reqs[1].Query, "status") != "failed" {
		t.Fatalf("jobs %v, second query %v", jobs, reqs[1].Query)
	}
}

func TestJobDecodingFromFixture(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, fixture(t, "job.json"))
	job, err := f.client().Jobs.Get(context.Background(), "job_zTyZl9x2xJzbyp8ScuyQRm")
	must(t, err)
	f.expect("GET", "/v1/jobs/job_zTyZl9x2xJzbyp8ScuyQRm")
	if job.ID != "job_zTyZl9x2xJzbyp8ScuyQRm" || job.Status != JobCompleted || job.Kind != "transcode" || !job.Status.IsTerminal() {
		t.Fatalf("job = %+v", job)
	}
	if job.Input.Type != "asset" || job.Input.AssetID != "ast_mS8WEuP2joqZkCGyVc2DYZ" {
		t.Fatalf("input = %+v", job.Input)
	}
	if job.Output.Codec != "av1" || job.Output.Mode != "single" || len(job.Output.Raw()) == 0 {
		t.Fatalf("output = %+v", job.Output)
	}
	if job.InputInfo == nil || job.InputInfo.VideoCodec != "h264" {
		t.Fatalf("input_info = %+v", job.InputInfo)
	}
	if job.Billing == nil || job.Billing.AmountCents != 1 || job.Billing.Tier == nil || *job.Billing.Tier != "hd" ||
		job.Billing.AmountUSD == nil || *job.Billing.AmountUSD != 0.002789 {
		t.Fatalf("billing = %+v", job.Billing)
	}
	if len(job.Outputs) != 1 || job.Progress.Stage != "done" || job.PresetID == nil || *job.PresetID != "archive-av1-high" {
		t.Fatalf("outputs %d, stage %q", len(job.Outputs), job.Progress.Stage)
	}
	if job.PlaybackURL == nil || job.PlaylistURL != nil || job.Livemode == nil || !*job.Livemode || job.CompletedAt == nil || job.MaxAttempts != 3 {
		t.Fatalf("urls/timestamps: %+v", job)
	}
}

func TestJobsActions(t *testing.T) {
	f := newFakeAPI(t)
	ctx := context.Background()
	c := f.client()

	f.reply(200, `{"id":"job_1","status":"canceled"}`)
	job, err := c.Jobs.Cancel(ctx, "job_1")
	must(t, err)
	f.expect("POST", "/v1/jobs/job_1/cancel")
	if job.Status != JobCanceled {
		t.Fatalf("status = %s", job.Status)
	}

	f.reply(200, `{"id":"job_1","status":"queued"}`)
	_, err = c.Jobs.Retry(ctx, "job_1")
	must(t, err)
	f.expect("POST", "/v1/jobs/job_1/retry")

	f.reply(204, "")
	must(t, c.Jobs.Delete(ctx, "job_1"))
	f.expect("DELETE", "/v1/jobs/job_1")

	f.reply(200, fixture(t, "job_events.json"))
	events, err := c.Jobs.Events(ctx, "job_1")
	must(t, err)
	f.expect("GET", "/v1/jobs/job_1/events")
	if len(events) != 6 || events[0].Type != "created" || events[0].Message != "Job created" || events[0].CreatedAt.IsZero() || len(events[0].Data) == 0 {
		t.Fatalf("events = %+v", events)
	}

	f.reply(200, `{"object":"list","data":[{"label":"1080p","width":1920,"height":1080,"bytes":5,"path":"1080p.mp4","url":"u"}],"has_more":false}`)
	outputs, err := c.Jobs.Outputs(ctx, "job_1")
	must(t, err)
	f.expect("GET", "/v1/jobs/job_1/outputs")
	if len(outputs) != 1 || outputs[0].Label != "1080p" || outputs[0].Path != "1080p.mp4" {
		t.Fatalf("outputs = %+v", outputs)
	}

	f.reply(200, `{"url":"https://example.com/signed","expires_at":"2026-09-27T12:00:00Z"}`)
	u, err := c.Jobs.OutputURL(ctx, "job_1", "1080p")
	must(t, err)
	r := f.expect("GET", "/v1/jobs/job_1/outputs/1080p")
	if qv(r.Query, "redirect") != "false" || u.URL != "https://example.com/signed" || u.ExpiresAt.IsZero() {
		t.Fatalf("output url %+v, query %v", u, r.Query)
	}

	_, err = c.Jobs.FileURL(ctx, "job_1", "720p/seg 00001.m4s")
	must(t, err)
	r = f.expect("GET", "/v1/jobs/job_1/files/720p/seg%2000001.m4s")
	if qv(r.Query, "redirect") != "false" {
		t.Fatalf("query = %v", r.Query)
	}

	f.reply(200, `{"data":[{"id":"dlv_1","connection_id":"con_1","status":"succeeded","files":3}],"has_more":false}`)
	deliveries, err := c.Jobs.Deliveries(ctx, "job_1")
	must(t, err)
	f.expect("GET", "/v1/jobs/job_1/deliveries")
	if len(deliveries) != 1 || deliveries[0].Files != 3 {
		t.Fatalf("deliveries = %+v", deliveries)
	}

	f.reply(201, `{"id":"dlv_2","connection_id":"con_2","status":"pending"}`)
	d, err := c.Jobs.Deliver(ctx, "job_1", &JobDestination{ConnectionID: "con_2", Prefix: "out/"})
	must(t, err)
	r = f.expect("POST", "/v1/jobs/job_1/deliveries")
	if string(r.Body) != `{"connection_id":"con_2","prefix":"out/"}` || d.ID != "dlv_2" {
		t.Fatalf("body %s, delivery %+v", r.Body, d)
	}
}

func TestJobIDsAreEscaped(t *testing.T) {
	f := newFakeAPI(t)
	_, _ = f.client().Jobs.Get(context.Background(), "job/../x")
	f.expect("GET", "/v1/jobs/job%2F..%2Fx")
}

func TestJobStatusIsTerminal(t *testing.T) {
	terminal := map[JobStatus]bool{JobCompleted: true, JobFailed: true, JobCanceled: true}
	for _, s := range JobStatuses {
		if s.IsTerminal() != terminal[s] {
			t.Errorf("%s.IsTerminal() = %v", s, s.IsTerminal())
		}
	}
	if len(JobStatuses) != 7 {
		t.Fatalf("JobStatuses = %v", JobStatuses)
	}
}

func TestWaitFor(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"id":"job_1","status":"queued"}`).
		reply(200, `{"id":"job_1","status":"running","progress":{"percent":50}}`).
		reply(200, `{"id":"job_1","status":"failed","error":{"code":"decode_failed","message":"bad","retryable":false}}`)
	var seen []JobStatus
	job, err := f.client().Jobs.WaitFor(context.Background(), "job_1", &WaitForOptions{
		PollInterval: time.Millisecond,
		OnProgress:   func(j *Job) { seen = append(seen, j.Status) },
	})
	must(t, err)
	if job.Status != JobFailed || job.Error == nil || job.Error.Code != "decode_failed" {
		t.Fatalf("job = %+v", job)
	}
	if len(seen) != 3 || seen[1] != JobRunning {
		t.Fatalf("progress calls = %v", seen)
	}
	for _, r := range f.all() {
		if r.Method != "GET" || r.Path != "/v1/jobs/job_1" {
			t.Fatalf("request %s %s", r.Method, r.Path)
		}
	}
}

func TestWaitForTimeout(t *testing.T) {
	f := newFakeAPI(t)
	for i := 0; i < 50; i++ {
		f.reply(200, `{"id":"job_1","status":"running"}`)
	}
	job, err := f.client().Jobs.WaitFor(context.Background(), "job_1", &WaitForOptions{PollInterval: 5 * time.Millisecond, Timeout: 20 * time.Millisecond})
	var wt *WaitTimeoutError
	if !errors.As(err, &wt) || wt.JobID != "job_1" || wt.Job == nil || wt.Job.Status != JobRunning || job == nil {
		t.Fatalf("err = %v", err)
	}
	if wt.Error() != "transcdr: job job_1 was still running after 20ms" {
		t.Fatalf("message = %q", wt.Error())
	}
}

func TestWaitForContextCancel(t *testing.T) {
	f := newFakeAPI(t)
	for i := 0; i < 50; i++ {
		f.reply(200, `{"id":"job_1","status":"queued"}`)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := f.client().Jobs.WaitFor(ctx, "job_1", &WaitForOptions{PollInterval: 10 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitForAPIError(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(404, `{"error":{"type":"invalid_request_error","message":"No such job."}}`)
	_, err := f.client().Jobs.WaitFor(context.Background(), "job_x", nil)
	if !IsNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestProbe(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"id":"job_p","kind":"probe","status":"completed"}`)
	job, err := f.client().Probe.Create(context.Background(), &ProbeParams{Input: URLInput("https://example.com/in.mp4"), Wait: true})
	must(t, err)
	r := f.expect("POST", "/v1/probe")
	if qv(r.Query, "wait") != "true" || string(r.Body) != `{"input":{"type":"url","url":"https://example.com/in.mp4"}}` || job.Kind != "probe" {
		t.Fatalf("query %v, body %s", r.Query, r.Body)
	}
	_, err = f.client().Probe.Create(context.Background(), &ProbeParams{Input: AssetInput("ast_1")})
	must(t, err)
	if _, ok := f.last().Query["wait"]; ok {
		t.Fatal("wait sent without Wait")
	}
}
