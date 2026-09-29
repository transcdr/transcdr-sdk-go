// Package transcdr is the Go client for the Transcdr video transcoding API.
//
//	client := transcdr.NewClient(transcdr.WithAPIKey(os.Getenv("TRANSCDR_API_KEY")))
//	job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
//		Input:  transcdr.URLInput("https://example.com/in.mp4"),
//		Preset: transcdr.String("hls-av1-abr"),
//	})
//
// Every call takes a [context.Context]. Lists come back a page at a time
// (List) or as an iterator over every item (All). Errors from the API are
// [*Error] values carrying the HTTP status, error type and code, field errors
// and the request id.
package transcdr

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Version is this SDK's version, sent in the User-Agent header.
const Version = "0.9.0"

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://api.transcdr.com"

// Client is the Transcdr API client. It is safe for concurrent use.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	userAgent    string
	maxRetries   int
	retryDelay   time.Duration
	maxRetryWait time.Duration
	timeout      time.Duration
	headers      http.Header

	keyMu  sync.RWMutex
	apiKey string

	Auth         *AuthService
	Organization *OrganizationService
	// Organizations are the user's organizations (session tokens only).
	Organizations *OrganizationsService
	APIKeys       *APIKeysService
	Uploads       *UploadsService
	Assets        *AssetsService
	Jobs          *JobsService
	Probe         *ProbeService
	Presets       *PresetsService
	Webhooks      *WebhooksService
	Events        *EventsService
	Usage         *UsageService
	Billing       *BillingService
	Plans         *PlansService
	Capabilities  *CapabilitiesService
	Status        *StatusService
	Stats         *StatsService
	Connections   *ConnectionsService
	Automations   *AutomationsService
	Deliveries    *DeliveriesService
	Announcements *AnnouncementsService
	// Changelog is public: no key needed.
	Changelog *ChangelogService
	// Admin is the platform operator console (operator session tokens only).
	Admin *AdminService
}

// Option configures a [Client].
type Option func(*Client)

// WithAPIKey sets the bearer token: a secret API key (tdk_live_… or
// tdk_test_…) or a session token (tds_…). By default the TRANSCDR_API_KEY
// environment variable is used.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL sets the API base URL. By default TRANSCDR_BASE_URL, else
// [DefaultBaseURL].
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient sets the HTTP client requests are sent with.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets how many times a failed request that is safe to repeat
// is retried: GET, PUT and DELETE requests, and POSTs carrying an
// Idempotency-Key (every create sends one), after a 429, a 5xx or a network
// error. Default 2.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = max(0, n) } }

// WithRetryDelay sets the base backoff delay; it doubles each attempt, with
// jitter, up to 8 s. Default 500 ms.
func WithRetryDelay(d time.Duration) Option { return func(c *Client) { c.retryDelay = d } }

// WithTimeout sets a per-attempt timeout. Default 60 s; 0 means none.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithUserAgent prepends a product token to the User-Agent header, e.g.
// "my-app/1.2".
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = strings.TrimSpace(ua + " " + c.userAgent) }
}

// WithHeader adds a header sent with every request.
func WithHeader(name, value string) Option { return func(c *Client) { c.headers.Add(name, value) } }

// NewClient returns a client configured by opts.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:      DefaultBaseURL,
		httpClient:   &http.Client{},
		userAgent:    "transcdr-sdk-go/" + Version,
		maxRetries:   2,
		retryDelay:   500 * time.Millisecond,
		maxRetryWait: 60 * time.Second,
		timeout:      60 * time.Second,
		headers:      http.Header{},
		apiKey:       os.Getenv("TRANSCDR_API_KEY"),
	}
	if u := os.Getenv("TRANSCDR_BASE_URL"); u != "" {
		c.baseURL = strings.TrimRight(u, "/")
	}
	for _, opt := range opts {
		opt(c)
	}

	c.Auth = &AuthService{c}
	c.Organization = &OrganizationService{client: c, Members: &MembersService{c}}
	c.Organizations = &OrganizationsService{c}
	c.APIKeys = &APIKeysService{c}
	c.Uploads = &UploadsService{c}
	c.Assets = &AssetsService{c}
	c.Jobs = &JobsService{c}
	c.Probe = &ProbeService{c}
	c.Presets = &PresetsService{c}
	c.Webhooks = &WebhooksService{c}
	c.Events = &EventsService{c}
	c.Usage = &UsageService{c}
	c.Billing = &BillingService{client: c, Invoices: &InvoicesService{c}}
	c.Plans = &PlansService{c}
	c.Capabilities = &CapabilitiesService{c}
	c.Status = &StatusService{c}
	c.Stats = &StatsService{c}
	c.Connections = &ConnectionsService{c}
	c.Automations = &AutomationsService{c}
	c.Deliveries = &DeliveriesService{c}
	c.Announcements = &AnnouncementsService{c}
	c.Changelog = &ChangelogService{c}
	c.Admin = &AdminService{client: c, Announcements: &AdminAnnouncementsService{c}, Incidents: &AdminIncidentsService{c}}
	return c
}

// BaseURL is the API base URL in use.
func (c *Client) BaseURL() string { return c.baseURL }

// APIKey is the bearer token in use, or "".
func (c *Client) APIKey() string {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.apiKey
}

// SetAPIKey replaces the bearer token, e.g. with the token from
// [AuthService.Login]. Pass "" to clear it.
func (c *Client) SetAPIKey(key string) {
	c.keyMu.Lock()
	c.apiKey = key
	c.keyMu.Unlock()
}
