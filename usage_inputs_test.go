package transcdr

import (
	"context"
	"testing"
)

// The report is the one in the TypeScript SDK's usage test.
const inputReportJSON = `{"object":"input_report","from":"2026-09-01","to":"2026-09-30","unmeasured":1,
"kinds":[{"kind":"mp4/h264","container":"mp4","video_codec":"h264","files":2,"size_bytes":22000000,"input_minutes":2.07,"billable_minutes":4.13},
{"kind":"other","container":null,"video_codec":null,"files":1,"size_bytes":10,"input_minutes":0.1,"billable_minutes":0.2}],
"points":[{"kind":"mp4/h264","files":2,"size_bytes":22000000,"input_minutes":2.07,"billable_minutes":4.13,
"mean_duration_seconds":62,"mean_size_bytes":11000000,"duration_range":[56.23,100],"size_range":[10000000,17782794]}]}`

func TestUsageInputs(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, inputReportJSON)
	report, err := f.client().Usage.Inputs(context.Background(), &InputReportParams{From: "2026-09-01", To: "2026-09-30"})
	must(t, err)
	r := f.expect("GET", "/v1/usage/inputs")
	if r.Query["from"][0] != "2026-09-01" || r.Query["to"][0] != "2026-09-30" || len(r.Query) != 2 {
		t.Errorf("query = %v", r.Query)
	}
	if report.Unmeasured != 1 || len(report.Kinds) != 2 || report.Kinds[0].Kind != "mp4/h264" || *report.Kinds[0].VideoCodec != "h264" {
		t.Errorf("kinds = %+v", report.Kinds)
	}
	if report.Kinds[1].Container != nil || report.Kinds[1].Files != 1 {
		t.Errorf("other kind = %+v", report.Kinds[1])
	}
	p := report.Points[0]
	if p.MeanDurationSeconds != 62 || p.MeanSizeBytes != 11000000 || p.DurationRange != [2]float64{56.23, 100} ||
		p.SizeRange[1] != 17782794 || p.Files != 2 || p.BillableMinutes != 4.13 {
		t.Errorf("point = %+v", p)
	}

	// Without a range, the API picks the last 30 days.
	f.reply(200, inputReportJSON)
	_, err = f.client().Usage.Inputs(context.Background(), nil)
	must(t, err)
	if r := f.expect("GET", "/v1/usage/inputs"); len(r.Query) != 0 {
		t.Errorf("query = %v", r.Query)
	}
}
