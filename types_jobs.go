package transcdr

import (
	"encoding/json"
	"net/url"
	"strings"
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
	// Format is an image output's format.
	Format ImageFormat `json:"format,omitempty"`
	// Rendition is the rendition an image output was made for: its label, or
	// the size it came out at.
	Rendition string `json:"rendition,omitempty"`
	// Frame is which still of a video an image output is, from 1, when there
	// are several.
	Frame *int `json:"frame,omitempty"`
	// AtSeconds is the time of a video's still, in seconds.
	AtSeconds *float64 `json:"at_seconds,omitempty"`
}

// An output image's price tier, by the pixels it came out at
// (JobBilling.Tier, Usage.ByImageTier).
const (
	ImageTierUpTo1MP = "up_to_1mp"
	ImageTierUpTo4MP = "up_to_4mp"
	ImageTierOver4MP = "over_4mp"
)

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
	// BillableImages is the output images billed; an image job bills no
	// minutes.
	BillableImages int64 `json:"billable_images,omitempty"`
	// AmountCents is rounded up to the cent.
	AmountCents int64 `json:"amount_cents"`
	// AmountUSD is exact (sub-cent).
	AmountUSD *float64 `json:"amount_usd,omitempty"`
	// Tier is sd, hd or uhd, or for an image job up_to_1mp, up_to_4mp or
	// over_4mp (the ImageTier* constants); nil until the job has produced
	// output.
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
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	System      bool   `json:"system"`
	// Category is the group the preset is shown in.
	Category PresetCategory `json:"category"`
	// Compatibility lists the platforms the output plays on.
	Compatibility []Platform `json:"compatibility"`
	// CompatibilityNotes gives each platform in Compatibility its minimum
	// versions and conditions, such as audio that depends on the source.
	CompatibilityNotes map[Platform]string `json:"compatibility_notes"`
	Output             OutputSpec          `json:"output"`
	Metadata           Metadata            `json:"metadata"`
	CreatedAt          *time.Time          `json:"created_at"`
	UpdatedAt          *time.Time          `json:"updated_at"`
}

// PresetCategory is the group a preset is shown in. More may be added, so
// treat an unknown one as uncategorised.
type PresetCategory string

// Preset categories.
const (
	// CategoryWeb is a single MP4 for browsers.
	CategoryWeb PresetCategory = "web"
	// CategoryMobile is a single file for native iOS and Android playback.
	CategoryMobile PresetCategory = "mobile"
	// CategoryStreaming is adaptive HLS.
	CategoryStreaming PresetCategory = "streaming"
	// CategoryTV is smart TVs, set-top boxes and constant bit rate.
	CategoryTV PresetCategory = "tv"
	// CategorySocial is portrait video for social apps.
	CategorySocial PresetCategory = "social"
	// CategoryAudio is audio-only output.
	CategoryAudio PresetCategory = "audio"
	// CategoryArchive is preservation and mastering: visually lossless, HDR.
	CategoryArchive PresetCategory = "archive"
	// CategoryImage is still images.
	CategoryImage PresetCategory = "image"
)

// Platform is where an output plays. More may be added.
type Platform string

// Platforms.
const (
	// PlatformWeb is current Chrome, Edge, Firefox and Safari.
	PlatformWeb Platform = "web"
	// PlatformIOS is iPhone and iPad.
	PlatformIOS Platform = "ios"
	// PlatformAndroid is Android phones and tablets.
	PlatformAndroid Platform = "android"
	// PlatformSmartTV is smart TVs and streaming sticks.
	PlatformSmartTV Platform = "smart_tv"
	// PlatformLegacy is old browsers and devices, and set-top boxes.
	PlatformLegacy Platform = "legacy"
	// PlatformEditing is editing applications.
	PlatformEditing Platform = "editing"
)

// PresetCreateParams create a preset.
type PresetCreateParams struct {
	Name        string           `json:"name"`
	Slug        *string          `json:"slug,omitempty"`
	Description *string          `json:"description,omitempty"`
	Output      *OutputSpecInput `json:"output"`
	Metadata    Metadata         `json:"metadata,omitempty"`
	// Category, Compatibility and CompatibilityNotes left out are derived
	// from Output. A non-nil empty Compatibility claims no platform; notes
	// are only for platforms the preset claims.
	Category           PresetCategory      `json:"category,omitempty"`
	Compatibility      []Platform          `json:"compatibility,omitzero"`
	CompatibilityNotes map[Platform]string `json:"compatibility_notes,omitzero"`
}

// PresetUpdateParams change a preset (PATCH): a field left out is
// unchanged, Null clears Description or Metadata and derives Category,
// Compatibility or CompatibilityNotes from the spec again, and Output is
// merged into the stored spec. To set the whole preset, use
// [PresetsService.Replace].
type PresetUpdateParams struct {
	Name        *string            `json:"name,omitempty"`
	Slug        *string            `json:"slug,omitempty"`
	Description Nullable[string]   `json:"description,omitzero"`
	Output      *OutputSpecInput   `json:"output,omitempty"`
	Metadata    Nullable[Metadata] `json:"metadata,omitzero"`

	Category           Nullable[PresetCategory]      `json:"category,omitzero"`
	Compatibility      Nullable[[]Platform]          `json:"compatibility,omitzero"`
	CompatibilityNotes Nullable[map[Platform]string] `json:"compatibility_notes,omitzero"`
}

// PresetReplaceParams replace a preset (PUT). Output is the whole spec: a
// field left out of it takes its default, as on create. Description and
// Metadata left out are emptied; Category, Compatibility and
// CompatibilityNotes left out are derived again; Slug left out is kept.
type PresetReplaceParams struct {
	Name               string              `json:"name"`
	Output             *OutputSpecInput    `json:"output"`
	Slug               *string             `json:"slug,omitempty"`
	Description        string              `json:"description,omitempty"`
	Metadata           Metadata            `json:"metadata,omitempty"`
	Category           PresetCategory      `json:"category,omitempty"`
	Compatibility      []Platform          `json:"compatibility,omitzero"`
	CompatibilityNotes map[Platform]string `json:"compatibility_notes,omitzero"`
}

// PresetListParams filter presets. [PresetsService.List] and
// [PresetsService.All] also take a plain *ListParams.
type PresetListParams struct {
	ListParams
	// Category keeps presets in any of these categories.
	Category []PresetCategory
	// CompatibleWith keeps presets that play on every one of these.
	CompatibleWith []Platform
	// ExcludeSystem leaves the system presets out.
	ExcludeSystem bool
}

// PresetListQuery is a *ListParams or a *PresetListParams.
type PresetListQuery interface {
	presetQuery() url.Values
}

func (p *ListParams) presetQuery() url.Values { return p.values(nil) }

func (p *PresetListParams) presetQuery() url.Values {
	if p == nil {
		return url.Values{}
	}
	q := p.ListParams.values(nil)
	if len(p.Category) > 0 {
		names := make([]string, len(p.Category))
		for i, c := range p.Category {
			names[i] = string(c)
		}
		q.Set("category", strings.Join(names, ","))
	}
	if len(p.CompatibleWith) > 0 {
		names := make([]string, len(p.CompatibleWith))
		for i, c := range p.CompatibleWith {
			names[i] = string(c)
		}
		q.Set("compatible_with", strings.Join(names, ","))
	}
	if p.ExcludeSystem {
		q.Set("system", "false")
	}
	return q
}

func presetQuery(params PresetListQuery) url.Values {
	if params == nil {
		return url.Values{}
	}
	return params.presetQuery()
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
