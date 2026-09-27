package transcdr

import (
	"context"
	"encoding/json"
	"iter"
)

// WebhooksService manages event destinations: HTTPS webhooks, Amazon SNS
// topics and Amazon SQS queues, or messaging connections.
type WebhooksService struct{ client *Client }

// List returns one page of endpoints.
func (s *WebhooksService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[WebhookEndpoint], error) {
	return getPage[WebhookEndpoint](ctx, s.client, "/v1/webhooks", params.values(nil), opts)
}

// All iterates over every endpoint.
func (s *WebhooksService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[WebhookEndpoint, error] {
	return iterate[WebhookEndpoint](ctx, s.client, "/v1/webhooks", params.values(nil), opts)
}

// Create creates an endpoint. Its Secret is returned now and on rotation only.
func (s *WebhooksService) Create(ctx context.Context, params *WebhookCreateParams, opts ...RequestOption) (*WebhookEndpoint, error) {
	return doJSONBody[WebhookEndpoint](ctx, s.client, "POST", "/v1/webhooks", params, opts)
}

// Get retrieves an endpoint.
func (s *WebhooksService) Get(ctx context.Context, id string, opts ...RequestOption) (*WebhookEndpoint, error) {
	return doJSON[WebhookEndpoint](ctx, s.client, "GET", "/v1/webhooks/"+seg(id), nil, opts)
}

// Update changes an endpoint; its type cannot change.
func (s *WebhooksService) Update(ctx context.Context, id string, params *WebhookUpdateParams, opts ...RequestOption) (*WebhookEndpoint, error) {
	return doJSONBody[WebhookEndpoint](ctx, s.client, "PATCH", "/v1/webhooks/"+seg(id), params, opts)
}

// Delete deletes an endpoint.
func (s *WebhooksService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/webhooks/"+seg(id), nil, nil, nil, opts)
}

// RotateSecret issues a new signing secret; the old one stops working.
func (s *WebhooksService) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (*WebhookEndpoint, error) {
	return doJSON[WebhookEndpoint](ctx, s.client, "POST", "/v1/webhooks/"+seg(id)+"/rotate-secret", nil, opts)
}

// Test sends a webhook.test event to the endpoint and waits for the answer.
// The result is the delivery (usually) or the event, as JSON.
func (s *WebhooksService) Test(ctx context.Context, id string, opts ...RequestOption) (json.RawMessage, error) {
	var out json.RawMessage
	if err := s.client.do(ctx, "POST", "/v1/webhooks/"+seg(id)+"/test", nil, nil, &out, opts); err != nil {
		return nil, err
	}
	return out, nil
}

// Check verifies a destination without saving it: it checks the credentials
// and sends a webhook.test signed with a throwaway secret.
func (s *WebhooksService) Check(ctx context.Context, params *WebhookCreateParams, opts ...RequestOption) (*WebhookCheck, error) {
	return doJSONBody[WebhookCheck](ctx, s.client, "POST", "/v1/webhooks/check", params, opts)
}

// CheckSaved verifies a saved endpoint.
func (s *WebhooksService) CheckSaved(ctx context.Context, id string, opts ...RequestOption) (*WebhookCheck, error) {
	return doJSON[WebhookCheck](ctx, s.client, "POST", "/v1/webhooks/"+seg(id)+"/check", nil, opts)
}

// Deliveries returns one page of the endpoint's deliveries.
func (s *WebhooksService) Deliveries(ctx context.Context, id string, params *ListParams, opts ...RequestOption) (*Page[WebhookDelivery], error) {
	return getPage[WebhookDelivery](ctx, s.client, "/v1/webhooks/"+seg(id)+"/deliveries", params.values(nil), opts)
}

// AllDeliveries iterates over every delivery to the endpoint.
func (s *WebhooksService) AllDeliveries(ctx context.Context, id string, params *ListParams, opts ...RequestOption) iter.Seq2[WebhookDelivery, error] {
	return iterate[WebhookDelivery](ctx, s.client, "/v1/webhooks/"+seg(id)+"/deliveries", params.values(nil), opts)
}

// Redeliver queues another attempt of a delivery.
func (s *WebhooksService) Redeliver(ctx context.Context, deliveryID string, opts ...RequestOption) (*WebhookDelivery, error) {
	return doJSON[WebhookDelivery](ctx, s.client, "POST", "/v1/webhook-deliveries/"+seg(deliveryID)+"/redeliver", nil, opts)
}

// VerifySignature is [VerifySignature].
func (s *WebhooksService) VerifySignature(payload []byte, header, secret string, opts ...VerifyOption) bool {
	return VerifySignature(payload, header, secret, opts...)
}

// VerifySNSSQSSignature is [VerifySNSSQSSignature].
func (s *WebhooksService) VerifySNSSQSSignature(message []byte, attributes any, secret string, opts ...VerifyOption) bool {
	return VerifySNSSQSSignature(message, attributes, secret, opts...)
}

// ConstructEvent is [ConstructEvent].
func (s *WebhooksService) ConstructEvent(payload []byte, header, secret string, opts ...VerifyOption) (*Event, error) {
	return ConstructEvent(payload, header, secret, opts...)
}
