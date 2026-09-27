package transcdr

import (
	"context"
	"iter"
	"net/url"
)

// ConnectionsService manages storage connections (where inputs come from and
// outputs go) and messaging connections (sqs, sns and webhook: they receive
// events, and an sqs queue can trigger automations).
type ConnectionsService struct{ client *Client }

// List returns one page of connections.
func (s *ConnectionsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Connection], error) {
	return getPage[Connection](ctx, s.client, "/v1/connections", params.values(nil), opts)
}

// All iterates over every connection.
func (s *ConnectionsService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Connection, error] {
	return iterate[Connection](ctx, s.client, "/v1/connections", params.values(nil), opts)
}

// Create creates a connection. It is tested when saved; the outcome is in
// Status and LastError.
func (s *ConnectionsService) Create(ctx context.Context, params *ConnectionCreateParams, opts ...RequestOption) (*Connection, error) {
	return create[Connection](ctx, s.client, "/v1/connections", params, opts)
}

// Get retrieves a connection.
func (s *ConnectionsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Connection, error) {
	return doJSON[Connection](ctx, s.client, "GET", "/v1/connections/"+seg(id), nil, opts)
}

// Update changes a connection: the config merges, an omitted secret is kept
// and "" clears it.
func (s *ConnectionsService) Update(ctx context.Context, id string, params *ConnectionUpdateParams, opts ...RequestOption) (*Connection, error) {
	return doJSONBody[Connection](ctx, s.client, "PATCH", "/v1/connections/"+seg(id), params, opts)
}

// Enable turns a disabled connection back on: the failure count resets and
// it is tested again.
func (s *ConnectionsService) Enable(ctx context.Context, id string, opts ...RequestOption) (*Connection, error) {
	return s.Update(ctx, id, &ConnectionUpdateParams{Enabled: Bool(true)}, opts...)
}

// Disable turns a connection off by hand; anything using it is refused with
// 409 connection_disabled until it is back on.
func (s *ConnectionsService) Disable(ctx context.Context, id string, opts ...RequestOption) (*Connection, error) {
	return s.Update(ctx, id, &ConnectionUpdateParams{Enabled: Bool(false)}, opts...)
}

// Delete deletes a connection. It is refused (409 connection_in_use) while an
// automation uses it.
func (s *ConnectionsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/connections/"+seg(id), nil, nil, nil, opts)
}

// Test checks the credentials and reachability again.
func (s *ConnectionsService) Test(ctx context.Context, id string, opts ...RequestOption) (*ConnectionTestResult, error) {
	return doJSON[ConnectionTestResult](ctx, s.client, "POST", "/v1/connections/"+seg(id)+"/test", nil, opts)
}

// Check verifies settings without saving them: it signs in, lists, writes a
// probe object, reads it back and deletes it, and reports each step.
func (s *ConnectionsService) Check(ctx context.Context, params *ConnectionCheckParams, opts ...RequestOption) (*ConnectionCheck, error) {
	return doJSONBody[ConnectionCheck](ctx, s.client, "POST", "/v1/connections/check", params, opts)
}

// CheckSaved verifies a saved connection and updates its status.
func (s *ConnectionsService) CheckSaved(ctx context.Context, id string, opts ...RequestOption) (*ConnectionCheck, error) {
	return doJSON[ConnectionCheck](ctx, s.client, "POST", "/v1/connections/"+seg(id)+"/check", nil, opts)
}

// Browse lists files under a prefix (up to 1,000; HasMore means the listing
// was cut short).
func (s *ConnectionsService) Browse(ctx context.Context, id string, params *BrowseParams, opts ...RequestOption) (*Page[RemoteObject], error) {
	q := url.Values{}
	if params != nil {
		if params.Prefix != "" {
			q.Set("prefix", params.Prefix)
		}
		if params.Recursive {
			q.Set("recursive", "true")
		}
	}
	return getPage[RemoteObject](ctx, s.client, "/v1/connections/"+seg(id)+"/browse", q, opts)
}

// AutomationsService manages automations: when a file lands in a
// connection, transcode it like this and deliver it there.
type AutomationsService struct{ client *Client }

// List returns one page of automations.
func (s *AutomationsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Automation], error) {
	return getPage[Automation](ctx, s.client, "/v1/automations", params.values(nil), opts)
}

// All iterates over every automation.
func (s *AutomationsService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Automation, error] {
	return iterate[Automation](ctx, s.client, "/v1/automations", params.values(nil), opts)
}

// Create creates an automation.
func (s *AutomationsService) Create(ctx context.Context, params *AutomationParams, opts ...RequestOption) (*Automation, error) {
	return create[Automation](ctx, s.client, "/v1/automations", params, opts)
}

// Get retrieves an automation.
func (s *AutomationsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Automation, error) {
	return doJSON[Automation](ctx, s.client, "GET", "/v1/automations/"+seg(id), nil, opts)
}

// Update changes the fields set in params.
func (s *AutomationsService) Update(ctx context.Context, id string, params *AutomationParams, opts ...RequestOption) (*Automation, error) {
	return doJSONBody[Automation](ctx, s.client, "PATCH", "/v1/automations/"+seg(id), params, opts)
}

// Delete deletes an automation.
func (s *AutomationsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/automations/"+seg(id), nil, nil, nil, opts)
}

// Run polls a watch automation's source now, or reads one batch of a queue
// automation's queue.
func (s *AutomationsService) Run(ctx context.Context, id string, opts ...RequestOption) (*AutomationRun, error) {
	return doJSON[AutomationRun](ctx, s.client, "POST", "/v1/automations/"+seg(id)+"/run", nil, opts)
}

// Trigger processes specific paths now, as a hook push would.
func (s *AutomationsService) Trigger(ctx context.Context, id string, params *AutomationTriggerParams, opts ...RequestOption) (*AutomationRun, error) {
	return doJSONBody[AutomationRun](ctx, s.client, "POST", "/v1/automations/"+seg(id)+"/trigger", params, opts)
}

// RotateHookToken issues a new hook URL; the old one stops working.
func (s *AutomationsService) RotateHookToken(ctx context.Context, id string, opts ...RequestOption) (*Automation, error) {
	return doJSON[Automation](ctx, s.client, "POST", "/v1/automations/"+seg(id)+"/rotate-hook-token", nil, opts)
}

// Items returns source objects the automation has processed, newest first.
func (s *AutomationsService) Items(ctx context.Context, id string, params *ListParams, opts ...RequestOption) (*Page[AutomationItem], error) {
	return getPage[AutomationItem](ctx, s.client, "/v1/automations/"+seg(id)+"/items", params.values(nil), opts)
}

// PushHook posts a payload to an automation's hook URL, as a sender would:
// {"path": …}, {"paths": [...]}, or an S3-style notification. No key is sent:
// the URL is the credential.
func (s *AutomationsService) PushHook(ctx context.Context, hookURL string, payload any, opts ...RequestOption) (*AutomationRun, error) {
	return doJSONBody[AutomationRun](ctx, s.client, "POST", hookURL, payload, append([]RequestOption{withoutAuth()}, opts...))
}

// DeliveriesService retries deliveries of outputs to connections.
type DeliveriesService struct{ client *Client }

// Retry runs a failed (or finished) delivery again.
func (s *DeliveriesService) Retry(ctx context.Context, id string, opts ...RequestOption) (*Delivery, error) {
	return doJSON[Delivery](ctx, s.client, "POST", "/v1/deliveries/"+seg(id)+"/retry", nil, opts)
}
