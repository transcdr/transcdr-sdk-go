package transcdr

import (
	"encoding/json"
	"time"
)

// JobStatus is where a job is in its life.
type JobStatus string

// Job statuses.
const (
	JobQueued    JobStatus = "queued"
	JobScheduled JobStatus = "scheduled"
	JobRunning   JobStatus = "running"
	JobUploading JobStatus = "uploading"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

// JobStatuses lists every status.
var JobStatuses = []JobStatus{JobQueued, JobScheduled, JobRunning, JobUploading, JobCompleted, JobFailed, JobCanceled}

// IsTerminal reports whether no further transitions happen on their own:
// completed, failed or canceled.
func (s JobStatus) IsTerminal() bool {
	return s == JobCompleted || s == JobFailed || s == JobCanceled
}

// Priorities.
const (
	PriorityNormal = "normal"
	PriorityHigh   = "high"
)

// JobInput is where a job reads its source: a URL, an asset, or a file in a
// connection. Build one with [URLInput], [AssetInput] or [ConnectionInput].
type JobInput struct {
	// Type is url, asset or connection.
	Type         string `json:"type"`
	URL          string `json:"url,omitempty"`
	AssetID      string `json:"asset_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	Path         string `json:"path,omitempty"`
}

// URLInput reads an https:// URL (or s3://… with a storage integration).
func URLInput(url string) JobInput { return JobInput{Type: "url", URL: url} }

// AssetInput reads an uploaded or linked asset.
func AssetInput(assetID string) JobInput { return JobInput{Type: "asset", AssetID: assetID} }

// ConnectionInput reads a file in one of your connections.
func ConnectionInput(connectionID, path string) JobInput {
	return JobInput{Type: "connection", ConnectionID: connectionID, Path: path}
}

// JobDestination is where a job's outputs are delivered when it completes.
type JobDestination struct {
	ConnectionID string `json:"connection_id"`
	// Prefix is a template: {job_id}, {name}, {stem}, {ext}, {dir}, {date},
	// {automation}, {org}.
	Prefix string `json:"prefix,omitempty"`
}

// RenditionProgress is one rendition's progress.
type RenditionProgress struct {
	Index           int     `json:"index"`
	Label           string  `json:"label"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	Status          string  `json:"status"`
	Percent         float64 `json:"percent"`
	FramesDone      int64   `json:"frames_done"`
	FramesTotal     *int64  `json:"frames_total"`
	SegmentsWritten int64   `json:"segments_written"`
	BytesOut        int64   `json:"bytes_out"`
	Message         string  `json:"message,omitempty"`
}

// Progress is a job's progress.
type Progress struct {
	Percent float64 `json:"percent"`
	// Stage is waiting, fetching, probing, encoding, uploading or done.
	Stage      string              `json:"stage"`
	Renditions []RenditionProgress `json:"renditions"`
}

// JobOutput is one output file.
type JobOutput struct {
	Label       string `json:"label"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Frames      int64  `json:"frames"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
	// Path is relative to the job's output root, e.g. "1080p.mp4".
	Path string `json:"path"`
	URL  string `json:"url"`
}

// JobError is why a job failed.
type JobError struct {
	// Code is stable, e.g. decode_failed or input_unreachable.
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// JobBilling is what a job costs.
type JobBilling struct {
	BillableMinutes float64 `json:"billable_minutes"`
	// AmountCents is rounded up to the cent.
	AmountCents int64 `json:"amount_cents"`
	// AmountUSD is exact (sub-cent).
	AmountUSD *float64 `json:"amount_usd,omitempty"`
	// Tier is sd, hd or uhd; nil until the job has produced output.
	Tier *string `json:"tier"`
	// OutputDuration is seconds of output, once known.
	OutputDuration *float64 `json:"output_duration,omitempty"`
}

// Job is a transcode (or probe).
type Job struct {
	ID string `json:"id"`
	// Kind is transcode or probe.
	Kind      string      `json:"kind,omitempty"`
	Status    JobStatus   `json:"status"`
	Input     JobInput    `json:"input"`
	InputInfo *MediaInfo  `json:"input_info"`
	PresetID  *string     `json:"preset_id"`
	Output    OutputSpec  `json:"output"`
	Priority  string      `json:"priority"`
	Progress  Progress    `json:"progress"`
	Outputs   []JobOutput `json:"outputs"`
	// PlaylistURL is the bearer-authenticated HLS master playlist.
	PlaylistURL *string `json:"playlist_url"`
	// PlaybackURL is, for completed jobs, a signed URL (6 h) a player can
	// load without an Authorization header.
	PlaybackURL *string     `json:"playback_url,omitempty"`
	Error       *JobError   `json:"error"`
	Metadata    Metadata    `json:"metadata"`
	WebhookURL  *string     `json:"webhook_url"`
	Attempts    int         `json:"attempts"`
	MaxAttempts int         `json:"max_attempts"`
	Billing     *JobBilling `json:"billing"`
	Livemode    *bool       `json:"livemode,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	ScheduledAt *time.Time  `json:"scheduled_at"`
	StartedAt   *time.Time  `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// JobCreateParams submit a job.
type JobCreateParams struct {
	Input JobInput `json:"input"`
	// Output overrides are merged over the preset's spec.
	Output *OutputSpecInput `json:"output,omitempty"`
	// Preset is a system preset slug (e.g. hls-av1-abr) or a pre_… id.
	Preset     *string  `json:"preset,omitempty"`
	Priority   *string  `json:"priority,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
	WebhookURL *string  `json:"webhook_url,omitempty"`
	// Destination delivers every output file to a connection on completion.
	Destination *JobDestination `json:"destination,omitempty"`
	// MaxCostCents refuses the job (cost_limit_exceeded) if it would cost more.
	MaxCostCents *int64 `json:"max_cost_cents,omitempty"`
}

// JobListParams filter jobs.
type JobListParams struct {
	ListParams
	Status        JobStatus
	Preset        string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	// Metadata filters on metadata, sent as metadata[key]=value.
	Metadata Metadata
}

// JobEvent is one entry of a job's timeline.
type JobEvent struct {
	Type      string          `json:"type"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// ProbeParams probe an input without transcoding.
type ProbeParams struct {
	Input JobInput `json:"input"`
	// Wait blocks up to 60 s for the result.
	Wait bool `json:"-"`
}

// Asset is an uploaded or linked source file.
type Asset struct {
	ID string `json:"id"`
	// Status is pending_upload, ready, failed or deleted.
	Status         string     `json:"status"`
	Filename       string     `json:"filename"`
	ContentType    string     `json:"content_type"`
	SizeBytes      int64      `json:"size_bytes"`
	ChecksumSHA256 *string    `json:"checksum_sha256"`
	InputInfo      *MediaInfo `json:"input_info"`
	Metadata       Metadata   `json:"metadata"`
	DownloadURL    string     `json:"download_url"`
	// SourceURL is set for assets linked by URL; jobs read it directly.
	SourceURL *string `json:"source_url,omitempty"`
	// Livemode is false for assets created with a test-mode key.
	Livemode  *bool      `json:"livemode,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// AssetImportParams link a remote file by URL.
type AssetImportParams struct {
	URL      string   `json:"url"`
	Filename string   `json:"filename,omitempty"`
	Metadata Metadata `json:"metadata,omitempty"`
}

// Upload is a direct-to-storage upload session.
type Upload struct {
	ID      string `json:"id"`
	AssetID string `json:"asset_id"`
	// Status is pending, completed or expired.
	Status        string            `json:"status"`
	UploadURL     string            `json:"upload_url"`
	UploadMethod  string            `json:"upload_method"`
	UploadHeaders map[string]string `json:"upload_headers"`
	ExpiresAt     time.Time         `json:"expires_at"`
}

// UploadCreateParams open an upload session.
type UploadCreateParams struct {
	Filename    string   `json:"filename"`
	ContentType string   `json:"content_type"`
	SizeBytes   int64    `json:"size_bytes"`
	Metadata    Metadata `json:"metadata,omitempty"`
}

// Preset is a named output specification: a system one (its ID is its slug)
// or the organization's own.
type Preset struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	System      bool       `json:"system"`
	Output      OutputSpec `json:"output"`
	Metadata    Metadata   `json:"metadata"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// PresetCreateParams create a preset.
type PresetCreateParams struct {
	Name        string           `json:"name"`
	Slug        *string          `json:"slug,omitempty"`
	Description *string          `json:"description,omitempty"`
	Output      *OutputSpecInput `json:"output"`
	Metadata    Metadata         `json:"metadata,omitempty"`
}

// PresetUpdateParams change a preset. Output is merged into the stored spec.
type PresetUpdateParams struct {
	Name        *string          `json:"name,omitempty"`
	Slug        *string          `json:"slug,omitempty"`
	Description *string          `json:"description,omitempty"`
	Output      *OutputSpecInput `json:"output,omitempty"`
	Metadata    Metadata         `json:"metadata,omitzero"` // a non-nil empty map clears it
}

// Delivery is a job's outputs delivered to a connection.
type Delivery struct {
	ID           string `json:"id"`
	ConnectionID string `json:"connection_id"`
	Prefix       string `json:"prefix"`
	// Status is waiting, pending, running, succeeded or failed.
	Status      string     `json:"status"`
	Files       int64      `json:"files"`
	Bytes       int64      `json:"bytes"`
	Attempts    int        `json:"attempts"`
	Error       *string    `json:"error"`
	NextRetryAt *time.Time `json:"next_retry_at"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
