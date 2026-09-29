package transcdr

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

var ixCtx = context.Background()

// qv is the first value of a query parameter.
func qv(q map[string][]string, key string) string {
	if v := q[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func ixJSONEq(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got invalid JSON %s", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want invalid JSON %s", want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("body = %s\nwant   %s", got, want)
	}
}

func TestConnectionsListAndAll(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"object":"list","data":[{"id":"con_1"}],"has_more":true,"next_cursor":"con_1"}`)
	page, err := f.client().Connections.List(ixCtx, &ListParams{Limit: 5, Cursor: "con_0"})
	must(t, err)
	r := f.expect("GET", "/v1/connections")
	if r.Query["limit"][0] != "5" || r.Query["cursor"][0] != "con_0" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(page.Data) != 1 || !page.HasMore || page.NextCursor != "con_1" {
		t.Fatalf("page = %+v", page)
	}

	f2 := newFakeAPI(t)
	f2.reply(200, `{"data":[{"id":"con_1"}],"has_more":true,"next_cursor":"con_1"}`).
		reply(200, `{"data":[{"id":"con_2"}],"has_more":false,"next_cursor":null}`)
	all, err := Collect(f2.client().Connections.All(ixCtx, nil), 0)
	must(t, err)
	if len(all) != 2 || all[1].ID != "con_2" {
		t.Fatalf("all = %+v", all)
	}
	if got := f2.all()[1].Query["cursor"]; len(got) != 1 || got[0] != "con_1" {
		t.Fatalf("second page cursor = %v", got)
	}
}

func TestConnectionsCreate(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, `{"id":"con_1","kind":"s3","status":"ok","enabled":true}`)
	conn, err := f.client().Connections.Create(ixCtx, &ConnectionCreateParams{
		Name: "Ingest",
		Kind: KindS3,
		Config: ConnectionConfig{
			Bucket:    Value("media"),
			Region:    Value("us-east-1"),
			PathStyle: Bool(true),
			Endpoint:  Null[string](),
		},
		Secrets: &ConnectionSecrets{AccessKeyID: String("AKIAEXAMPLE"), SecretAccessKey: String("secret-example")},
	})
	must(t, err)
	if conn.ID != "con_1" {
		t.Fatalf("conn = %+v", conn)
	}
	r := f.expect("POST", "/v1/connections")
	ixJSONEq(t, r.Body, `{"name":"Ingest","kind":"s3",
		"config":{"bucket":"media","region":"us-east-1","path_style":true,"endpoint":null},
		"secrets":{"access_key_id":"AKIAEXAMPLE","secret_access_key":"secret-example"}}`)
	if !uuidShape(r.Header.Get("Idempotency-Key")) {
		t.Errorf("Idempotency-Key = %q", r.Header.Get("Idempotency-Key"))
	}
}

func TestConnectionsGetUpdateEnableDisable(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	_, err := c.Connections.Get(ixCtx, "con_1")
	must(t, err)
	f.expect("GET", "/v1/connections/con_1")

	_, err = c.Connections.Update(ixCtx, "con_1", &ConnectionUpdateParams{
		Name:    String("Renamed"),
		Config:  &ConnectionConfig{Root: Null[string](), Port: Value(2222)},
		Secrets: &ConnectionSecrets{Password: String("")},
	})
	must(t, err)
	r := f.expect("PATCH", "/v1/connections/con_1")
	ixJSONEq(t, r.Body, `{"name":"Renamed","config":{"root":null,"port":2222},"secrets":{"password":""}}`)

	_, err = c.Connections.Enable(ixCtx, "con_1")
	must(t, err)
	ixJSONEq(t, f.expect("PATCH", "/v1/connections/con_1").Body, `{"enabled":true}`)
	_, err = c.Connections.Disable(ixCtx, "con_1")
	must(t, err)
	ixJSONEq(t, f.expect("PATCH", "/v1/connections/con_1").Body, `{"enabled":false}`)
}

func TestConnectionsDeleteConflict(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(409, `{"error":{"type":"invalid_request_error","code":"connection_in_use","message":"An automation uses this connection; delete or change it first.","request_id":"req_1"}}`)
	err := f.client().Connections.Delete(ixCtx, "con_1")
	f.expect("DELETE", "/v1/connections/con_1")
	if !IsConflict(err) || !IsInvalidRequest(err) {
		t.Fatalf("err = %v", err)
	}
	e, _ := AsError(err)
	if e.Code != "connection_in_use" || e.RequestID != "req_1" {
		t.Fatalf("error = %+v", e)
	}
	if len(f.all()) != 1 {
		t.Fatal("a 409 is not retried")
	}
}

func TestConnectionsTestCheckBrowse(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"connection_test","ok":false,"error":"Access denied","connection":{"id":"con_1","status":"error"}}`)
	res, err := c.Connections.Test(ixCtx, "con_1")
	must(t, err)
	f.expect("POST", "/v1/connections/con_1/test")
	if res.OK || res.Error == nil || *res.Error != "Access denied" || res.Connection.Status != "error" {
		t.Fatalf("test = %+v", res)
	}

	f.reply(200, `{"object":"connection_check","ok":true,
		"steps":[{"id":"identity","label":"Sign in","status":"passed","detail":"arn:aws:iam::123456789012:user/transcdr"}],
		"identity":{"provider":"aws","arn":"arn:aws:iam::123456789012:user/transcdr","account":"123456789012"},
		"setup":{"iam_policy":{"Version":"2012-10-17"}},
		"roles":{"source":true,"watch_folder":true,"destination":false}}`)
	check, err := c.Connections.Check(ixCtx, &ConnectionCheckParams{Kind: KindS3, Config: ConnectionConfig{Bucket: Value("media")}})
	must(t, err)
	r := f.expect("POST", "/v1/connections/check")
	ixJSONEq(t, r.Body, `{"kind":"s3","config":{"bucket":"media"}}`)
	if !check.OK || len(check.Steps) != 1 || check.Identity.Account != "123456789012" ||
		check.Roles.Source == nil || !*check.Roles.Source || check.Roles.Destination == nil || *check.Roles.Destination {
		t.Fatalf("check = %+v", check)
	}
	ixJSONEq(t, check.Setup, `{"iam_policy":{"Version":"2012-10-17"}}`)

	f.reply(200, `{"object":"connection_check","ok":false,"steps":[],"identity":null,"setup":null,"roles":{"trigger":false},"connection":{"id":"con_1","status":"error"}}`)
	saved, err := c.Connections.CheckSaved(ixCtx, "con_1")
	must(t, err)
	f.expect("POST", "/v1/connections/con_1/check")
	if saved.Connection == nil || saved.Connection.ID != "con_1" || saved.Roles.Trigger == nil || *saved.Roles.Trigger {
		t.Fatalf("saved = %+v", saved)
	}

	f.reply(200, `{"object":"list","data":[{"object":"remote_object","path":"incoming/a.mp4","size":12,"last_modified":"2026-09-27T10:00:00Z"},{"path":"incoming/sub/","size":null,"last_modified":null}],"has_more":false}`)
	page, err := c.Connections.Browse(ixCtx, "con_1", &BrowseParams{Prefix: "incoming/", Recursive: true})
	must(t, err)
	r = f.expect("GET", "/v1/connections/con_1/browse")
	if qv(r.Query, "prefix") != "incoming/" || qv(r.Query, "recursive") != "true" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(page.Data) != 2 || *page.Data[0].Size != 12 || page.Data[1].Size != nil {
		t.Fatalf("browse = %+v", page.Data)
	}
	_, err = c.Connections.Browse(ixCtx, "con_1", nil)
	must(t, err)
	if len(f.last().Query) != 0 {
		t.Fatalf("no params, no query: %v", f.last().Query)
	}
}

func TestAutomationsCRUD(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"data":[{"id":"aut_1"}],"has_more":false}`)
	page, err := c.Automations.List(ixCtx, &ListParams{Limit: 10})
	must(t, err)
	if qv(f.expect("GET", "/v1/automations").Query, "limit") != "10" || len(page.Data) != 1 {
		t.Fatal("list")
	}
	f.reply(200, `{"data":[{"id":"aut_1"},{"id":"aut_2"}],"has_more":false}`)
	all, err := Collect(c.Automations.All(ixCtx, nil), 0)
	must(t, err)
	if len(all) != 2 {
		t.Fatalf("all = %v", all)
	}

	f.reply(201, `{"id":"aut_1","trigger":"queue","hook_url":"https://api.example.com/v1/hooks/automations/ahk_example"}`)
	a, err := c.Automations.Create(ixCtx, &AutomationParams{
		Name:                "Ingest",
		Trigger:             TriggerQueue,
		TriggerConnectionID: Value("con_q"),
		Source:              &AutomationSourceParams{ConnectionID: "con_s", Prefix: String("incoming/"), Pattern: String("**/*.mp4")},
		PollIntervalSeconds: Int(120),
		SettleSeconds:       Int(0),
		Preset:              Value("hls-av1-abr"),
		Output:              Value(OutputOverrides{"video": map[string]any{"codec": "h264"}}),
		Destination:         Value(JobDestination{ConnectionID: "con_s", Prefix: "out/{stem}/"}),
		AfterSuccess:        "delete",
		Priority:            PriorityHigh,
		Metadata:            Value(Metadata{"team": "video"}),
		WebhookURL:          Value("https://example.com/jobs"),
	})
	must(t, err)
	if a.HookURL == nil || a.Trigger != "queue" {
		t.Fatalf("automation = %+v", a)
	}
	ixJSONEq(t, f.expect("POST", "/v1/automations").Body, `{"name":"Ingest","trigger":"queue","trigger_connection_id":"con_q",
		"source":{"connection_id":"con_s","prefix":"incoming/","pattern":"**/*.mp4"},
		"poll_interval_seconds":120,"settle_seconds":0,"preset":"hls-av1-abr","output":{"video":{"codec":"h264"}},
		"destination":{"connection_id":"con_s","prefix":"out/{stem}/"},"after_success":"delete","priority":"high",
		"metadata":{"team":"video"},"webhook_url":"https://example.com/jobs"}`)

	_, err = c.Automations.Get(ixCtx, "aut_1")
	must(t, err)
	f.expect("GET", "/v1/automations/aut_1")

	// Clearing: every nullable field sent as null.
	_, err = c.Automations.Update(ixCtx, "aut_1", &AutomationParams{
		TriggerConnectionID: Null[string](),
		Destination:         Null[JobDestination](),
		Metadata:            Null[Metadata](),
		Output:              Null[OutputOverrides](),
		Preset:              Null[string](),
		WebhookURL:          Null[string](),
		Enabled:             Bool(false),
	})
	must(t, err)
	ixJSONEq(t, f.expect("PATCH", "/v1/automations/aut_1").Body,
		`{"enabled":false,"trigger_connection_id":null,"preset":null,"output":null,"destination":null,"metadata":null,"webhook_url":null}`)

	// Overrides with a removal, and an empty metadata map, are sent as they are.
	_, err = c.Automations.Update(ixCtx, "aut_1", &AutomationParams{
		Output:   Value(OutputOverrides{"container": map[string]any{"format": "mp4", "segment_seconds": nil}}),
		Metadata: Value(Metadata{}),
	})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"output":{"container":{"format":"mp4","segment_seconds":null}},"metadata":{}}`)

	// Only what is set is sent.
	_, err = c.Automations.Update(ixCtx, "aut_1", &AutomationParams{Name: "Renamed"})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"name":"Renamed"}`)

	must(t, c.Automations.Delete(ixCtx, "aut_1"))
	f.expect("DELETE", "/v1/automations/aut_1")
}

func TestAutomationsRunTriggerRotateItems(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"automation_run","jobs_created":1,"messages_received":3,"messages_deleted":3}`)
	run, err := c.Automations.Run(ixCtx, "aut_1")
	must(t, err)
	f.expect("POST", "/v1/automations/aut_1/run")
	if run.JobsCreated != 1 || *run.MessagesReceived != 3 || *run.MessagesDeleted != 3 {
		t.Fatalf("run = %+v", run)
	}

	f.reply(200, `{"object":"automation_run","jobs_created":1,"job_ids":["job_1"]}`)
	run, err = c.Automations.Trigger(ixCtx, "aut_1", &AutomationTriggerParams{Path: "incoming/a.mp4"})
	must(t, err)
	ixJSONEq(t, f.expect("POST", "/v1/automations/aut_1/trigger").Body, `{"path":"incoming/a.mp4"}`)
	if len(run.JobIDs) != 1 || run.MessagesReceived != nil {
		t.Fatalf("run = %+v", run)
	}
	_, err = c.Automations.Trigger(ixCtx, "aut_1", &AutomationTriggerParams{Paths: []string{"a.mp4", "b.mp4"}})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"paths":["a.mp4","b.mp4"]}`)

	f.reply(200, `{"id":"aut_1","hook_url":"https://api.example.com/v1/hooks/automations/ahk_new"}`)
	a, err := c.Automations.RotateHookToken(ixCtx, "aut_1")
	must(t, err)
	f.expect("POST", "/v1/automations/aut_1/rotate-hook-token")
	if *a.HookURL != "https://api.example.com/v1/hooks/automations/ahk_new" {
		t.Fatalf("hook = %v", *a.HookURL)
	}

	f.reply(200, fixture(t, "automation_items.json"))
	items, err := c.Automations.Items(ixCtx, "aut_1", &ListParams{Limit: 50})
	must(t, err)
	if qv(f.expect("GET", "/v1/automations/aut_1/items").Query, "limit") != "50" {
		t.Fatal("items limit")
	}
	it := items.Data[0]
	if it.Path != "fanout/b.mp4" || it.Status != "job_created" || *it.JobID != "job_hdHSxv0XVtbwdfEHlPqDSW" || *it.SizeBytes != 788493 || it.Error != nil {
		t.Fatalf("item = %+v", it)
	}
}

func TestAutomationsPushHookSendsNoKey(t *testing.T) {
	hook := newFakeAPI(t)
	hook.reply(202, `{"jobs_created":1,"job_ids":["job_1"]}`)
	api := newFakeAPI(t)
	c := api.client()
	run, err := c.Automations.PushHook(ixCtx, hook.server.URL+"/v1/hooks/automations/ahk_example", map[string]string{"path": "incoming/a.mp4"})
	must(t, err)
	r := hook.expect("POST", "/v1/hooks/automations/ahk_example")
	if got := r.Header.Get("Authorization"); got != "" {
		t.Fatalf("the hook URL is the credential; Authorization = %q", got)
	}
	ixJSONEq(t, r.Body, `{"path":"incoming/a.mp4"}`)
	if run.JobsCreated != 1 || len(api.all()) != 0 {
		t.Fatalf("run = %+v, api requests = %d", run, len(api.all()))
	}
	// The client keeps its key for other calls.
	_, err = c.Automations.Get(ixCtx, "aut_1")
	must(t, err)
	if api.last().Header.Get("Authorization") != "Bearer tdk_test_example" {
		t.Fatal("key lost after PushHook")
	}
}

func TestDeliveriesRetry(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"object":"delivery","id":"dlv_1","connection_id":"con_1","prefix":"out/job_1/","status":"pending","files":0,"bytes":0,"attempts":2,"error":null,"next_retry_at":null,"created_at":"2026-09-27T10:00:00Z","completed_at":null}`)
	d, err := f.client().Deliveries.Retry(ixCtx, "dlv_1")
	must(t, err)
	f.expect("POST", "/v1/deliveries/dlv_1/retry")
	if d.Status != "pending" || d.Attempts != 2 || d.CompletedAt != nil {
		t.Fatalf("delivery = %+v", d)
	}
}

func TestDecodeConnectionsFixture(t *testing.T) {
	var page Page[Connection]
	must(t, json.Unmarshal([]byte(fixture(t, "connections.json")), &page))
	if len(page.Data) != 6 {
		t.Fatalf("connections = %d", len(page.Data))
	}
	q := page.Data[0]
	if q.ID != "con_tPnA0CKgVVoSDXJiK5eMIC" || q.Kind != KindSQS || q.Class != "messaging" || q.Status != "ok" || !q.Enabled ||
		!q.Capabilities.Trigger || !q.Capabilities.Events || q.Capabilities.Source {
		t.Fatalf("sqs = %+v", q)
	}
	if u, ok := q.Config.QueueURL.Get(); !ok || u != "http://localhost:4566/000000000000/fanout-23391" {
		t.Fatalf("queue_url = %v", q.Config.QueueURL)
	}
	if !reflect.DeepEqual(q.SecretsSet, []string{"access_key_id", "secret_access_key"}) {
		t.Fatalf("secrets_set = %v", q.SecretsSet)
	}
	s3 := page.Data[2]
	if b, _ := s3.Config.Bucket.Get(); b != "media-23391" || s3.Config.PathStyle == nil || !*s3.Config.PathStyle || !s3.Config.Root.IsZero() {
		t.Fatalf("s3 config = %+v", s3.Config)
	}
	if !IsMessagingKind(q.Kind) || IsMessagingKind(s3.Kind) {
		t.Fatal("IsMessagingKind")
	}
}

func TestDecodeAutomationsFixture(t *testing.T) {
	var page Page[Automation]
	must(t, json.Unmarshal([]byte(fixture(t, "automations.json")), &page))
	if len(page.Data) != 4 {
		t.Fatalf("automations = %d", len(page.Data))
	}
	a := page.Data[0]
	if a.ID != "aut_XKyOXawT0EznNSW9quWHKf" || a.Trigger != TriggerQueue || *a.TriggerConnectionID != "con_tPnA0CKgVVoSDXJiK5eMIC" ||
		a.Source.Prefix != "fanout" || a.Source.Pattern != "**/*.mp4" || a.Destination != nil || a.JobsCreated != 1 ||
		a.LastError == nil || a.LastTriggeredAt == nil || a.LastPolledAt != nil {
		t.Fatalf("automation = %+v", a)
	}
	if a.Output == nil || len(a.Output) != 0 || a.ResolvedOutput == nil || a.ResolvedOutput.Video.Codec != CodecAV1 || ValidateOutput(*a.ResolvedOutput) != nil {
		t.Fatalf("output %v, resolved %s", a.Output, a.ResolvedOutput.Raw())
	}
}

func TestDecodeAutomationItemsFixture(t *testing.T) {
	var page Page[AutomationItem]
	must(t, json.Unmarshal([]byte(fixture(t, "automation_items.json")), &page))
	if len(page.Data) != 1 || page.Data[0].CreatedAt.IsZero() {
		t.Fatalf("items = %+v", page.Data)
	}
}
