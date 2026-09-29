package transcdr

import "time"

// Connection kinds. Storage kinds hold files; messaging kinds receive events,
// and an sqs connection can trigger automations.
const (
	KindS3        = "s3"
	KindGCS       = "gcs"
	KindAzureBlob = "azure_blob"
	KindFTP       = "ftp"
	KindFTPS      = "ftps"
	KindSFTP      = "sftp"
	KindHTTP      = "http"
	KindWebDAV    = "webdav"
	KindSQS       = "sqs"
	KindSNS       = "sns"
	KindWebhook   = "webhook"
)

// IsMessagingKind reports whether a kind is a messaging one: never a job
// input, a destination or an automation source.
func IsMessagingKind(kind string) bool {
	return kind == KindSQS || kind == KindSNS || kind == KindWebhook
}

// ConnectionConfig holds non-secret settings. Which fields apply depends on
// the kind. On update the config is merged into the stored one; a Null
// clears a field.
type ConnectionConfig struct {
	// Endpoint is, for s3, a custom endpoint (R2, B2, MinIO…); for http and
	// webdav, the base URL; for sns, an SNS-compatible service.
	Endpoint Nullable[string] `json:"endpoint,omitzero"`
	// Bucket is the s3 or gcs bucket, or the azure_blob container.
	Bucket Nullable[string] `json:"bucket,omitzero"`
	Region Nullable[string] `json:"region,omitzero"`
	// PathStyle addresses an s3 bucket by path rather than by subdomain.
	PathStyle *bool `json:"path_style,omitempty"`
	// Account is the azure_blob storage account.
	Account Nullable[string] `json:"account,omitzero"`
	// Host is the ftp, ftps or sftp host.
	Host     Nullable[string] `json:"host,omitzero"`
	Port     Nullable[int]    `json:"port,omitzero"`
	Username Nullable[string] `json:"username,omitzero"`
	// Root is the folder every path is relative to.
	Root Nullable[string] `json:"root,omitzero"`
	// Passive is ftp/ftps passive mode (default true).
	Passive Nullable[bool] `json:"passive,omitzero"`
	// HostKeyFingerprint pins the sftp host key, "SHA256:…".
	HostKeyFingerprint Nullable[string] `json:"host_key_fingerprint,omitzero"`
	// QueueURL is the sqs queue URL.
	QueueURL Nullable[string] `json:"queue_url,omitzero"`
	// TopicARN is the sns topic ARN.
	TopicARN Nullable[string] `json:"topic_arn,omitzero"`
	// URL is the webhook's https URL.
	URL Nullable[string] `json:"url,omitzero"`
	// MessageGroupID is for FIFO queues and topics receiving events.
	MessageGroupID Nullable[string] `json:"message_group_id,omitzero"`
	// Kind echoes the connection's kind on storage configs; it is ignored when
	// sent.
	Kind string `json:"kind,omitempty"`
}

// ConnectionSecrets are write-only credentials. On update an omitted secret
// is kept and "" clears it (storage connections). The API never returns
// them: [Connection.Secrets] says which are set, with a fingerprint.
type ConnectionSecrets struct {
	AccessKeyID          *string `json:"access_key_id,omitempty"`
	SecretAccessKey      *string `json:"secret_access_key,omitempty"`
	SessionToken         *string `json:"session_token,omitempty"`
	Password             *string `json:"password,omitempty"`
	PrivateKey           *string `json:"private_key,omitempty"`
	PrivateKeyPassphrase *string `json:"private_key_passphrase,omitempty"`
	ServiceAccountJSON   *string `json:"service_account_json,omitempty"`
	AccountKey           *string `json:"account_key,omitempty"`
	SASToken             *string `json:"sas_token,omitempty"`
	BearerToken          *string `json:"bearer_token,omitempty"`
}

// ConnectionCapabilities say what a connection can be used for.
type ConnectionCapabilities struct {
	Source      bool `json:"source"`
	Destination bool `json:"destination"`
	Watch       bool `json:"watch"`
	// Trigger: it can trigger queue automations (sqs).
	Trigger bool `json:"trigger"`
	// Events: it can receive events (messaging kinds).
	Events bool `json:"events"`
}

// Connection is something outside Transcdr, with credentials, that it talks
// to: storage, or messaging.
type Connection struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Kind   string           `json:"kind"`
	Config ConnectionConfig `json:"config"`
	// SecretsSet names the stored secrets; their values are never returned.
	SecretsSet []string `json:"secrets_set"`
	// Secrets has an entry per stored secret, by name, with its fingerprint.
	Secrets      map[string]SecretStatus `json:"secrets,omitempty"`
	Capabilities ConnectionCapabilities  `json:"capabilities"`
	// Status is untested, ok or error.
	Status string `json:"status"`
	// Class is storage or messaging.
	Class string `json:"class"`
	// Enabled turns off by itself after a permanent failure, or 5 transient
	// ones in a row; while off, using it is refused (409 connection_disabled).
	Enabled      bool `json:"enabled"`
	FailureCount int  `json:"failure_count"`
	// DisabledReason is "<activity>: <error>" or "Disabled by hand.".
	DisabledReason *string    `json:"disabled_reason"`
	DisabledAt     *time.Time `json:"disabled_at"`
	LastError      *string    `json:"last_error"`
	LastCheckedAt  *time.Time `json:"last_checked_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ConnectionCreateParams create a connection; it is tested when saved.
type ConnectionCreateParams struct {
	Name    string             `json:"name"`
	Kind    string             `json:"kind"`
	Config  ConnectionConfig   `json:"config"`
	Secrets *ConnectionSecrets `json:"secrets,omitempty"`
}

// ConnectionCheckParams verify settings without saving them.
type ConnectionCheckParams struct {
	Name    string             `json:"name,omitempty"`
	Kind    string             `json:"kind"`
	Config  ConnectionConfig   `json:"config"`
	Secrets *ConnectionSecrets `json:"secrets,omitempty"`
}

// SecretStatus describes a write-only secret that is set. The fingerprint
// is an HMAC keyed with a server secret and bound to the object and the
// field: it changes when the secret changes, and cannot be computed or
// checked on the client. Compare it with an earlier read to notice a secret
// changed elsewhere.
type SecretStatus struct {
	Set bool `json:"set"`
	// Fingerprint is "hmac-sha256:" and 12 hex digits.
	Fingerprint string `json:"fingerprint"`
}

// SecretChanged reports whether a secret differs between two reads of the
// same object: it was set, cleared or replaced in between.
func SecretChanged(before, after map[string]SecretStatus, name string) bool {
	b, bok := before[name]
	a, aok := after[name]
	return bok != aok || b != a
}

// ConnectionUpdateParams change a connection.
type ConnectionUpdateParams struct {
	Name *string `json:"name,omitempty"`
	// Enabled true turns it back on (the failure count resets and it is
	// tested again); false turns it off.
	Enabled *bool `json:"enabled,omitempty"`
	// Config is merged into the stored config.
	Config  *ConnectionConfig  `json:"config,omitempty"`
	Secrets *ConnectionSecrets `json:"secrets,omitempty"`
}

// ConnectionTestResult is the outcome of testing a connection.
type ConnectionTestResult struct {
	OK         bool       `json:"ok"`
	Error      *string    `json:"error"`
	Connection Connection `json:"connection"`
}

// RemoteObject is a file (or, when Path ends in /, a folder) in a connection.
type RemoteObject struct {
	Path         string     `json:"path"`
	Size         *int64     `json:"size"`
	LastModified *time.Time `json:"last_modified"`
}

// BrowseParams list files in a connection.
type BrowseParams struct {
	Prefix    string
	Recursive bool
}

// Automation triggers.
const (
	TriggerWatch = "watch"
	TriggerHook  = "hook"
	TriggerQueue = "queue"
)

// AutomationSource is where an automation's files are.
type AutomationSource struct {
	ConnectionID string `json:"connection_id"`
	Prefix       string `json:"prefix"`
	Pattern      string `json:"pattern"`
}

// Automation turns files that land in a connection into jobs.
type Automation struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Trigger is watch, hook or queue.
	Trigger string `json:"trigger"`
	// TriggerConnectionID is, for queue, the sqs connection consumed.
	TriggerConnectionID *string          `json:"trigger_connection_id"`
	Source              AutomationSource `json:"source"`
	PollIntervalSeconds int              `json:"poll_interval_seconds"`
	SettleSeconds       int              `json:"settle_seconds"`
	// Preset is a preset slug, pre_… id, or "<slug>@N" for version N.
	Preset *string `json:"preset"`
	// Output holds fields merged over the preset when a job is made, in v2.
	Output OutputOverrides `json:"output"`
	// ResolvedOutput is the complete spec they resolve to now.
	ResolvedOutput *OutputSpec     `json:"resolved_output"`
	Destination    *JobDestination `json:"destination"`
	// AfterSuccess is keep or delete (the source file).
	AfterSuccess string   `json:"after_success"`
	Priority     string   `json:"priority"`
	Metadata     Metadata `json:"metadata"`
	WebhookURL   *string  `json:"webhook_url"`
	// HookURL is the push endpoint for hook automations, shown to
	// automations:write holders. It is a credential.
	HookURL         *string    `json:"hook_url,omitempty"`
	JobsCreated     int64      `json:"jobs_created"`
	LastPolledAt    *time.Time `json:"last_polled_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at"`
	LastError       *string    `json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AutomationSourceParams set an automation's source.
type AutomationSourceParams struct {
	ConnectionID string  `json:"connection_id,omitempty"`
	Prefix       *string `json:"prefix,omitempty"`
	Pattern      *string `json:"pattern,omitempty"`
}

// AutomationParams create (Name and Source.ConnectionID required) or update
// an automation. On update a field left out is unchanged, and a Nullable
// field sent as [Null] is cleared:
//
//	client.Automations.Update(ctx, id, &transcdr.AutomationParams{
//		Destination: transcdr.Null[transcdr.JobDestination](), // keep outputs in Transcdr
//		WebhookURL:  transcdr.Null[string](),
//	})
type AutomationParams struct {
	Name    string `json:"name,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	// Trigger is watch (default), hook or queue.
	Trigger string `json:"trigger,omitempty"`
	// TriggerConnectionID is the sqs connection a queue automation consumes
	// (required for queue).
	TriggerConnectionID Nullable[string]        `json:"trigger_connection_id,omitzero"`
	Source              *AutomationSourceParams `json:"source,omitempty"`
	// PollIntervalSeconds is 60–86400.
	PollIntervalSeconds *int `json:"poll_interval_seconds,omitempty"`
	// SettleSeconds is 0–86400.
	SettleSeconds *int `json:"settle_seconds,omitempty"`
	// Preset is a preset slug, pre_… id, or "<slug>@N" to pin version N;
	// otherwise each job uses the preset's latest version.
	Preset Nullable[string] `json:"preset,omitzero"`
	// Output holds fields merged over the preset when a job is made, in v2.
	Output Nullable[OutputOverrides] `json:"output,omitzero"`
	// Destination delivers outputs to a storage connection.
	Destination Nullable[JobDestination] `json:"destination,omitzero"`
	// AfterSuccess is keep or delete.
	AfterSuccess string `json:"after_success,omitempty"`
	Priority     string `json:"priority,omitempty"`
	// Metadata is added to every job; on update it replaces the stored map.
	Metadata Nullable[Metadata] `json:"metadata,omitzero"`
	// WebhookURL is a per-job webhook for every job created.
	WebhookURL Nullable[string] `json:"webhook_url,omitzero"`
}

// AutomationRun is the result of running or triggering an automation.
type AutomationRun struct {
	JobsCreated int64    `json:"jobs_created"`
	JobIDs      []string `json:"job_ids,omitempty"`
	// MessagesReceived is, for queue automations, messages read.
	MessagesReceived *int64 `json:"messages_received,omitempty"`
	// MessagesDeleted is, for queue automations, messages handled.
	MessagesDeleted *int64 `json:"messages_deleted,omitempty"`
}

// AutomationTriggerParams name files to process now: Path or Paths.
type AutomationTriggerParams struct {
	Path  string   `json:"path,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

// AutomationItem is one processed source object.
type AutomationItem struct {
	Path      string    `json:"path"`
	SizeBytes *int64    `json:"size_bytes"`
	Status    string    `json:"status"`
	JobID     *string   `json:"job_id"`
	Error     *string   `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}
