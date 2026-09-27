package transcdr

import (
	"bytes"
	"encoding/json"
	"testing"
)

// stripObject removes every "object" key, which the SDK does not model.
func stripObject(v any) any {
	switch x := v.(type) {
	case map[string]any:
		delete(x, "object")
		for k, e := range x {
			x[k] = stripObject(e)
		}
	case []any:
		for i, e := range x {
			x[i] = stripObject(e)
		}
	}
	return v
}

// decodeStrict decodes a fixture into v, failing on JSON keys v does not model.
func decodeStrict(t *testing.T, name string, v any) {
	t.Helper()
	raw := fixture(t, name)
	// Leniently first: the real decoder must succeed.
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var generic any
	must(t, json.Unmarshal([]byte(raw), &generic))
	b, err := json.Marshal(stripObject(generic))
	must(t, err)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Errorf("%s: a field the SDK does not model: %v", name, err)
	}
}

// decodeStrictPage decodes a list fixture with [Page]'s own decoder, then
// strictly through an equivalent struct (Page's custom decoder would hide
// unknown fields).
func decodeStrictPage[T any](t *testing.T, name string) *Page[T] {
	t.Helper()
	var p Page[T]
	if err := json.Unmarshal([]byte(fixture(t, name)), &p); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var strict struct {
		Data       []T     `json:"data"`
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor"`
	}
	decodeStrict(t, name, &strict)
	return &p
}

// Types with a custom decoder, checked strictly through a method-less copy.
type (
	strictSpec  OutputSpec
	strictInput OutputSpecInput
	strictCaps  Capabilities
)

// checkNested strictly decodes the JSON at key in each object of a fixture
// (or of its data list) into v.
func checkNested(t *testing.T, name, key string, newV func() any) {
	t.Helper()
	var generic any
	must(t, json.Unmarshal([]byte(fixture(t, name)), &generic))
	var objects []any
	if m, ok := generic.(map[string]any); ok {
		if data, ok := m["data"].([]any); ok {
			objects = data
		} else {
			objects = []any{m}
		}
	}
	for _, o := range objects {
		m, ok := o.(map[string]any)
		if !ok || m[key] == nil {
			continue
		}
		b, err := json.Marshal(stripObject(m[key]))
		must(t, err)
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(newV()); err != nil {
			t.Errorf("%s: %s: a field the SDK does not model: %v", name, key, err)
		}
	}
}

func TestFixturesNestedSpecs(t *testing.T) {
	checkNested(t, "job.json", "output", func() any { return new(strictSpec) })
	checkNested(t, "jobs.json", "output", func() any { return new(strictSpec) })
	checkNested(t, "presets.json", "output", func() any { return new(strictSpec) })
	checkNested(t, "automations.json", "output", func() any { return new(strictInput) })
	var caps strictCaps
	decodeStrict(t, "capabilities.json", &caps)
}

func TestFixturesDecode(t *testing.T) {
	cases := map[string]func(t *testing.T){
		"api_keys.json": func(t *testing.T) {
			p := decodeStrictPage[APIKey](t, "api_keys.json")
			if len(p.Data) == 0 || p.Data[0].ID == "" {
				t.Fatal("no keys decoded")
			}
		},
		"assets.json": func(t *testing.T) {
			p := decodeStrictPage[Asset](t, "assets.json")
			if len(p.Data) == 0 || p.Data[0].Filename == "" {
				t.Fatal("no assets decoded")
			}
		},
		"automation_items.json": func(t *testing.T) {
			p := decodeStrictPage[AutomationItem](t, "automation_items.json")
			if len(p.Data) == 0 || p.Data[0].Path == "" {
				t.Fatal("no items decoded")
			}
		},
		"automations.json": func(t *testing.T) {
			p := decodeStrictPage[Automation](t, "automations.json")
			if len(p.Data) == 0 || p.Data[0].Trigger == "" {
				t.Fatal("no automations decoded")
			}
		},
		"billing.json": func(t *testing.T) {
			var b Billing
			decodeStrict(t, "billing.json", &b)
			if b.Plan.ID == "" {
				t.Fatal("no plan decoded")
			}
		},
		"capabilities.json": func(t *testing.T) {
			var c Capabilities
			decodeStrict(t, "capabilities.json", &c)
			if len(c.Codecs) == 0 || len(c.SystemPresets) == 0 {
				t.Fatal("capabilities incomplete")
			}
		},
		"connections.json": func(t *testing.T) {
			p := decodeStrictPage[Connection](t, "connections.json")
			if len(p.Data) == 0 || p.Data[0].Kind == "" {
				t.Fatal("no connections decoded")
			}
		},
		"events.json": func(t *testing.T) {
			p := decodeStrictPage[Event](t, "events.json")
			if len(p.Data) == 0 || len(p.Data[0].Data.Object) == 0 {
				t.Fatal("no events decoded")
			}
		},
		"job.json": func(t *testing.T) {
			var j Job
			decodeStrict(t, "job.json", &j)
			if j.ID == "" || j.Output.Codec == "" {
				t.Fatal("job incomplete")
			}
		},
		"job_events.json": func(t *testing.T) {
			p := decodeStrictPage[JobEvent](t, "job_events.json")
			if len(p.Data) == 0 || p.Data[0].Type == "" {
				t.Fatal("no job events decoded")
			}
		},
		"jobs.json": func(t *testing.T) {
			p := decodeStrictPage[Job](t, "jobs.json")
			if len(p.Data) == 0 {
				t.Fatal("no jobs decoded")
			}
		},
		"me.json": func(t *testing.T) {
			var m Me
			decodeStrict(t, "me.json", &m)
			if m.Organization.ID == "" {
				t.Fatal("me incomplete")
			}
		},
		"members.json": func(t *testing.T) {
			p := decodeStrictPage[User](t, "members.json")
			if len(p.Data) == 0 || p.Data[0].Email == "" {
				t.Fatal("no members decoded")
			}
		},
		"organization.json": func(t *testing.T) {
			var o Organization
			decodeStrict(t, "organization.json", &o)
			if o.ID == "" {
				t.Fatal("organization incomplete")
			}
		},
		"plans.json": func(t *testing.T) {
			p := decodeStrictPage[Plan](t, "plans.json")
			if len(p.Data) == 0 || p.Data[0].ID == "" {
				t.Fatal("no plans decoded")
			}
		},
		"presets.json": func(t *testing.T) {
			p := decodeStrictPage[Preset](t, "presets.json")
			if len(p.Data) == 0 || p.Data[0].Slug == "" {
				t.Fatal("no presets decoded")
			}
		},
		"statements.json": func(t *testing.T) {
			p := decodeStrictPage[Statement](t, "statements.json")
			if len(p.Data) == 0 {
				t.Fatal("no statements decoded")
			}
		},
		"stats.json": func(t *testing.T) {
			var s Stats
			decodeStrict(t, "stats.json", &s)
			if len(s.Daily) == 0 {
				t.Fatal("stats incomplete")
			}
		},
		"status.json": func(t *testing.T) {
			var s Status
			decodeStrict(t, "status.json", &s)
			if s.Status != "operational" {
				t.Fatalf("status = %q", s.Status)
			}
		},
		"transactions.json": func(t *testing.T) {
			p := decodeStrictPage[CreditTransaction](t, "transactions.json")
			if len(p.Data) == 0 {
				t.Fatal("no transactions decoded")
			}
		},
		"usage.json": func(t *testing.T) {
			var u Usage
			decodeStrict(t, "usage.json", &u)
			if len(u.ByCodec) == 0 {
				t.Fatal("usage incomplete")
			}
		},
		"webhooks.json": func(t *testing.T) {
			p := decodeStrictPage[WebhookEndpoint](t, "webhooks.json")
			if len(p.Data) == 0 || p.Data[0].Type == "" {
				t.Fatal("no endpoints decoded")
			}
		},
	}
	if len(cases) != 22 {
		t.Fatalf("expected a case per fixture, have %d", len(cases))
	}
	for name, run := range cases {
		t.Run(name, run)
	}
}
