package transcdr

import (
	"encoding/json"
	"time"
)

// AdminOverview is the operator console's summary.
type AdminOverview struct {
	Organizations int64 `json:"organizations"`
	// JobsByStatus counts live jobs by status.
	JobsByStatus map[JobStatus]int64 `json:"jobs_by_status"`
	raw          json.RawMessage
}

// Raw is the whole response, including the processing pool's state.
func (o *AdminOverview) Raw() json.RawMessage { return o.raw }

// UnmarshalJSON decodes and keeps the raw response.
func (o *AdminOverview) UnmarshalJSON(b []byte) error {
	type plain AdminOverview
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*o = AdminOverview(p)
	o.raw = append(json.RawMessage(nil), b...)
	return nil
}

// AdminJob is a job as the operator console sees it.
type AdminJob struct {
	Job
	// Organization is the organization's id.
	Organization json.RawMessage `json:"organization"`
	// Internals are operator-only processing details, as returned.
	Internals json.RawMessage `json:"internals"`
}

// AdminJobListParams filter the console's job list.
type AdminJobListParams struct {
	ListParams
	Status JobStatus
}

// AdminOrganizationUpdateParams change an organization's plan or suspend it.
type AdminOrganizationUpdateParams struct {
	Plan      string `json:"plan,omitempty"`
	Suspended *bool  `json:"suspended,omitempty"`
}

// AdminCreditParams grant (or, negative, remove) credit.
type AdminCreditParams struct {
	// AmountCents is non-zero, at most $100,000 either way.
	AmountCents int64 `json:"amount_cents"`
	// Description appears on the statement; default "Credit adjustment".
	Description string `json:"description,omitempty"`
	// ExpiresInDays makes a positive grant promotional credit that expires.
	ExpiresInDays *int `json:"expires_in_days,omitempty"`
}

// AdminAnnouncementParams create or update a changelog entry.
type AdminAnnouncementParams struct {
	Title string `json:"title,omitempty"`
	// Body is Markdown.
	Body string                     `json:"body,omitempty"`
	Link Nullable[AnnouncementLink] `json:"link,omitzero"`
	Tags []string                   `json:"tags,omitempty"`
	// PublishedAt defaults to now on create; Null saves (or takes it back
	// to) a draft; a future time schedules it.
	PublishedAt Nullable[time.Time] `json:"published_at,omitzero"`
}

// IncidentDetector finds the jobs a known defect affected.
type IncidentDetector struct {
	Name    string          `json:"name"`
	Summary string          `json:"summary"`
	Filters json.RawMessage `json:"filters"`
}

// Incident is a service incident whose affected jobs are credited.
type Incident struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Detector    *string         `json:"detector"`
	Filters     json.RawMessage `json:"filters"`
	WindowStart time.Time       `json:"window_start"`
	WindowEnd   time.Time       `json:"window_end"`
	Multiplier  int             `json:"multiplier"`
	// Status is draft, previewed or applied.
	Status                string     `json:"status"`
	AffectedJobs          int64      `json:"affected_jobs"`
	AffectedOrganizations int64      `json:"affected_organizations"`
	ReviewJobs            int64      `json:"review_jobs"`
	CreditUSD             float64    `json:"credit_usd"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	AppliedAt             *time.Time `json:"applied_at"`
	// Impacts are the affected jobs, on retrieve, preview and apply.
	Impacts []json.RawMessage `json:"impacts,omitempty"`
}

// IncidentCreateParams open a draft incident; nothing is credited until it is
// previewed and applied.
type IncidentCreateParams struct {
	Title string `json:"title"`
	// Description is what customers are told.
	Description string     `json:"description"`
	WindowStart time.Time  `json:"window_start"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	// Detector is one of [AdminIncidentsService.Detectors].
	Detector string          `json:"detector,omitempty"`
	Filters  json.RawMessage `json:"filters,omitempty"`
	// Multiplier is 1–20.
	Multiplier *int `json:"multiplier,omitempty"`
}
