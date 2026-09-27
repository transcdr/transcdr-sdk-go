package transcdr

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const incidentBody = `{"object":"incident","id":"inc_1","title":"Silent audio","description":"Some outputs had no audio.","detector":"silent_audio",
	"filters":{"statuses":["completed"]},"window_start":"2026-09-20T00:00:00Z","window_end":"2026-09-21T00:00:00Z","multiplier":3,"status":"draft",
	"affected_jobs":2,"affected_organizations":1,"review_jobs":0,"credit_usd":0.12,"created_by":"usr_1","created_at":"2026-09-27T10:00:00Z","applied_at":null,
	"impacts":[{"job_id":"job_1"}]}`

const announcementBody = `{"object":"announcement","id":"ann_1","kind":"changelog","title":"Constant bit rate","body":"CBR is here.","published_at":null,
	"link":{"label":"Read","url":"/docs/cbr"},"tags":["quality"],"credit":null,"seen":false,"seen_at":null}`

func TestAdmin(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"admin_overview","organizations":42,"jobs_by_status":{"queued":1,"running":2},"pool":{"ready":1}}`)
	o, err := c.Admin.Overview(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/admin/overview")
	if o.Organizations != 42 || o.JobsByStatus[JobRunning] != 2 || !strings.Contains(string(o.Raw()), `"pool"`) {
		t.Fatalf("overview = %+v", o)
	}

	job := fixture(t, "job.json")
	var jobMap map[string]any
	must(t, json.Unmarshal([]byte(job), &jobMap))
	jobMap["organization"] = "org_1"
	jobMap["internals"] = map[string]any{"raw_error": nil}
	adminJob, _ := json.Marshal(jobMap)
	f.reply(200, `{"object":"list","data":[`+string(adminJob)+`],"has_more":false,"next_cursor":null}`)
	jobs, err := c.Admin.Jobs(fxCtx, &AdminJobListParams{Status: JobFailed, ListParams: ListParams{Limit: 10}})
	must(t, err)
	r := f.expect("GET", "/v1/admin/jobs")
	if r.Query["status"][0] != "failed" || r.Query["limit"][0] != "10" {
		t.Fatalf("query = %v", r.Query)
	}
	if jobs.Data[0].ID == "" || string(jobs.Data[0].Organization) != `"org_1"` || len(jobs.Data[0].Internals) == 0 {
		t.Fatalf("admin job = %+v", jobs.Data[0])
	}

	f.reply(200, `{"object":"list","data":[`+fixture(t, "organization.json")+`],"has_more":false,"next_cursor":null}`)
	orgs, err := c.Admin.Organizations(fxCtx, &ListParams{Limit: 100})
	must(t, err)
	if r := f.expect("GET", "/v1/admin/organizations"); r.Query["limit"][0] != "100" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(orgs.Data) != 1 {
		t.Fatal("no organizations")
	}

	f.reply(200, fixture(t, "organization.json"))
	_, err = c.Admin.UpdateOrganization(fxCtx, "org_1", &AdminOrganizationUpdateParams{Plan: "growth", Suspended: Bool(false)})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/admin/organizations/org_1"), map[string]any{"plan": "growth", "suspended": false})

	f.reply(200, fixture(t, "billing.json"))
	_, err = c.Admin.GrantCredit(fxCtx, "org_1", &AdminCreditParams{AmountCents: 1000, Description: "Goodwill", ExpiresInDays: Int(30)})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/admin/organizations/org_1/credit"), map[string]any{"amount_cents": float64(1000), "description": "Goodwill", "expires_in_days": float64(30)})
}

func TestAdminAnnouncements(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"list","data":[`+announcementBody+`],"has_more":false,"next_cursor":null}`)
	list, err := c.Admin.Announcements.List(fxCtx, nil)
	must(t, err)
	f.expect("GET", "/v1/admin/announcements")
	if list.Data[0].PublishedAt != nil {
		t.Fatal("a draft has no published_at")
	}

	f.reply(201, announcementBody)
	_, err = c.Admin.Announcements.Create(fxCtx, &AdminAnnouncementParams{
		Title: "Constant bit rate", Body: "CBR is here.", Tags: []string{"quality"},
		Link: Value(AnnouncementLink{Label: "Read", URL: "/docs/cbr"}), PublishedAt: Null[time.Time](),
	})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/admin/announcements"), map[string]any{
		"title": "Constant bit rate", "body": "CBR is here.", "tags": []any{"quality"},
		"link": map[string]any{"label": "Read", "url": "/docs/cbr"}, "published_at": nil,
	})

	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	f.reply(200, announcementBody)
	_, err = c.Admin.Announcements.Update(fxCtx, "ann_1", &AdminAnnouncementParams{PublishedAt: Value(at), Link: Null[AnnouncementLink]()})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/admin/announcements/ann_1"), map[string]any{"published_at": "2026-10-01T09:00:00Z", "link": nil})

	f.reply(204, "")
	must(t, c.Admin.Announcements.Delete(fxCtx, "ann_1"))
	f.expect("DELETE", "/v1/admin/announcements/ann_1")
}

func TestAdminIncidents(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"list","data":[{"name":"silent_audio","summary":"Outputs without audio","filters":{"input_has_audio":true}}],"has_more":false,"next_cursor":null}`)
	det, err := c.Admin.Incidents.Detectors(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/admin/incident-detectors")
	if len(det) != 1 || det[0].Name != "silent_audio" || !strings.Contains(string(det[0].Filters), "input_has_audio") {
		t.Fatalf("detectors = %+v", det)
	}

	f.reply(200, `{"object":"list","data":[`+incidentBody+`],"has_more":false,"next_cursor":null}`)
	list, err := c.Admin.Incidents.List(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/admin/incidents")
	if len(list) != 1 || list[0].Multiplier != 3 {
		t.Fatalf("incidents = %+v", list)
	}

	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	f.reply(201, incidentBody)
	inc, err := c.Admin.Incidents.Create(fxCtx, &IncidentCreateParams{
		Title: "Silent audio", Description: "Some outputs had no audio.", WindowStart: start,
		Detector: "silent_audio", Filters: json.RawMessage(`{"statuses":["completed"]}`), Multiplier: Int(3),
	})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/admin/incidents"), map[string]any{
		"title": "Silent audio", "description": "Some outputs had no audio.", "window_start": "2026-09-20T00:00:00Z",
		"detector": "silent_audio", "filters": map[string]any{"statuses": []any{"completed"}}, "multiplier": float64(3),
	})
	if inc.ID != "inc_1" || inc.Detector == nil || len(inc.Impacts) != 1 {
		t.Fatalf("incident = %+v", inc)
	}

	f.reply(200, incidentBody)
	_, err = c.Admin.Incidents.Get(fxCtx, "inc_1")
	must(t, err)
	f.expect("GET", "/v1/admin/incidents/inc_1")

	f.reply(200, incidentBody)
	_, err = c.Admin.Incidents.Preview(fxCtx, "inc_1")
	must(t, err)
	f.expect("POST", "/v1/admin/incidents/inc_1/preview")

	f.reply(200, incidentBody)
	_, err = c.Admin.Incidents.Apply(fxCtx, "inc_1", true)
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/admin/incidents/inc_1/apply"), map[string]any{"include_review": true})
}
