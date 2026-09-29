package transcdr

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const presetBody = `{"object":"preset","id":"pre_1","slug":"broadcast-cbr","name":"Broadcast CBR","description":"","system":false,"version":2,
	"output":{"kind":"video","container":{"format":"hls","segment_seconds":4},
	"video":{"codec":"h264","cbr":{"bitrate":"3M","buffer_ms":1500},"bit_depth":"from_color","color":"sdr","frame_rate":{"max":"source"},"gop":"segment","filters":[]},
	"audio":{"handling":"auto","codec":"opus","bitrate":"standard","channels":"source","he_aac":"auto","stereo_fallback":false},
	"renditions":{"sizes":[{"label":"by_size","width":1920,"height":1080,"fit":"contain","orientation":"auto","upscale":false,"video":{"cbr":{"bitrate":"6M"}}}]},
	"subtitles":{"tracks":"all"},"trim":{"start":0,"end":"source"},
	"privacy":{"location":"strip","capture_time":"strip","device":"strip","descriptive":"strip"}},
	"metadata":{"team":"broadcast"},"created_at":"2026-09-27T10:00:00Z","updated_at":"2026-09-27T10:00:00Z"}`

// specMap is a spec as the generic JSON a request body decodes to.
func specMap(t *testing.T, s *OutputSpec) map[string]any {
	t.Helper()
	var m map[string]any
	must(t, json.Unmarshal(mustJSON(t, s), &m))
	return m
}

// testSpec is a complete spec: HLS at a constant bit rate.
func testSpec() *OutputSpec {
	s := NewVideoOutput(
		ContainerHLS(4),
		NewVideo(CodecH264, ConstantBitRate("3M", 1500), BitDepthFromColor, ColorSDR, FrameRateSource(), GopSegment(), nil),
		Audio{Handling: HandlingAuto, Codec: AudioCodecOpus, Bitrate: BitrateStandard, Channels: ChannelsSource, HeAac: HeAacAuto, StereoFallback: Bool(false)},
		RenditionSizes(NewSize(LabelBySize, 1920, 1080, FitContain, OrientationAuto, false).WithBitrate("6M")),
		SubtitlesAll(),
		NewTrim(0, TrimEndSource()),
		PrivacyPreset(PrivacyStripAll),
	)
	return &s
}

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

	// A whole spec is sent as built.
	f.reply(201, presetBody)
	p, err := c.Presets.Create(fxCtx, &PresetCreateParams{Name: "Broadcast CBR", Slug: String("broadcast-cbr"), Output: testSpec(), Metadata: Metadata{"team": "broadcast"}})
	must(t, err)
	r := f.expect("POST", "/v1/presets")
	var sent struct {
		Output json.RawMessage `json:"output"`
	}
	must(t, json.Unmarshal(r.Body, &sent))
	sameJSON(t, testSpec(), string(sent.Output))
	if p.Version != 2 || p.Output.Video.CBR == nil || p.Output.Video.CBR.BufferMs != 1500 || p.Output.Renditions.Sizes[0].Video.CBR.Bitrate != "6M" {
		t.Fatalf("output = %+v", p.Output)
	}
	if !strings.Contains(string(p.Output.Raw()), `"buffer_ms":1500`) {
		t.Fatalf("Raw lost the spec: %s", p.Output.Raw())
	}

	f.reply(200, presetBody)
	_, err = c.Presets.Get(fxCtx, "hls-av1-abr")
	must(t, err)
	f.expect("GET", "/v1/presets/hls-av1-abr")

	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Description: Value("new"), Metadata: Value(Metadata{})})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/presets/pre_1"), map[string]any{"description": "new", "metadata": map[string]any{}})

	// Output on update is overrides over the latest version.
	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Output: OutputOverrides{"video": map[string]any{"crf": 23}}})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"output": map[string]any{"video": map[string]any{"crf": float64(23)}}})

	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Description: Null[string](), Metadata: Null[Metadata]()})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"description": nil, "metadata": nil})

	// Replace: PUT with the whole preset; description and metadata left out
	// are not sent (the API empties them).
	f.reply(200, presetBody)
	audio := NewAudioOutput(ContainerAudio(FormatM4A), NewAudio(HandlingEncode, AudioCodecALAC, ChannelsStereo, HeAacAuto), PrivacyPreset(PrivacyStripAll))
	audio.Audio.BitDepth = AudioBitDepth16
	_, err = c.Presets.Replace(fxCtx, "pre_1", &PresetReplaceParams{Name: "Broadcast CBR", Output: &audio})
	must(t, err)
	wantAudio := map[string]any{"kind": "audio", "container": map[string]any{"format": "m4a"}, "privacy": map[string]any{"preset": "strip_all"},
		"audio": map[string]any{"handling": "encode", "codec": "alac", "channels": "stereo", "he_aac": "auto", "bit_depth": "16"}}
	fxBody(t, f.expect("PUT", "/v1/presets/pre_1"), map[string]any{"name": "Broadcast CBR", "output": wantAudio})
	f.reply(200, presetBody)
	_, err = c.Presets.Replace(fxCtx, "pre_1", &PresetReplaceParams{
		Name: "n", Slug: String("s"), Description: "d", Metadata: Metadata{"k": "v"}, Output: &audio,
	})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"name": "n", "slug": "s", "description": "d", "metadata": map[string]any{"k": "v"}, "output": wantAudio})
	if f.last().Header.Get("Idempotency-Key") != "" {
		t.Error("PUT needs no idempotency key")
	}

	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{Name: String("n")})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"name": "n"})

	f.reply(204, "")
	must(t, c.Presets.Delete(fxCtx, "pre_1"))
	f.expect("DELETE", "/v1/presets/pre_1")
}

func TestPresetCategoriesAndCompatibility(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	// System presets say where they are grouped and where they play.
	f.reply(200, fixture(t, "presets.json"))
	page, err := c.Presets.List(fxCtx, nil)
	must(t, err)
	var compat *Preset
	for i := range page.Data {
		if page.Data[i].Slug == "mp4-h264-compat-1080p" {
			compat = &page.Data[i]
		}
	}
	if compat == nil || compat.Category != CategoryTV || len(compat.Compatibility) != 6 || compat.Compatibility[1] != PlatformIOS {
		t.Fatalf("compat = %+v", compat)
	}
	if !strings.Contains(compat.CompatibilityNotes[PlatformIOS], "iOS 17") {
		t.Fatalf("notes = %v", compat.CompatibilityNotes)
	}

	// Unknown categories and platforms decode as they are.
	f.reply(200, `{"id":"x","category":"podcast","compatibility":["web","vr"],"compatibility_notes":{"vr":"Headsets."}}`)
	p, err := c.Presets.Get(fxCtx, "x")
	must(t, err)
	if p.Category != "podcast" || p.Compatibility[1] != "vr" || p.CompatibilityNotes["vr"] != "Headsets." {
		t.Fatalf("preset = %+v", p)
	}

	// Filters: categories are any-of, platforms all-of, both comma-joined.
	f.reply(200, `{"object":"list","data":[],"has_more":false,"next_cursor":null}`)
	_, err = c.Presets.List(fxCtx, &PresetListParams{
		ListParams:     ListParams{Limit: 10},
		Category:       []PresetCategory{CategoryWeb, CategoryMobile},
		CompatibleWith: []Platform{PlatformIOS, PlatformAndroid},
		ExcludeSystem:  true,
	})
	must(t, err)
	q := url.Values(f.expect("GET", "/v1/presets").Query)
	if q.Get("category") != "web,mobile" || q.Get("compatible_with") != "ios,android" || q.Get("system") != "false" || q.Get("limit") != "10" {
		t.Fatalf("query = %v", q)
	}
	f.reply(200, `{"object":"list","data":[],"has_more":false,"next_cursor":null}`)
	_, err = Collect(c.Presets.All(fxCtx, &PresetListParams{CompatibleWith: []Platform{PlatformLegacy}}), 0)
	must(t, err)
	if q := url.Values(f.last().Query); q.Get("compatible_with") != "legacy" || q.Has("category") || q.Has("system") {
		t.Fatalf("query = %v", q)
	}
	var none *PresetListParams
	f.reply(200, `{"object":"list","data":[],"has_more":false,"next_cursor":null}`)
	_, err = c.Presets.List(fxCtx, none)
	must(t, err)
	if len(f.last().Query) != 0 {
		t.Fatalf("query = %v", f.last().Query)
	}

	// Create: set them, or leave them out to derive them.
	f.reply(201, presetBody)
	_, err = c.Presets.Create(fxCtx, &PresetCreateParams{
		Name: "Phones", Output: testSpec(),
		Category:           CategoryMobile,
		Compatibility:      []Platform{PlatformIOS},
		CompatibilityNotes: map[Platform]string{PlatformIOS: "Our app only."},
	})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/presets"), map[string]any{
		"name": "Phones", "output": specMap(t, testSpec()), "category": "mobile",
		"compatibility": []any{"ios"}, "compatibility_notes": map[string]any{"ios": "Our app only."},
	})
	f.reply(201, presetBody)
	_, err = c.Presets.Create(fxCtx, &PresetCreateParams{Name: "x", Output: testSpec(), Compatibility: []Platform{}})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"name": "x", "output": specMap(t, testSpec()), "compatibility": []any{}})

	// Update: Value sets, Null derives again, left out is kept.
	f.reply(200, presetBody)
	_, err = c.Presets.Update(fxCtx, "pre_1", &PresetUpdateParams{
		Category:           Value(CategoryStreaming),
		Compatibility:      Value([]Platform{PlatformSmartTV}),
		CompatibilityNotes: Null[map[Platform]string](),
	})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/presets/pre_1"), map[string]any{
		"category": "streaming", "compatibility": []any{"smart_tv"}, "compatibility_notes": nil,
	})

	// Replace: left out is not sent (the API derives them).
	f.reply(200, presetBody)
	_, err = c.Presets.Replace(fxCtx, "pre_1", &PresetReplaceParams{Name: "n", Output: testSpec(), Category: CategoryArchive})
	must(t, err)
	fxBody(t, f.expect("PUT", "/v1/presets/pre_1"), map[string]any{"name": "n", "output": specMap(t, testSpec()), "category": "archive"})
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
