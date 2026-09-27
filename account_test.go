package transcdr

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

var fxCtx = context.Background()

const authBody = `{"token":"tds_example","user":{"id":"usr_1","name":"Sample User","email":"person@example.com","role":"owner","organization_id":"org_1","created_at":"2026-09-26T08:51:37Z"},
"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter","billing_email":null,"created_at":"2026-09-26T08:51:37Z"},
"organizations":[{"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter"},"role":"owner","created_at":"2026-09-26T08:51:37Z"}]}`

func fxBody(t *testing.T, r recorded, want map[string]any) {
	t.Helper()
	if got := r.JSON(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}

func TestAuth(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(201, authBody)
	res, err := c.Auth.Register(fxCtx, &RegisterParams{Name: "Sample User", Email: "person@example.com", Password: "example-password", OrganizationName: "Acme"})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/auth/register"), map[string]any{"name": "Sample User", "email": "person@example.com", "password": "example-password", "organization_name": "Acme"})
	if res.Token != "tds_example" || res.User.Role != RoleOwner || len(res.Organizations) != 1 || res.Organizations[0].Organization.Slug != "acme" {
		t.Fatalf("decoded %+v", res)
	}
	if c.APIKey() != "tdk_test_example" {
		t.Fatal("Register must not store the session token")
	}

	f.reply(200, authBody)
	_, err = c.Auth.Login(fxCtx, &LoginParams{Email: "person@example.com", Password: "p", OrganizationID: "org_2"})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/auth/login"), map[string]any{"email": "person@example.com", "password": "p", "organization_id": "org_2"})

	f.reply(200, authBody)
	_, err = c.Auth.Login(fxCtx, &LoginParams{Email: "person@example.com", Password: "p"})
	must(t, err)
	if _, ok := f.last().JSON(t)["organization_id"]; ok {
		t.Fatal("an empty organization_id is left out")
	}

	f.reply(200, authBody)
	_, err = c.Auth.Switch(fxCtx, "org_2")
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/auth/switch"), map[string]any{"organization_id": "org_2"})

	f.reply(204, "")
	must(t, c.Auth.Logout(fxCtx))
	if r := f.expect("POST", "/v1/auth/logout"); len(r.Body) != 0 {
		t.Fatalf("logout sends no body, got %s", r.Body)
	}

	f.reply(204, "")
	must(t, c.Auth.ChangePassword(fxCtx, &ChangePasswordParams{CurrentPassword: "a", NewPassword: "b"}))
	fxBody(t, f.expect("POST", "/v1/auth/password"), map[string]any{"current_password": "a", "new_password": "b"})

	f.reply(200, fixture(t, "me.json"))
	me, err := c.Auth.Me(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/me")
	if me.Organization.ID == "" || len(me.Scopes) == 0 {
		t.Fatalf("me = %+v", me)
	}
}

func TestOrganization(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "organization.json"))
	org, err := c.Organization.Get(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/organization")
	if org.ID == "" || org.PlanDetails == nil {
		t.Fatalf("org = %+v", org)
	}

	f.reply(200, fixture(t, "organization.json"))
	_, err = c.Organization.Update(fxCtx, &OrganizationUpdateParams{Name: String("Acme 2")})
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/organization"), map[string]any{"name": "Acme 2"})

	f.reply(200, fixture(t, "organization.json"))
	_, err = c.Organization.Update(fxCtx, &OrganizationUpdateParams{BillingEmail: Null[string]()})
	must(t, err)
	fxBody(t, f.last(), map[string]any{"billing_email": nil})

	f.reply(200, fixture(t, "organization.json"))
	_, err = c.Organization.RotateJobWebhookSecret(fxCtx)
	must(t, err)
	f.expect("POST", "/v1/organization/rotate-job-webhook-secret")
}

func TestMembers(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	user := `{"id":"usr_2","name":"Sample User","email":"person@example.com","role":"admin","organization_id":"org_1","created_at":"2026-09-26T08:51:37Z"}`

	f.reply(200, fixture(t, "members.json"))
	members, err := c.Organization.Members.List(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/organization/members")
	if len(members) == 0 {
		t.Fatal("no members")
	}

	f.reply(201, user)
	_, err = c.Organization.Members.Create(fxCtx, &MemberCreateParams{Email: "person@example.com", Role: RoleAdmin})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/organization/members"), map[string]any{"email": "person@example.com", "role": "admin"})

	f.reply(200, user)
	_, err = c.Organization.Members.Update(fxCtx, "usr_2", RoleMember)
	must(t, err)
	fxBody(t, f.expect("PATCH", "/v1/organization/members/usr_2"), map[string]any{"role": "member"})

	f.reply(204, "")
	must(t, c.Organization.Members.Delete(fxCtx, "usr_2"))
	f.expect("DELETE", "/v1/organization/members/usr_2")
}

func TestMembersLeave(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	f.reply(200, `{"user":{"id":"usr_9","name":"Sample User","email":"person@example.com","role":"member","organization_id":"org_1","created_at":"2026-09-26T08:51:37Z"},
		"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter","billing_email":null,"created_at":"2026-09-26T08:51:37Z"},
		"organizations":[{"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter"},"role":"member"}],
		"api_key":{"id":"key_s","name":"Session","prefix":"tds_ab12","scopes":["*"],"mode":"live","last_used_at":null,"expires_at":null,"created_at":"2026-09-26T08:51:37Z"},"scopes":["*"]}`)
	f.reply(204, "")
	must(t, c.Organization.Members.Leave(fxCtx))
	reqs := f.all()
	if len(reqs) != 2 || reqs[0].Method != "GET" || reqs[0].Path != "/v1/me" || reqs[1].Method != "DELETE" || reqs[1].Path != "/v1/organization/members/usr_9" {
		t.Fatalf("requests = %+v", reqs)
	}

	// An API key's /v1/me names the key's creator as the user: an error, and
	// nothing is deleted.
	g := newFakeAPI(t)
	g.reply(200, `{"user":{"id":"usr_9","name":"Sample User","email":"person@example.com","role":"owner","organization_id":"org_1","created_at":"2026-09-26T08:51:37Z"},"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter","billing_email":null,"created_at":"2026-09-26T08:51:37Z"},
		"organizations":[],"api_key":{"id":"key_1","name":"CI","prefix":"tdk_test_ab12","scopes":["*"],"mode":"test","last_used_at":null,"expires_at":null,"created_at":"2026-09-26T08:51:37Z"},"scopes":["*"]}`)
	if err := g.client().Organization.Members.Leave(fxCtx); err == nil {
		t.Fatal("Leave with an API key must fail")
	}
	if n := len(g.all()); n != 1 {
		t.Fatalf("%d requests, want only GET /v1/me", n)
	}
}

func TestOrganizations(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	f.reply(200, `{"object":"list","data":[{"organization":{"id":"org_1","name":"Acme","slug":"acme","plan":"starter"},"role":"owner","created_at":"2026-09-26T08:51:37Z"}],"has_more":false,"next_cursor":null}`)
	list, err := c.Organizations.List(fxCtx)
	must(t, err)
	f.expect("GET", "/v1/organizations")
	if len(list) != 1 || list[0].Role != RoleOwner {
		t.Fatalf("list = %+v", list)
	}

	f.reply(201, authBody)
	res, err := c.Organizations.Create(fxCtx, &OrganizationCreateParams{Name: "Second"})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/organizations"), map[string]any{"name": "Second"})
	if res.Token == "" {
		t.Fatal("no token")
	}
}

func TestAPIKeys(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "api_keys.json"))
	page, err := c.APIKeys.List(fxCtx, &ListParams{Limit: 5, Cursor: "key_x"})
	must(t, err)
	r := f.expect("GET", "/v1/api-keys")
	if r.Query["limit"][0] != "5" || r.Query["cursor"][0] != "key_x" {
		t.Fatalf("query = %v", r.Query)
	}
	if len(page.Data) == 0 {
		t.Fatal("no keys")
	}

	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	f.reply(201, `{"id":"key_1","name":"CI","prefix":"tdk_test_ab12","scopes":["jobs:read"],"mode":"test","last_used_at":null,"expires_at":"2027-01-01T00:00:00Z","created_at":"2026-09-26T08:51:37Z","secret":"tdk_test_example"}`)
	key, err := c.APIKeys.Create(fxCtx, &APIKeyCreateParams{Name: "CI", Scopes: []string{"jobs:read"}, Mode: "test", ExpiresAt: &exp})
	must(t, err)
	fxBody(t, f.expect("POST", "/v1/api-keys"), map[string]any{"name": "CI", "scopes": []any{"jobs:read"}, "mode": "test", "expires_at": "2027-01-01T00:00:00Z"})
	if key.Secret == nil || *key.Secret != "tdk_test_example" || !key.ExpiresAt.Equal(exp) {
		t.Fatalf("key = %+v", key)
	}
	if r := f.last(); !uuidShape(r.Header.Get("Idempotency-Key")) {
		t.Fatalf("Idempotency-Key = %q", r.Header.Get("Idempotency-Key"))
	}

	f.reply(204, "")
	must(t, c.APIKeys.Revoke(fxCtx, "key_1"))
	f.expect("DELETE", "/v1/api-keys/key_1")
	f.reply(204, "")
	must(t, c.APIKeys.Delete(fxCtx, "key_2"))
	f.expect("DELETE", "/v1/api-keys/key_2")
}

func TestAPIKeysGet(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"object":"api_key","id":"key_b","name":"b","prefix":"tdk_live_ab12","scopes":["*"],"mode":"live","last_used_at":null,"expires_at":null,"revoked_at":null,"created_at":"2026-09-26T08:51:37Z"}`)
	key, err := f.client().APIKeys.Get(fxCtx, "key_b")
	must(t, err)
	f.expect("GET", "/v1/api-keys/key_b")
	if key.ID != "key_b" || key.Secret != nil {
		t.Fatalf("key = %+v", key)
	}

	// Revoked: 404, not retried.
	f.reply(404, `{"error":{"type":"invalid_request_error","code":"not_found","message":"No such API key."}}`)
	if _, err := f.client().APIKeys.Get(fxCtx, "key_r"); !IsNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}

	// Find is the deprecated name for Get.
	f.reply(200, `{"id":"key_b"}`)
	key, err = f.client().APIKeys.Find(fxCtx, "key_b")
	must(t, err)
	if key.ID != "key_b" || f.last().Path != "/v1/api-keys/key_b" {
		t.Fatalf("Find = %+v via %s", key, f.last().Path)
	}
}

func TestAPIKeysAll(t *testing.T) {
	page1 := `{"object":"list","data":[{"id":"key_a","name":"a","prefix":"p","scopes":["*"],"mode":"live","last_used_at":null,"expires_at":null,"created_at":"2026-09-26T08:51:37Z"}],"has_more":true,"next_cursor":"key_a"}`
	page2 := `{"object":"list","data":[{"id":"key_b","name":"b","prefix":"p","scopes":["*"],"mode":"live","last_used_at":null,"expires_at":null,"created_at":"2026-09-26T08:51:37Z"}],"has_more":false,"next_cursor":null}`

	f := newFakeAPI(t)
	f.reply(200, page1).reply(200, page2)
	var ids []string
	for k, err := range f.client().APIKeys.All(fxCtx, nil) {
		must(t, err)
		ids = append(ids, k.ID)
	}
	if !reflect.DeepEqual(ids, []string{"key_a", "key_b"}) {
		t.Fatalf("ids = %v", ids)
	}
	if reqs := f.all(); reqs[1].Query["cursor"][0] != "key_a" {
		t.Fatalf("second page query = %v", reqs[1].Query)
	}

}

func TestAnnouncements(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	list := `{"object":"list","data":[{"object":"announcement","id":"ann_1","kind":"service_credit","title":"Outage","body":"**Sorry**","published_at":"2026-09-27T09:00:00Z",
		"link":{"label":"Try it","url":"/app"},"tags":[],"credit":{"incident_id":"inc_1","amount_usd":0.2563,"multiplier":3,"jobs":["job_1"],"applied_at":"2026-09-27T09:00:00Z"},"seen":false,"seen_at":null}],"has_more":false,"next_cursor":null}`

	f.reply(200, list)
	page, err := c.Announcements.List(fxCtx, &AnnouncementListParams{Unseen: true, Kind: AnnouncementServiceCredit, Limit: 10})
	must(t, err)
	r := f.expect("GET", "/v1/announcements")
	if r.Query["unseen"][0] != "true" || r.Query["kind"][0] != "service_credit" || r.Query["limit"][0] != "10" {
		t.Fatalf("query = %v", r.Query)
	}
	if a := page.Data[0]; a.Credit == nil || a.Credit.Multiplier != 3 || a.Link.URL != "/app" {
		t.Fatalf("announcement = %+v", a)
	}

	f.reply(200, list)
	_, err = c.Announcements.List(fxCtx, nil)
	must(t, err)
	if q := f.last().Query; len(q) != 0 {
		t.Fatalf("no params, no query; got %v", q)
	}

	f.reply(204, "")
	must(t, c.Announcements.MarkSeen(fxCtx, []string{"ann_1", "ann_2"}))
	fxBody(t, f.expect("POST", "/v1/announcements/seen"), map[string]any{"ids": []any{"ann_1", "ann_2"}})

	n := len(f.all())
	must(t, c.Announcements.MarkSeen(fxCtx, nil))
	if len(f.all()) != n {
		t.Fatal("MarkSeen with no ids must not call the API")
	}

	f.reply(204, "")
	must(t, c.Announcements.MarkAllSeen(fxCtx))
	fxBody(t, f.expect("POST", "/v1/announcements/seen"), map[string]any{"all": true})
}

func TestChangelogIsPublic(t *testing.T) {
	t.Setenv("TRANSCDR_API_KEY", "")
	f := newFakeAPI(t)
	c := f.client(WithAPIKey(""))
	entry := `{"object":"announcement","id":"ann_%s","kind":"changelog","title":"t","body":"b","published_at":"2026-09-27T09:00:00Z","link":null,"tags":["integrations"],"credit":null,"seen":false,"seen_at":null}`
	f.reply(200, `{"object":"list","data":[`+fmt.Sprintf(entry, "1")+`],"has_more":true,"next_cursor":"ann_1"}`)
	page, err := c.Changelog.List(fxCtx, &ListParams{Limit: 1})
	must(t, err)
	r := f.expect("GET", "/v1/changelog")
	if r.Header.Get("Authorization") != "" {
		t.Fatal("no key, no Authorization header")
	}
	if !page.HasMore || page.NextCursor != "ann_1" || page.Data[0].Tags[0] != "integrations" {
		t.Fatalf("page = %+v", page)
	}

	f.reply(200, `{"object":"list","data":[`+fmt.Sprintf(entry, "1")+`],"has_more":true,"next_cursor":"ann_1"}`)
	f.reply(200, `{"object":"list","data":[`+fmt.Sprintf(entry, "2")+`],"has_more":false,"next_cursor":null}`)
	all, err := Collect(c.Changelog.All(fxCtx, nil), 0)
	must(t, err)
	if len(all) != 2 || all[1].ID != "ann_2" {
		t.Fatalf("all = %+v", all)
	}
}
