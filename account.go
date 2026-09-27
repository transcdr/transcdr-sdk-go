package transcdr

import (
	"context"
	"errors"
	"iter"
	"net/url"
	"strconv"
)

// AuthService signs users in and out.
type AuthService struct{ client *Client }

// Register creates a user and an organization. The session token is returned,
// not stored on the client.
func (s *AuthService) Register(ctx context.Context, params *RegisterParams, opts ...RequestOption) (*AuthResponse, error) {
	return doJSONBody[AuthResponse](ctx, s.client, "POST", "/v1/auth/register", params, opts)
}

// Login exchanges an email and password for a session token (not stored on
// the client; see [Client.SetAPIKey]).
func (s *AuthService) Login(ctx context.Context, params *LoginParams, opts ...RequestOption) (*AuthResponse, error) {
	return doJSONBody[AuthResponse](ctx, s.client, "POST", "/v1/auth/login", params, opts)
}

// Switch issues a session token for another of the user's organizations
// (sessions only). The current token is revoked: store the new one.
func (s *AuthService) Switch(ctx context.Context, organizationID string, opts ...RequestOption) (*AuthResponse, error) {
	return doJSONBody[AuthResponse](ctx, s.client, "POST", "/v1/auth/switch", map[string]string{"organization_id": organizationID}, opts)
}

// Logout revokes the session token in use.
func (s *AuthService) Logout(ctx context.Context, opts ...RequestOption) error {
	return s.client.do(ctx, "POST", "/v1/auth/logout", nil, nil, nil, opts)
}

// ChangePassword changes the signed-in user's password and revokes their
// other sessions.
func (s *AuthService) ChangePassword(ctx context.Context, params *ChangePasswordParams, opts ...RequestOption) error {
	return s.client.do(ctx, "POST", "/v1/auth/password", nil, params, nil, opts)
}

// Me returns the caller: the user (for an API key, the user who created it),
// the organization, a session's organizations (empty for an API key), the
// token presented and its scopes.
func (s *AuthService) Me(ctx context.Context, opts ...RequestOption) (*Me, error) {
	return doJSON[Me](ctx, s.client, "GET", "/v1/me", nil, opts)
}

// OrganizationService manages the organization the token belongs to.
type OrganizationService struct {
	client  *Client
	Members *MembersService
}

// Get retrieves the organization.
func (s *OrganizationService) Get(ctx context.Context, opts ...RequestOption) (*Organization, error) {
	return doJSON[Organization](ctx, s.client, "GET", "/v1/organization", nil, opts)
}

// Update changes the organization (owners and admins).
func (s *OrganizationService) Update(ctx context.Context, params *OrganizationUpdateParams, opts ...RequestOption) (*Organization, error) {
	return doJSONBody[Organization](ctx, s.client, "PATCH", "/v1/organization", params, opts)
}

// RotateJobWebhookSecret issues a new job_webhook_secret, which signs
// per-job webhook_url deliveries.
func (s *OrganizationService) RotateJobWebhookSecret(ctx context.Context, opts ...RequestOption) (*Organization, error) {
	return doJSON[Organization](ctx, s.client, "POST", "/v1/organization/rotate-job-webhook-secret", nil, opts)
}

// MembersService manages the organization's members.
type MembersService struct{ client *Client }

// List lists the members.
func (s *MembersService) List(ctx context.Context, opts ...RequestOption) ([]User, error) {
	return getAll[User](ctx, s.client, "/v1/organization/members", nil, opts)
}

// Create adds a member (see [MemberCreateParams]).
func (s *MembersService) Create(ctx context.Context, params *MemberCreateParams, opts ...RequestOption) (*User, error) {
	return create[User](ctx, s.client, "/v1/organization/members", params, opts)
}

// Update changes a member's role.
func (s *MembersService) Update(ctx context.Context, id, role string, opts ...RequestOption) (*User, error) {
	return doJSONBody[User](ctx, s.client, "PATCH", "/v1/organization/members/"+seg(id), map[string]string{"role": role}, opts)
}

// Delete removes a member.
func (s *MembersService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/organization/members/"+seg(id), nil, nil, nil, opts)
}

// Leave removes the signed-in user's own membership (sessions only). The last
// owner cannot leave (409 last_owner).
func (s *MembersService) Leave(ctx context.Context, opts ...RequestOption) error {
	me, err := doJSON[Me](ctx, s.client, "GET", "/v1/me", nil, opts)
	if err != nil {
		return err
	}
	if !me.IsSession() || me.User == nil {
		return errors.New("transcdr: Members.Leave needs a session token, not an API key")
	}
	return s.Delete(ctx, me.User.ID, opts...)
}

// OrganizationsService lists and creates the user's organizations (session
// tokens only).
type OrganizationsService struct{ client *Client }

// List lists the organizations the user belongs to.
func (s *OrganizationsService) List(ctx context.Context, opts ...RequestOption) ([]Membership, error) {
	return getAll[Membership](ctx, s.client, "/v1/organizations", nil, opts)
}

// Create creates an organization owned by the caller and returns a session
// token in it; the current token keeps working.
func (s *OrganizationsService) Create(ctx context.Context, params *OrganizationCreateParams, opts ...RequestOption) (*AuthResponse, error) {
	return create[AuthResponse](ctx, s.client, "/v1/organizations", params, opts)
}

// APIKeysService manages secret API keys.
type APIKeysService struct{ client *Client }

// List returns one page of keys (revoked keys are not listed).
func (s *APIKeysService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[APIKey], error) {
	return getPage[APIKey](ctx, s.client, "/v1/api-keys", params.values(nil), opts)
}

// All iterates over every key.
func (s *APIKeysService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[APIKey, error] {
	return iterate[APIKey](ctx, s.client, "/v1/api-keys", params.values(nil), opts)
}

// Create creates a key; its Secret is shown only now.
func (s *APIKeysService) Create(ctx context.Context, params *APIKeyCreateParams, opts ...RequestOption) (*APIKey, error) {
	return create[APIKey](ctx, s.client, "/v1/api-keys", params, opts)
}

// Revoke revokes a key immediately.
func (s *APIKeysService) Revoke(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/api-keys/"+seg(id), nil, nil, nil, opts)
}

// Delete is [APIKeysService.Revoke].
func (s *APIKeysService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.Revoke(ctx, id, opts...)
}

// Get retrieves a key (without its secret). A revoked key, or another
// organization's, is an error satisfying [IsNotFound].
func (s *APIKeysService) Get(ctx context.Context, id string, opts ...RequestOption) (*APIKey, error) {
	return doJSON[APIKey](ctx, s.client, "GET", "/v1/api-keys/"+seg(id), nil, opts)
}

// Find is [APIKeysService.Get].
//
// Deprecated: use Get. Find walked the list before the API could return one
// key.
func (s *APIKeysService) Find(ctx context.Context, id string, opts ...RequestOption) (*APIKey, error) {
	return s.Get(ctx, id, opts...)
}

// AnnouncementsService lists the changelog and service credits for the
// signed-in user.
type AnnouncementsService struct{ client *Client }

// List returns announcements, newest first. With Unseen, only what the user
// has not seen, service credits first.
func (s *AnnouncementsService) List(ctx context.Context, params *AnnouncementListParams, opts ...RequestOption) (*Page[Announcement], error) {
	q := url.Values{}
	if params != nil {
		if params.Unseen {
			q.Set("unseen", "true")
		}
		if params.Kind != "" {
			q.Set("kind", params.Kind)
		}
		if params.Limit > 0 {
			q.Set("limit", strconv.Itoa(params.Limit))
		}
	}
	return getPage[Announcement](ctx, s.client, "/v1/announcements", q, opts)
}

// MarkSeen records that the user has seen these announcements (sessions
// only; idempotent). No ids is a no-op.
func (s *AnnouncementsService) MarkSeen(ctx context.Context, ids []string, opts ...RequestOption) error {
	if len(ids) == 0 {
		return nil
	}
	return s.client.do(ctx, "POST", "/v1/announcements/seen", nil, map[string][]string{"ids": ids}, nil, opts)
}

// MarkAllSeen marks everything the user can see as seen (sessions only).
func (s *AnnouncementsService) MarkAllSeen(ctx context.Context, opts ...RequestOption) error {
	return s.client.do(ctx, "POST", "/v1/announcements/seen", nil, map[string]bool{"all": true}, nil, opts)
}

// ChangelogService reads the public changelog; no key is needed.
type ChangelogService struct{ client *Client }

// List returns one page of published changelog entries, newest first.
func (s *ChangelogService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Announcement], error) {
	return getPage[Announcement](ctx, s.client, "/v1/changelog", params.values(nil), opts)
}

// All iterates over every published changelog entry.
func (s *ChangelogService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Announcement, error] {
	return iterate[Announcement](ctx, s.client, "/v1/changelog", params.values(nil), opts)
}
