package transcdr

import (
	"context"
	"net/url"
)

// AdminService is the platform operator console. It answers 404 to anyone
// who is not an operator signed in with a session.
type AdminService struct {
	client        *Client
	Announcements *AdminAnnouncementsService
	Incidents     *AdminIncidentsService
}

// Overview returns the console's summary.
func (s *AdminService) Overview(ctx context.Context, opts ...RequestOption) (*AdminOverview, error) {
	return doJSON[AdminOverview](ctx, s.client, "GET", "/v1/admin/overview", nil, opts)
}

// Jobs returns recent jobs across every organization.
func (s *AdminService) Jobs(ctx context.Context, params *AdminJobListParams, opts ...RequestOption) (*Page[AdminJob], error) {
	q := url.Values{}
	if params != nil {
		q = params.ListParams.values(nil)
		if params.Status != "" {
			q.Set("status", string(params.Status))
		}
	}
	return getPage[AdminJob](ctx, s.client, "/v1/admin/jobs", q, opts)
}

// Organizations returns organizations.
func (s *AdminService) Organizations(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Organization], error) {
	return getPage[Organization](ctx, s.client, "/v1/admin/organizations", params.values(nil), opts)
}

// UpdateOrganization changes an organization's plan or suspends it.
func (s *AdminService) UpdateOrganization(ctx context.Context, id string, params *AdminOrganizationUpdateParams, opts ...RequestOption) (*Organization, error) {
	return doJSONBody[Organization](ctx, s.client, "PATCH", "/v1/admin/organizations/"+seg(id), params, opts)
}

// GrantCredit adds (or removes) credit and returns the organization's billing
// summary.
func (s *AdminService) GrantCredit(ctx context.Context, organizationID string, params *AdminCreditParams, opts ...RequestOption) (*Billing, error) {
	return doJSONBody[Billing](ctx, s.client, "POST", "/v1/admin/organizations/"+seg(organizationID)+"/credit", params, opts)
}

// AdminAnnouncementsService writes the changelog.
type AdminAnnouncementsService struct{ client *Client }

// List returns every announcement, including drafts and service credits.
func (s *AdminAnnouncementsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Announcement], error) {
	return getPage[Announcement](ctx, s.client, "/v1/admin/announcements", params.values(nil), opts)
}

// Create creates a changelog entry: published now by default, a draft with
// PublishedAt Null, or scheduled for a future time.
func (s *AdminAnnouncementsService) Create(ctx context.Context, params *AdminAnnouncementParams, opts ...RequestOption) (*Announcement, error) {
	return doJSONBody[Announcement](ctx, s.client, "POST", "/v1/admin/announcements", params, opts)
}

// Update changes a changelog entry.
func (s *AdminAnnouncementsService) Update(ctx context.Context, id string, params *AdminAnnouncementParams, opts ...RequestOption) (*Announcement, error) {
	return doJSONBody[Announcement](ctx, s.client, "PATCH", "/v1/admin/announcements/"+seg(id), params, opts)
}

// Delete deletes a changelog entry.
func (s *AdminAnnouncementsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/admin/announcements/"+seg(id), nil, nil, nil, opts)
}

// AdminIncidentsService finds the jobs an incident affected and credits them.
type AdminIncidentsService struct{ client *Client }

// Detectors lists the known-defect detectors an incident can use.
func (s *AdminIncidentsService) Detectors(ctx context.Context, opts ...RequestOption) ([]IncidentDetector, error) {
	return getAll[IncidentDetector](ctx, s.client, "/v1/admin/incident-detectors", nil, opts)
}

// List returns the latest 100 incidents.
func (s *AdminIncidentsService) List(ctx context.Context, opts ...RequestOption) ([]Incident, error) {
	return getAll[Incident](ctx, s.client, "/v1/admin/incidents", nil, opts)
}

// Create opens a draft incident.
func (s *AdminIncidentsService) Create(ctx context.Context, params *IncidentCreateParams, opts ...RequestOption) (*Incident, error) {
	return doJSONBody[Incident](ctx, s.client, "POST", "/v1/admin/incidents", params, opts)
}

// Get retrieves an incident with every impact.
func (s *AdminIncidentsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Incident, error) {
	return doJSON[Incident](ctx, s.client, "GET", "/v1/admin/incidents/"+seg(id), nil, opts)
}

// Preview finds the affected jobs; nothing is credited.
func (s *AdminIncidentsService) Preview(ctx context.Context, id string, opts ...RequestOption) (*Incident, error) {
	return doJSON[Incident](ctx, s.client, "POST", "/v1/admin/incidents/"+seg(id)+"/preview", nil, opts)
}

// Apply credits each affected organization once. With includeReview, jobs
// flagged for review are credited too.
func (s *AdminIncidentsService) Apply(ctx context.Context, id string, includeReview bool, opts ...RequestOption) (*Incident, error) {
	return doJSONBody[Incident](ctx, s.client, "POST", "/v1/admin/incidents/"+seg(id)+"/apply", map[string]bool{"include_review": includeReview}, opts)
}
