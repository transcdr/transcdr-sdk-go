package transcdr

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strconv"
)

// PresetsService manages presets: the system catalogue and the
// organization's own.
type PresetsService struct{ client *Client }

// List returns one page of presets: the organization's, then the system ones.
func (s *PresetsService) List(ctx context.Context, params PresetListQuery, opts ...RequestOption) (*Page[Preset], error) {
	return getPage[Preset](ctx, s.client, "/v1/presets", presetQuery(params), opts)
}

// All iterates over every preset.
func (s *PresetsService) All(ctx context.Context, params PresetListQuery, opts ...RequestOption) iter.Seq2[Preset, error] {
	return iterate[Preset](ctx, s.client, "/v1/presets", presetQuery(params), opts)
}

// Create creates a preset, version 1. Its output is the whole spec: an
// incomplete one returns an [*Error] listing every problem, and nothing is
// sent.
func (s *PresetsService) Create(ctx context.Context, params *PresetCreateParams, opts ...RequestOption) (*Preset, error) {
	if err := checkOutput(params.Output); err != nil {
		return nil, err
	}
	return create[Preset](ctx, s.client, "/v1/presets", params, opts)
}

// Get retrieves a preset, its latest version, by pre_… id or by slug
// (system or custom).
func (s *PresetsService) Get(ctx context.Context, idOrSlug string, opts ...RequestOption) (*Preset, error) {
	return doJSON[Preset](ctx, s.client, "GET", "/v1/presets/"+seg(idOrSlug), nil, opts)
}

// GetVersion retrieves a preset as it was at version n: Version and Output
// are that version's.
func (s *PresetsService) GetVersion(ctx context.Context, idOrSlug string, n int, opts ...RequestOption) (*Preset, error) {
	return doJSON[Preset](ctx, s.client, "GET", "/v1/presets/"+seg(idOrSlug)+"@"+strconv.Itoa(n), nil, opts)
}

// Versions lists every version of a preset, oldest first: each a complete
// spec that never changes.
func (s *PresetsService) Versions(ctx context.Context, idOrSlug string, opts ...RequestOption) ([]PresetVersion, error) {
	return getAll[PresetVersion](ctx, s.client, "/v1/presets/"+seg(idOrSlug)+"/versions", nil, opts)
}

// Update changes the fields set in params (PATCH); output is merged over the
// latest version, so a field left out of it keeps its value, and a changed
// spec is a new version.
func (s *PresetsService) Update(ctx context.Context, id string, params *PresetUpdateParams, opts ...RequestOption) (*Preset, error) {
	return doJSONBody[Preset](ctx, s.client, "PATCH", "/v1/presets/"+seg(id), params, opts)
}

// Replace sets the whole preset (PUT): output is the whole spec, checked
// before sending, and a changed spec is a new version. Description and
// metadata left out are emptied; the slug is kept unless set. PUT is safe
// to retry.
func (s *PresetsService) Replace(ctx context.Context, id string, params *PresetReplaceParams, opts ...RequestOption) (*Preset, error) {
	if err := checkOutput(params.Output); err != nil {
		return nil, err
	}
	return doJSONBody[Preset](ctx, s.client, "PUT", "/v1/presets/"+seg(id), params, opts)
}

// Delete deletes a preset.
func (s *PresetsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/presets/"+seg(id), nil, nil, nil, opts)
}

// EventsService reads the event log.
type EventsService struct{ client *Client }

func (p *EventListParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	q := p.ListParams.values(nil)
	if p.Type != "" {
		q.Set("type", p.Type)
	}
	return q
}

// List returns one page of events, newest first.
func (s *EventsService) List(ctx context.Context, params *EventListParams, opts ...RequestOption) (*Page[Event], error) {
	return getPage[Event](ctx, s.client, "/v1/events", params.values(), opts)
}

// All iterates over every matching event.
func (s *EventsService) All(ctx context.Context, params *EventListParams, opts ...RequestOption) iter.Seq2[Event, error] {
	return iterate[Event](ctx, s.client, "/v1/events", params.values(), opts)
}

// Get retrieves an event.
func (s *EventsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Event, error) {
	return doJSON[Event](ctx, s.client, "GET", "/v1/events/"+seg(id), nil, opts)
}

// UsageService reports usage.
type UsageService struct{ client *Client }

// Get returns usage over a window.
func (s *UsageService) Get(ctx context.Context, params *UsageParams, opts ...RequestOption) (*Usage, error) {
	q := url.Values{}
	if params != nil {
		if params.From != "" {
			q.Set("from", params.From)
		}
		if params.To != "" {
			q.Set("to", params.To)
		}
		if params.Granularity != "" {
			q.Set("granularity", params.Granularity)
		}
	}
	return doJSON[Usage](ctx, s.client, "GET", "/v1/usage", q, opts)
}

// Inputs reports the inputs the range's jobs read, bucketed by duration,
// size and kind (container/codec), for a duration × size chart.
func (s *UsageService) Inputs(ctx context.Context, params *InputReportParams, opts ...RequestOption) (*InputReport, error) {
	q := url.Values{}
	if params != nil {
		if params.From != "" {
			q.Set("from", params.From)
		}
		if params.To != "" {
			q.Set("to", params.To)
		}
	}
	return doJSON[InputReport](ctx, s.client, "GET", "/v1/usage/inputs", q, opts)
}

// BillingService manages the plan, credit and spending controls.
type BillingService struct {
	client *Client
	// Invoices are the monthly statements.
	Invoices *InvoicesService
}

// Get returns the plan, credit balance, spending controls and this month's
// usage.
func (s *BillingService) Get(ctx context.Context, opts ...RequestOption) (*Billing, error) {
	return doJSON[Billing](ctx, s.client, "GET", "/v1/billing", nil, opts)
}

// Checkout subscribes to a plan or buys credit (owner only). Send the
// customer to the returned URL when it is not nil.
func (s *BillingService) Checkout(ctx context.Context, params *CheckoutParams, opts ...RequestOption) (*Checkout, error) {
	return doJSONBody[Checkout](ctx, s.client, "POST", "/v1/billing/checkout", params, opts)
}

// Portal returns a link to the payment portal (owner only).
func (s *BillingService) Portal(ctx context.Context, opts ...RequestOption) (*Portal, error) {
	return doJSONBody[Portal](ctx, s.client, "POST", "/v1/billing/portal", map[string]any{}, opts)
}

// UpdateSettings changes the monthly limit and auto-recharge (owner only) and
// returns the billing summary.
func (s *BillingService) UpdateSettings(ctx context.Context, params *BillingSettingsParams, opts ...RequestOption) (*Billing, error) {
	return doJSONBody[Billing](ctx, s.client, "PUT", "/v1/billing/settings", params, opts)
}

// Transactions returns the credit ledger, newest first.
func (s *BillingService) Transactions(ctx context.Context, params *CreditTransactionListParams, opts ...RequestOption) (*Page[CreditTransaction], error) {
	q := url.Values{}
	if params != nil && params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	return getPage[CreditTransaction](ctx, s.client, "/v1/billing/transactions", q, opts)
}

// ChangePlan moves an existing subscription to another plan (owner only); a
// first subscription goes through Checkout.
func (s *BillingService) ChangePlan(ctx context.Context, plan string, opts ...RequestOption) (*Billing, error) {
	return doJSONBody[Billing](ctx, s.client, "PUT", "/v1/billing/plan", map[string]string{"plan": plan}, opts)
}

// InvoicesService lists monthly statements.
type InvoicesService struct{ client *Client }

// List returns one page of statements.
func (s *InvoicesService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Statement], error) {
	return getPage[Statement](ctx, s.client, "/v1/billing/invoices", params.values(nil), opts)
}

// All iterates over every statement.
func (s *InvoicesService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Statement, error] {
	return iterate[Statement](ctx, s.client, "/v1/billing/invoices", params.values(nil), opts)
}

// PlansService lists plans (public).
type PlansService struct{ client *Client }

// List returns every plan, in display order.
func (s *PlansService) List(ctx context.Context, opts ...RequestOption) ([]Plan, error) {
	return getAll[Plan](ctx, s.client, "/v1/plans", nil, opts)
}

// CapabilitiesService describes the service (public).
type CapabilitiesService struct{ client *Client }

// Get returns the codecs, modes, color options, limits, filters and system
// presets.
func (s *CapabilitiesService) Get(ctx context.Context, opts ...RequestOption) (*Capabilities, error) {
	return doJSON[Capabilities](ctx, s.client, "GET", "/v1/capabilities", nil, opts)
}

// StatusService reports the service status (public).
type StatusService struct{ client *Client }

// Get returns the service status.
func (s *StatusService) Get(ctx context.Context, opts ...RequestOption) (*Status, error) {
	return doJSON[Status](ctx, s.client, "GET", "/v1/status", nil, opts)
}

// StatsService reports public platform-wide counters.
type StatsService struct{ client *Client }

// Get returns the counters (cached for about 30 s by the API).
func (s *StatsService) Get(ctx context.Context, opts ...RequestOption) (*Stats, error) {
	return doJSON[Stats](ctx, s.client, "GET", "/v1/stats", nil, opts)
}

// OpenAPI returns the API's OpenAPI 3.1 document.
func (c *Client) OpenAPI(ctx context.Context, opts ...RequestOption) (json.RawMessage, error) {
	var doc json.RawMessage
	if err := c.do(ctx, "GET", "/v1/openapi.json", nil, nil, &doc, opts); err != nil {
		return nil, err
	}
	return doc, nil
}
