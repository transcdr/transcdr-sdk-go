package transcdr

import (
	"encoding/json"
	"strings"
	"time"
)

// Scopes an API key can carry, besides "*".
var Scopes = []string{
	"jobs:read", "jobs:write", "assets:read", "assets:write", "presets:read", "presets:write",
	"webhooks:read", "webhooks:write", "usage:read", "billing:read", "billing:write", "keys:read", "keys:write",
	"org:read", "org:write", "connections:read", "connections:write", "automations:read", "automations:write",
}

// APIKey is a secret API key.
type APIKey struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Prefix string   `json:"prefix"`
	Scopes []string `json:"scopes"`
	// Mode is live or test.
	Mode       string     `json:"mode"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	// RevokedAt is when the key was revoked; revoked keys are not listed.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// Secret is present only in the create response.
	Secret *string `json:"secret,omitempty"`
}

// APIKeyCreateParams create a key.
type APIKeyCreateParams struct {
	Name string `json:"name"`
	// Scopes default to ["*"]; a key cannot grant more than the key creating it.
	Scopes []string `json:"scopes,omitempty"`
	// Mode is live (default) or test.
	Mode      string     `json:"mode,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Organization is an organization.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Plan is the plan id, e.g. starter.
	Plan         string  `json:"plan"`
	BillingEmail *string `json:"billing_email"`
	// JobWebhookSecret signs per-job webhook_url deliveries; returned only to
	// owners and admins holding org:write.
	JobWebhookSecret *string `json:"job_webhook_secret,omitempty"`
	// PlanDetails is the full plan.
	PlanDetails *Plan `json:"plan_details,omitempty"`
	// Suspended organizations cannot create jobs.
	Suspended *bool     `json:"suspended,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationUpdateParams change the organization (owners and admins).
type OrganizationUpdateParams struct {
	Name *string `json:"name,omitempty"`
	// BillingEmail: Null (or "") clears it.
	BillingEmail Nullable[string] `json:"billing_email,omitzero"`
}

// OrganizationCreateParams create an organization owned by the caller.
type OrganizationCreateParams struct {
	Name string `json:"name"`
}

// Roles.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// User is a user, with their role in the organization.
type User struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	OrganizationID string    `json:"organization_id"`
	CreatedAt      time.Time `json:"created_at"`
	// LastLoginAt is when the user last signed in.
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// MemberCreateParams add a member. An existing user's email gives them
// access (Name and Password are refused); an unknown email creates the user,
// and then Name and Password are required.
type MemberCreateParams struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Name     string `json:"name,omitempty"`
	Password string `json:"password,omitempty"`
}

// MembershipOrganization is the organization of a [Membership].
type MembershipOrganization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan"`
}

// Membership is an organization the user belongs to, with their role there.
type Membership struct {
	Organization MembershipOrganization `json:"organization"`
	Role         string                 `json:"role"`
	CreatedAt    time.Time              `json:"created_at"`
}

// RegisterParams create a user and an organization.
type RegisterParams struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organization_name"`
}

// LoginParams sign in.
type LoginParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// OrganizationID picks the organization; by default the one used last.
	OrganizationID string `json:"organization_id,omitempty"`
}

// ChangePasswordParams change the signed-in user's password.
type ChangePasswordParams struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// AuthResponse is a session.
type AuthResponse struct {
	// Token is a session token; the client does not store it (see
	// [Client.SetAPIKey]).
	Token        string       `json:"token"`
	User         User         `json:"user"`
	Organization Organization `json:"organization"`
	// Organizations are every organization the user belongs to.
	Organizations []Membership `json:"organizations"`
}

// Me is the caller.
type Me struct {
	// User is the signed-in user or, for an API key, the user who created
	// the key.
	User         *User        `json:"user"`
	Organization Organization `json:"organization"`
	// Organizations are a session's memberships (never empty); always empty
	// for an API key.
	Organizations []Membership `json:"organizations"`
	// APIKey is the token presented: an API key (prefix tdk_live_ or
	// tdk_test_) or a session (prefix tds_). See [Me.IsSession].
	APIKey *APIKey  `json:"api_key,omitempty"`
	Scopes []string `json:"scopes"`
	// Livemode is false for test-mode keys.
	Livemode *bool `json:"livemode,omitempty"`
}

// IsSession reports whether the caller is a session (signed in as the
// user) rather than an API key.
func (m *Me) IsSession() bool {
	return m.APIKey != nil && strings.HasPrefix(m.APIKey.Prefix, "tds_")
}

// Announcement kinds.
const (
	AnnouncementChangelog     = "changelog"
	AnnouncementServiceCredit = "service_credit"
)

// AnnouncementLink is a call to action; a URL that is a path is on the
// dashboard.
type AnnouncementLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ServiceCredit is what an incident's credit gave back.
type ServiceCredit struct {
	IncidentID string  `json:"incident_id"`
	AmountUSD  float64 `json:"amount_usd"`
	// Multiplier is how many times the affected charges were credited.
	Multiplier float64   `json:"multiplier"`
	Jobs       []string  `json:"jobs"`
	AppliedAt  time.Time `json:"applied_at"`
}

// Announcement is a changelog entry or a service-credit notice.
type Announcement struct {
	ID string `json:"id"`
	// Kind is changelog or service_credit.
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Body is Markdown.
	Body string `json:"body"`
	// PublishedAt is nil for a draft (operator console only).
	PublishedAt *time.Time        `json:"published_at"`
	Link        *AnnouncementLink `json:"link"`
	Tags        []string          `json:"tags"`
	Credit      *ServiceCredit    `json:"credit"`
	// Seen is always false for API keys.
	Seen   bool       `json:"seen"`
	SeenAt *time.Time `json:"seen_at"`
}

// AnnouncementListParams filter announcements.
type AnnouncementListParams struct {
	// Unseen returns only what the signed-in user has not seen.
	Unseen bool
	// Kind is changelog or service_credit.
	Kind  string
	Limit int
}

// CapabilityCodec is an output codec the service offers.
type CapabilityCodec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Default     bool   `json:"default"`
	HDR         bool   `json:"hdr"`
	BitDepths   []int  `json:"bit_depths"`
	RoyaltyFree bool   `json:"royalty_free"`
}

// CapabilityMode is an output mode the service offers.
type CapabilityMode struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// CapabilityImageFormat is an image output format the service offers.
type CapabilityImageFormat struct {
	ID      ImageFormat `json:"id"`
	Name    string      `json:"name"`
	Default bool        `json:"default"`
	// Lossy formats take Image.Quality.
	Lossy bool `json:"lossy"`
	// Lossless is whether it can be lossless (PNG always, WebP with
	// Image.Lossless).
	Lossless bool `json:"lossless"`
	// Alpha is whether it keeps transparency.
	Alpha bool `json:"alpha"`
	// DefaultQuality is the quality used when Image.Quality is nil; lossy
	// formats only.
	DefaultQuality *int `json:"default_quality,omitempty"`
}

// ImageLimits are the image output limits (limits.image).
type ImageLimits struct {
	// MinDimension and MaxDimension bound a rendition's sides.
	MinDimension int `json:"min_dimension"`
	MaxDimension int `json:"max_dimension"`
	// MaxOutputs is the most files one job may make: stills × renditions ×
	// formats.
	MaxOutputs int `json:"max_outputs"`
	// MaxFrames is the most stills one video may give.
	MaxFrames int `json:"max_frames"`
	// MaxInputMegapixels is the largest image input.
	MaxInputMegapixels float64 `json:"max_input_megapixels"`
}

// Capabilities are what the service supports.
type Capabilities struct {
	Codecs           []CapabilityCodec `json:"codecs"`
	Modes            []CapabilityMode  `json:"modes"`
	Audio            []string          `json:"audio"`
	BitDepth         []string          `json:"bit_depth"`
	Color            []string          `json:"color"`
	QualityTargets   []string          `json:"quality_targets"`
	Filters          []string          `json:"filters"`
	InputContainers  []string          `json:"input_containers"`
	InputVideoCodecs []string          `json:"input_video_codecs"`
	InputAudioCodecs []string          `json:"input_audio_codecs"`
	// ImageFormats are the image output formats; empty when image output is
	// unavailable.
	ImageFormats []CapabilityImageFormat `json:"image_formats"`
	// InputImageFormats are the image inputs read: jpeg, png, webp, avif, gif
	// (first frame), tiff, bmp, heic.
	InputImageFormats []string `json:"input_image_formats"`
	// Limits are the spec limits, e.g. max_width and segment_seconds; see
	// [Capabilities.ImageLimits] for limits.image.
	Limits        json.RawMessage `json:"limits"`
	SystemPresets []Preset        `json:"system_presets"`
	raw           json.RawMessage
}

// Raw is the whole response, including fields newer than this SDK.
func (c *Capabilities) Raw() json.RawMessage { return c.raw }

// ImageLimits are the image output limits in Limits, or nil when the
// service does not list them.
func (c *Capabilities) ImageLimits() *ImageLimits {
	var l struct {
		Image *ImageLimits `json:"image"`
	}
	if len(c.Limits) == 0 || json.Unmarshal(c.Limits, &l) != nil {
		return nil
	}
	return l.Image
}

// UnmarshalJSON decodes and keeps the raw response.
func (c *Capabilities) UnmarshalJSON(b []byte) error {
	type plain Capabilities
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*c = Capabilities(p)
	c.raw = append(json.RawMessage(nil), b...)
	return nil
}

// Status is the service status.
type Status struct {
	// Status is operational or degraded.
	Status string `json:"status"`
	// QueueDepth is jobs waiting to start.
	QueueDepth int64 `json:"queue_depth"`
	// RunningJobs is jobs being processed.
	RunningJobs int64  `json:"running_jobs"`
	Version     string `json:"version,omitempty"`
}

// StatsTotals are all-time counters.
type StatsTotals struct {
	JobsCompleted       int64   `json:"jobs_completed"`
	OutputMinutes       float64 `json:"output_minutes"`
	SourceMinutes       float64 `json:"source_minutes"`
	BytesDelivered      int64   `json:"bytes_delivered"`
	RenditionsDelivered int64   `json:"renditions_delivered"`
	Customers           int64   `json:"customers"`
}

// StatsDay is one day of stats.
type StatsDay struct {
	Date          string  `json:"date"`
	JobsCompleted int64   `json:"jobs_completed"`
	OutputMinutes float64 `json:"output_minutes"`
}

// Stats are public, cached platform-wide counters.
type Stats struct {
	Since     time.Time   `json:"since"`
	UpdatedAt time.Time   `json:"updated_at"`
	Totals    StatsTotals `json:"totals"`
	Last24h   struct {
		JobsCompleted int64   `json:"jobs_completed"`
		OutputMinutes float64 `json:"output_minutes"`
	} `json:"last_24h"`
	Last30d *struct {
		ActiveCustomers int64 `json:"active_customers"`
	} `json:"last_30d,omitempty"`
	// Daily is the last 30 days, oldest first.
	Daily []StatsDay `json:"daily"`
}
