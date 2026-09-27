package transcdr

import (
	"context"
	"encoding/json"
	"testing"
)

// Every create sends an Idempotency-Key and, after a 5xx, is sent again with
// the same key and the same body, so the API replays rather than duplicates.
func TestEveryCreateIsIdempotentAndRetried(t *testing.T) {
	ctx := context.Background()
	creates := []struct {
		path   string
		create func(c *Client) error
	}{
		{"/v1/jobs", func(c *Client) error {
			_, err := c.Jobs.Create(ctx, &JobCreateParams{Input: URLInput("https://example.com/in.mp4")})
			return err
		}},
		{"/v1/probe", func(c *Client) error {
			_, err := c.Probe.Create(ctx, &ProbeParams{Input: URLInput("https://example.com/in.mp4")})
			return err
		}},
		{"/v1/uploads", func(c *Client) error {
			_, err := c.Uploads.Create(ctx, &UploadCreateParams{Filename: "in.mp4", SizeBytes: 10})
			return err
		}},
		{"/v1/assets", func(c *Client) error {
			_, err := c.Assets.Create(ctx, &AssetImportParams{URL: "https://example.com/in.mp4"})
			return err
		}},
		{"/v1/presets", func(c *Client) error {
			_, err := c.Presets.Create(ctx, &PresetCreateParams{Name: "p", Output: &OutputSpecInput{Codec: "av1"}})
			return err
		}},
		{"/v1/webhooks", func(c *Client) error {
			_, err := c.Webhooks.Create(ctx, &WebhookCreateParams{URL: "https://example.com/hooks"})
			return err
		}},
		{"/v1/connections", func(c *Client) error {
			_, err := c.Connections.Create(ctx, &ConnectionCreateParams{Name: "c", Kind: KindWebhook, Config: ConnectionConfig{URL: Value("https://example.com/hooks")}})
			return err
		}},
		{"/v1/automations", func(c *Client) error {
			_, err := c.Automations.Create(ctx, &AutomationParams{Name: "a", Source: &AutomationSourceParams{ConnectionID: "con_1"}})
			return err
		}},
		{"/v1/api-keys", func(c *Client) error {
			_, err := c.APIKeys.Create(ctx, &APIKeyCreateParams{Name: "k"})
			return err
		}},
		{"/v1/organization/members", func(c *Client) error {
			_, err := c.Organization.Members.Create(ctx, &MemberCreateParams{Email: "person@example.com", Role: RoleMember})
			return err
		}},
		{"/v1/organizations", func(c *Client) error {
			_, err := c.Organizations.Create(ctx, &OrganizationCreateParams{Name: "o"})
			return err
		}},
	}
	for _, tc := range creates {
		t.Run(tc.path, func(t *testing.T) {
			f := newFakeAPI(t)
			f.reply(502, "").reply(201, `{"id":"x"}`, "Idempotent-Replayed", "true")
			must(t, tc.create(f.client()))
			reqs := f.all()
			if len(reqs) != 2 {
				t.Fatalf("%d requests, want 2", len(reqs))
			}
			key := reqs[0].Header.Get("Idempotency-Key")
			if !uuidShape(key) || reqs[1].Header.Get("Idempotency-Key") != key {
				t.Fatalf("keys %q then %q", key, reqs[1].Header.Get("Idempotency-Key"))
			}
			if reqs[0].Method != "POST" || reqs[0].Path != tc.path || string(reqs[0].Body) != string(reqs[1].Body) {
				t.Fatalf("%s %s, bodies %s / %s", reqs[0].Method, reqs[0].Path, reqs[0].Body, reqs[1].Body)
			}
		})
	}
}

// Two creates get different keys, and a caller's key wins.
func TestCreateIdempotencyKeysAreFresh(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	for range 2 {
		_, err := c.Webhooks.Create(context.Background(), &WebhookCreateParams{URL: "https://example.com/hooks"})
		must(t, err)
	}
	_, err := c.Webhooks.Create(context.Background(), &WebhookCreateParams{URL: "https://example.com/hooks"}, WithIdempotencyKey("deploy-42"))
	must(t, err)
	reqs := f.all()
	if reqs[0].Header.Get("Idempotency-Key") == reqs[1].Header.Get("Idempotency-Key") {
		t.Fatal("two creates shared a key")
	}
	if got := reqs[2].Header.Get("Idempotency-Key"); got != "deploy-42" {
		t.Fatalf("caller key lost: %q", got)
	}
}

// A key reused for a different request is a 409 conflict, not retried.
func TestIdempotencyKeyReused(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(409, `{"error":{"type":"invalid_request_error","code":"idempotency_key_reused","message":"This Idempotency-Key was used for a different request."}}`)
	_, err := f.client().Presets.Create(context.Background(), &PresetCreateParams{Name: "p", Output: &OutputSpecInput{}}, WithIdempotencyKey("k"))
	e, ok := AsError(err)
	if !ok || !IsConflict(err) || e.Code != "idempotency_key_reused" || len(f.all()) != 1 {
		t.Fatalf("err = %v after %d requests", err, len(f.all()))
	}
}

func TestSecretFingerprints(t *testing.T) {
	var conn Connection
	must(t, json.Unmarshal([]byte(`{"id":"con_1","secrets_set":["access_key_id","secret_access_key"],
		"secrets":{"access_key_id":{"set":true,"fingerprint":"hmac-sha256:5c0e11112222"},
		"secret_access_key":{"set":true,"fingerprint":"hmac-sha256:a17d33334444"}}}`), &conn))
	if s := conn.Secrets["secret_access_key"]; !s.Set || s.Fingerprint != "hmac-sha256:a17d33334444" {
		t.Fatalf("secrets = %+v", conn.Secrets)
	}
	var w WebhookEndpoint
	must(t, json.Unmarshal([]byte(`{"id":"whk_1","secrets":{"secret":{"set":true,"fingerprint":"hmac-sha256:8314c9b1bfa0"}}}`), &w))
	if w.Secrets["secret"].Fingerprint != "hmac-sha256:8314c9b1bfa0" {
		t.Fatalf("secrets = %+v", w.Secrets)
	}

	before := conn.Secrets
	after := map[string]SecretStatus{
		"access_key_id":     before["access_key_id"],
		"secret_access_key": {Set: true, Fingerprint: "hmac-sha256:000000000000"},
	}
	if SecretChanged(before, after, "access_key_id") {
		t.Error("an unchanged secret changed")
	}
	if !SecretChanged(before, after, "secret_access_key") {
		t.Error("a replaced secret did not change")
	}
	if !SecretChanged(before, map[string]SecretStatus{}, "access_key_id") || !SecretChanged(nil, after, "access_key_id") {
		t.Error("a cleared or new secret did not change")
	}
	if SecretChanged(nil, nil, "password") {
		t.Error("a secret never set changed")
	}
}

func TestMeIsSession(t *testing.T) {
	cases := map[string]bool{"tds_ab12": true, "tdk_live_ab12": false, "tdk_test_ab12": false}
	for prefix, want := range cases {
		me := Me{APIKey: &APIKey{Prefix: prefix}}
		if me.IsSession() != want {
			t.Errorf("%s: IsSession = %v", prefix, !want)
		}
	}
	if (&Me{}).IsSession() {
		t.Error("no token is not a session")
	}
}
