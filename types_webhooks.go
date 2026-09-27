package transcdr

import (
	"encoding/json"
	"time"
)

// Event types.
const (
	EventJobCreated          = "job.created"
	EventJobScheduled        = "job.scheduled"
	EventJobStarted          = "job.started"
	EventJobCompleted        = "job.completed"
	EventJobFailed           = "job.failed"
	EventJobCanceled         = "job.canceled"
	EventAssetReady          = "asset.ready"
	EventAssetDeleted        = "asset.deleted"
	EventJobDelivered        = "job.delivered"
	EventJobDeliveryFailed   = "job.delivery_failed"
	EventAutomationTriggered = "automation.triggered"
	EventConnectionDisabled  = "connection.disabled"
	EventWebhookTest         = "webhook.test"
)

// EventTypes lists every event type.
var EventTypes = []string{
	EventJobCreated, EventJobScheduled, EventJobStarted, EventJobCompleted, EventJobFailed, EventJobCanceled,
	EventAssetReady, EventAssetDeleted, EventJobDelivered, EventJobDeliveryFailed, EventAutomationTriggered,
	EventConnectionDisabled, EventWebhookTest,
}

// Event is a webhook payload, and an entry of GET /v1/events.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Data      EventData `json:"data"`
	// Livemode is false for events of test-mode jobs.
	Livemode *bool `json:"livemode,omitempty"`
}

// EventData holds the event's object: a Job, an Asset, or another object
// depending on Type. Decode it with [EventData.Job], [EventData.Asset],
// [EventData.Connection] or [EventData.Decode].
type EventData struct {
	Object json.RawMessage `json:"object"`
}

// Decode unmarshals the event's object into v.
func (d EventData) Decode(v any) error { return json.Unmarshal(d.Object, v) }

// Job is the object of job.* events.
func (d EventData) Job() (*Job, error) {
	var j Job
	return &j, d.Decode(&j)
}

// Asset is the object of asset.* events.
func (d EventData) Asset() (*Asset, error) {
	var a Asset
	return &a, d.Decode(&a)
}

// Connection is the object of connection.disabled events.
func (d EventData) Connection() (*ConnectionDisabledEvent, error) {
	var c ConnectionDisabledEvent
	return &c, d.Decode(&c)
}

// ConnectionDisabledEvent is the object of a connection.disabled event.
type ConnectionDisabledEvent struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Enabled    bool      `json:"enabled"`
	DisabledAt time.Time `json:"disabled_at"`
	// DisabledReason is "<activity>: <error>".
	DisabledReason string `json:"disabled_reason"`
	// Activity is what Transcdr was doing, e.g. reading a queue.
	Activity string `json:"activity"`
	// Error is the provider's error.
	Error string `json:"error"`
	// Explanation is what it most likely means and how to fix it.
	Explanation string `json:"explanation"`
	// Automations are the automations using the connection; they pause.
	Automations []string `json:"automations"`
}

// EventListParams filter events.
type EventListParams struct {
	ListParams
	Type string
}

// Event destination types.
const (
	WebhookHTTPS = "https"
	WebhookSNS   = "sns"
	WebhookSQS   = "sqs"
)

// WebhookAWS is the AWS side of an sns or sqs destination, as returned. The
// secret access key is never returned.
type WebhookAWS struct {
	Region      string `json:"region"`
	AccessKeyID string `json:"access_key_id"`
	// Endpoint is an SNS-compatible service endpoint, when not AWS itself.
	Endpoint *string `json:"endpoint"`
	// MessageGroupID is for FIFO topics and queues.
	MessageGroupID     *string `json:"message_group_id"`
	SecretAccessKeySet bool    `json:"secret_access_key_set"`
}

// WebhookEndpoint is an event destination: an HTTPS URL, an SNS topic or an
// SQS queue, or a messaging connection.
type WebhookEndpoint struct {
	ID string `json:"id"`
	// Type is https, sns or sqs.
	Type string `json:"type"`
	// URL is the HTTPS URL; for sns and sqs the topic ARN or queue URL.
	URL         string      `json:"url"`
	TopicARN    *string     `json:"topic_arn"`
	QueueURL    *string     `json:"queue_url"`
	AWS         *WebhookAWS `json:"aws"`
	Description string      `json:"description"`
	// Events are event types, or ["*"].
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
	// Secret signs deliveries; returned only on create and rotate.
	Secret         *string    `json:"secret,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
	FailureCount   int        `json:"failure_count"`
	// ConnectionID is the messaging connection events go through, if any.
	ConnectionID *string `json:"connection_id"`
}

// WebhookAWSParams are the AWS settings of an sns or sqs destination. On
// create AccessKeyID and SecretAccessKey are required; on update an omitted
// secret keeps the stored one.
type WebhookAWSParams struct {
	AccessKeyID     *string `json:"access_key_id,omitempty"`
	SecretAccessKey *string `json:"secret_access_key,omitempty"`
	// Region is read from the topic ARN or queue URL when omitted.
	Region *string `json:"region,omitempty"`
	// Endpoint is an SNS-compatible service endpoint.
	Endpoint Nullable[string] `json:"endpoint,omitzero"`
	// MessageGroupID is for FIFO targets; the API's default is "transcdr".
	MessageGroupID Nullable[string] `json:"message_group_id,omitzero"`
}

// WebhookCreateParams create an event destination: set URL (https),
// TopicARN and AWS (sns), QueueURL and AWS (sqs), or ConnectionID.
type WebhookCreateParams struct {
	// Type is https, sns or sqs; read from the target when omitted.
	Type         string            `json:"type,omitempty"`
	URL          string            `json:"url,omitempty"`
	TopicARN     string            `json:"topic_arn,omitempty"`
	QueueURL     string            `json:"queue_url,omitempty"`
	AWS          *WebhookAWSParams `json:"aws,omitempty"`
	ConnectionID string            `json:"connection_id,omitempty"`
	// Events are event types, or ["*"] (the default).
	Events      []string `json:"events,omitempty"`
	Description string   `json:"description,omitempty"`
}

// WebhookUpdateParams change an event destination. The type cannot change.
type WebhookUpdateParams struct {
	URL         *string           `json:"url,omitempty"`
	TopicARN    *string           `json:"topic_arn,omitempty"`
	QueueURL    *string           `json:"queue_url,omitempty"`
	AWS         *WebhookAWSParams `json:"aws,omitempty"`
	Events      []string          `json:"events,omitempty"`
	Description *string           `json:"description,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

// WebhookDelivery is one attempt to deliver an event to an endpoint.
type WebhookDelivery struct {
	ID         string `json:"id"`
	EndpointID string `json:"endpoint_id"`
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	// Status is pending, succeeded or failed.
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	ResponseStatus *int       `json:"response_status"`
	ResponseBody   *string    `json:"response_body"`
	DurationMs     *int64     `json:"duration_ms"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// CheckStep is one step of a live check.
type CheckStep struct {
	// ID is settings, connect, identity, list, write, read, delete (connections)
	// or identity, deliver, publish, send (destinations).
	ID    string `json:"id"`
	Label string `json:"label"`
	// Status is passed, failed or skipped.
	Status string `json:"status"`
	// Detail is what happened.
	Detail *string `json:"detail,omitempty"`
	// Hint is, on failure, what to change.
	Hint       *string `json:"hint,omitempty"`
	DurationMs *int64  `json:"duration_ms,omitempty"`
}

// CheckIdentity is who the credentials sign in as.
type CheckIdentity struct {
	// Provider is aws, gcp, azure, s3_compatible or sftp.
	Provider       string `json:"provider"`
	ARN            string `json:"arn,omitempty"`
	Account        string `json:"account,omitempty"`
	ServiceAccount string `json:"service_account,omitempty"`
	Project        string `json:"project,omitempty"`
	Auth           string `json:"auth,omitempty"`
	AccessKeyID    string `json:"access_key_id,omitempty"`
	User           string `json:"user,omitempty"`
	Server         string `json:"server,omitempty"`
}

// CheckRoles are the roles a checked connection or destination can serve.
type CheckRoles struct {
	Source        *bool `json:"source,omitempty"`
	WatchFolder   *bool `json:"watch_folder,omitempty"`
	Destination   *bool `json:"destination,omitempty"`
	Trigger       *bool `json:"trigger,omitempty"`
	Notifications *bool `json:"notifications,omitempty"`
}

// ConnectionCheck is a step-by-step report on a connection's settings.
type ConnectionCheck struct {
	// OK is true when every step passed (skipped ones do not count).
	OK       bool           `json:"ok"`
	Steps    []CheckStep    `json:"steps"`
	Identity *CheckIdentity `json:"identity"`
	// Setup is provider-specific setup (IAM policy, queue policies, notes).
	Setup json.RawMessage `json:"setup"`
	Roles CheckRoles      `json:"roles"`
	// Connection is, for saved connections, the connection with its updated
	// status.
	Connection *Connection `json:"connection,omitempty"`
}

// WebhookCheck is a step-by-step report on an event destination.
type WebhookCheck struct {
	OK       bool            `json:"ok"`
	Steps    []CheckStep     `json:"steps"`
	Identity *CheckIdentity  `json:"identity"`
	Setup    json.RawMessage `json:"setup"`
	Roles    CheckRoles      `json:"roles"`
	// Endpoint is, for saved endpoints, the endpoint.
	Endpoint *WebhookEndpoint `json:"endpoint,omitempty"`
}
