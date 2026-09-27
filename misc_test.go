package transcdr

import (
	"encoding/json"
	"strings"
	"testing"
)

const presetBody = `{"object":"preset","id":"pre_1","slug":"broadcast-cbr","name":"Broadcast CBR","description":"","system":false,
	"output":{"mode":"hls","codec":"h264","renditions":[{"width":1920,"height":1080,"bitrate":"6M"}],"ladder":null,"quality":{"target":"cbr","bitrate":"3M","buffer_ms":1500},
	"gop":null,"segment_seconds":4.0,"audio":{"mode":"auto"},"subtitles":null,"color":"sdr","bit_depth":"auto","max_fps":null,"filters":null,"trim":null},
	"metadata":{"team":"broadcast"},"created_at":"2026-09-27T10:00:00Z","updated_at":"2026-09-27T10:00:00Z"}`

func TestPresets(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "presets.json"))
	page, err := c.Presets.List(fxCtx, &ListParams{Limit: 50})
	must(t, err)
	if r := f.expect("GET", "/v1/presets"); r.Query["limit"][0] != "50" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(page.Data) == 0 || !page.Data[0].System {
		t.Fatalf("page = %+v", page)
	}

	f.reply(200, `{"object":"list","data":[`+presetBody+`],"has_more":false,"next_cursor":null}`)
	all, err := Collect(c.Presets.All(fxCtx, nil), 0)
	must(t, err)
	if len(all) != 1 {
		t.Fatalf("all = %d", len(all))
	}

	// A raw spec is sent exactly as given.
	raw := `{"mode":"hls","codec":"h264","quality":{"target":"cbr","bitrate":"3M","buffer_ms":1500}}`
	f.reply(201, presetBody)
	p, err := c.Presets.Create(fxCtx, &PresetCreateParams{Name: "Broadcast CBR", Slug: String("broadcast-cbr"), Output: RawOutputSpec(json.RawMessage(raw)), Metadata: Metadata{"team": "broadcast"}})
	must(t, err)
	r := f.expect("POST", "/v1/presets")
	if !strings.Contains(string(r.Body), `"output":`+raw) {
		t.Fatalf("body = %s", r.Body)
	}
	fxBody(t, r, map[string]any{
		"name": "Broadcast CBR", "slug": "broadcast-cbr", "metadata": map[string]any{"team": "broadcast"},
		"output": map[string]any{"mode": "hls", "codec": "h264", "quality": map[string]any{"target": "cbr", "bitrate": "3M", "buffer_ms": float64(1500)}},
	})
	if p.Output.Quality.Target == nil || *p.Output.Quality.Target != QualityCBR || *p.Output.Renditions[0].Bitrate != "6M" {
		t.Fatalf("output = %+v", p.Output)
	}
	if !strings.Contains(string(p.Output.Raw()), `"buffer_ms":1500`) {
		t.Fatalf("Raw lost the spec: %s", p.Output.Raw())
	}

	// A typed spec: only the fields set are sent; Null sends null.
	f.reply(201, presetBody)
	_, err = c.Presets.Create(fxCtx, &PresetCreateParams{Name: "x", Output: &OutputSpecInput{Codec: "av1", Quality: &Quality{Target: String(QualityVMAF(93))}, Ladder: Null[Ladder]()}})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"name": "x", "output": map[string]any{"codec": "av1", "quality": map[string]any{"target": "vmaf=93"}, "ladder": nil}})

	f.reply(200, presetBody)
	_, err = c.Presets.Get(fxCtx, "hls-av1-abr")
	must(t, err)
	f.expect("GET", "/v1/presets/hls-av1-abr")

	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Description: String("new"), Metadata: Metadata{}})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/presets/pre_1"), map[string]any{"description": "new", "metadata": map[string]any{}})

	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Name: String("n")})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"name": "n"})

	f.reply(204, "")
	must(t, c.Presets.Delete(fxCtx, "pre_1"))
	f.expect("DELETE", "/v1/presets/pre_1")
}

func TestEvents(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "events.json"))
	page, err := c.Events.List(fxCtx, &EventListParams{Type: EventJobCompleted, ListParams: ListParams{Limit: 3}})
	must(t, err)
	r := f.expect("GET", "/v1/events")
	if r.Query["type"][0] != "job.completed" || r.Query["limit"][0] != "3" {
		t.Fatalf("query = %v", r.Query)
	}
	for _, e := range page.Data {
		if strings.HasPrefix(e.Type, "job.") {
			job, err := e.Data.Job()
			must(t, err)
			if job.ID == "" {
				t.Fatalf("event %s: no job", e.ID)
			}
		}
	}

	f.reply(200, fixture(t, "events.json"))
	n := 0
	for _, err := range c.Events.All(fxCtx, nil) {
		must(t, err)
		n++
	}
	if n != len(page.Data) {
		t.Fatalf("All yielded %d, want %d", n, len(page.Data))
	}

	f.reply(200, `{"object":"event","id":"evt_1","type":"connection.disabled","created_at":"2026-09-27T10:00:00Z",
		"data":{"object":{"object":"connection","id":"con_1","name":"q","kind":"sqs","enabled":false,"disabled_at":"2026-09-27T10:00:00Z",
		"disabled_reason":"reading a queue: denied","activity":"reading a queue","error":"denied","explanation":"fix the policy","automations":["aut_1"]}}}`)
	e, err := c.Events.Get(fxCtx, "evt_1")
	must(t, err)
	f.expect("GET", "/v1/events/evt_1")
	conn, err := e.Data.Connection()
	must(t, err)
	if conn.Activity != "reading a queue" || conn.Automations[0] != "aut_1" {
		t.Fatalf("connection = %+v", conn)
	}
}

func TestUsage(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, fixture(t, "usage.json"))
	u, err := f.client().Usage.Get(fxCtx, &UsageParams{From: "2026-09-01", To: "2026-09-30", Granularity: "day"})
	must(t, err)
	r := f.expect("GET", "/v1/usage")
	if r.Query["from"][0] != "2026-09-01" || r.Query["to"][0] != "2026-09-30" || r.Query["granularity"][0] != "day" {
		t.Fatalf("query = %v", r.Query)
	}
	if u.ByCodec["av1"] == 0 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestBilling(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	billing := fixture(t, "billing.json")

	f.reply(200, billing)
	b, err := c.Billing.Get(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/billing")
	if b.Plan.ID == "" {
		t.Fatal("no plan")
	}

	f.reply(200, `{"object":"checkout","url":"https://pay.example.com/c","changed":false}`)
	co, err := c.Billing.Checkout(fxCtx, &CheckoutParams{Plan: "growth"})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/billing/checkout"), map[string]any{"plan": "growth"})
	if co.URL == nil || *co.URL != "https://pay.example.com/c" {
		t.Fatalf("checkout = %+v", co)
	}

	f.reply(200, `{"object":"checkout","url":null,"changed":true,"plan":"scale"}`)
	co, err = c.Billing.Checkout(fxCtx, &CheckoutParams{CreditCents: 5000})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"credit_cents": float64(5000)})
	if co.URL != nil || !co.Changed {
		t.Fatalf("checkout = %+v", co)
	}

	f.reply(200, `{"object":"portal","url":"https://pay.example.com/p"}`)
	portal, err := c.Billing.Portal(fxCtx)
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/billing/portal"), map[string]any{})
	if portal.URL == "" {
		t.Fatal("no portal URL")
	}

	f.reply(200, billing)
	_, err = c.Billing.UpdateSettings(fxCtx, &BillingSettingsParams{
		MonthlyLimitCents: Null[int64](),
		AutoRecharge:      &AutoRechargeParams{Enabled: Bool(true), AmountCents: Int64(2000), MonthlyCapCents: Null[int64]()},
	})
	must(t, err)
	fxBody(t, f.expect("PUT", "/v1/billing/settings"), map[string]any{
		"monthly_limit_cents": nil,
		"auto_recharge":       map[string]any{"enabled": true, "amount_cents": float64(2000), "monthly_cap_cents": nil},
	})

	f.reply(200, billing)
	_, err = c.Billing.UpdateSettings(fxCtx, &BillingSettingsParams{MonthlyLimitCents: Value[int64](50_000)})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"monthly_limit_cents": float64(50000)})

	f.reply(200, billing)
	_, err = c.Billing.UpdateSettings(fxCtx, &BillingSettingsParams{AutoRecharge: &AutoRechargeParams{ThresholdCents: Int64(500)}})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"auto_recharge": map[string]any{"threshold_cents": float64(500)}})

	f.reply(200, fixture(t, "transactions.json"))
	tx, err := c.Billing.Transactions(fxCtx, &CreditTransactionListParams{Limit: 200})
	must(t, err)
	if r := f.expect("GET", "/v1/billing/transactions"); r.Query["limit"][0] != "200" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(tx.Data) == 0 {
		t.Fatal("no transactions")
	}

	f.reply(200, billing)
	_, err = c.Billing.ChangePlan(fxCtx, "scale")
	must(t, err)
	fxBody(t, f.expect("PUT", "/v1/billing/plan"), map[string]any{"plan": "scale"})

	f.reply(200, fixture(t, "statements.json"))
	st, err := c.Billing.Invoices.List(fxCtx, &ListParams{Limit: 12})
	must(t, err)
	if r := f.expect("GET", "/v1/billing/invoices"); r.Query["limit"][0] != "12" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(st.Data) == 0 {
		t.Fatal("no statements")
	}

	f.reply(200, fixture(t, "statements.json"))
	all, err := Collect(c.Billing.Invoices.All(fxCtx, nil), 0)
	must(t, err)
	if len(all) != len(st.Data) {
		t.Fatalf("All = %d, want %d", len(all), len(st.Data))
	}
}

func TestPublicInfo(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "plans.json"))
	plans, err := c.Plans.List(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/plans")
	if len(plans) == 0 || len(plans[0].Features) == 0 {
		t.Fatalf("plans = %+v", plans)
	}

	f.reply(200, fixture(t, "capabilities.json"))
	caps, err := c.Capabilities.Get(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/capabilities")
	if len(caps.Codecs) == 0 || !strings.Contains(string(caps.Raw()), `"input_containers"`) || !strings.Contains(string(caps.Limits), "max_width") {
		t.Fatalf("capabilities = %+v", caps)
	}

	f.reply(200, fixture(t, "status.json"))
	st, err := c.Status.Get(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/status")
	if st.Status != "operational" || st.Version == "" {
		t.Fatalf("status = %+v", st)
	}

	f.reply(200, fixture(t, "stats.json"))
	stats, err := c.Stats.Get(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/stats")
	if len(stats.Daily) == 0 || stats.Since.IsZero() {
		t.Fatalf("stats = %+v", stats)
	}

	f.reply(200, `{"openapi":"3.1.0","info":{"title":"Transcdr"}}`)
	doc, err := c.OpenAPI(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/openapi.json")
	if !strings.Contains(string(doc), `"3.1.0"`) {
		t.Fatalf("doc = %s", doc)
	}
}
