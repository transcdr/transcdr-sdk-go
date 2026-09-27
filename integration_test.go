package transcdr

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The integration test runs against a real API, such as a local development
// server with its demo organization:
//
//	TRANSCDR_INTEGRATION=1 TRANSCDR_BASE_URL=http://localhost:8080 TRANSCDR_API_KEY=tdk_live_… go test -run Integration -v
//
// It creates and removes its own presets, connections, event destinations,
// API keys, assets and jobs.
func TestIntegration(t *testing.T) {
	if os.Getenv("TRANSCDR_INTEGRATION") == "" {
		t.Skip("set TRANSCDR_INTEGRATION=1, TRANSCDR_BASE_URL and TRANSCDR_API_KEY to run against an API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := NewClient()
	suffix := strings.ToLower(NewIdempotencyKey()[:8])

	t.Run("public", func(t *testing.T) {
		status, err := c.Status.Get(ctx)
		must(t, err)
		if status.Status == "" {
			t.Error("empty status")
		}
		plans, err := c.Plans.List(ctx)
		must(t, err)
		if len(plans) == 0 {
			t.Error("no plans")
		}
		caps, err := c.Capabilities.Get(ctx)
		must(t, err)
		if len(caps.Codecs) == 0 || len(caps.SystemPresets) == 0 {
			t.Errorf("capabilities: %d codecs, %d system presets", len(caps.Codecs), len(caps.SystemPresets))
		}
		_, err = c.Stats.Get(ctx)
		must(t, err)
		doc, err := c.OpenAPI(ctx)
		must(t, err)
		if !bytes.Contains(doc, []byte("openapi")) {
			t.Error("not an OpenAPI document")
		}
	})

	t.Run("account", func(t *testing.T) {
		me, err := c.Auth.Me(ctx)
		must(t, err)
		// An API key: the user is the key's creator, and no memberships.
		if me.IsSession() || me.User == nil || me.APIKey == nil || len(me.Organizations) != 0 {
			t.Errorf("me = %+v", me)
		}
		org, err := c.Organization.Get(ctx)
		must(t, err)
		if me.Organization.ID != org.ID || org.PlanDetails == nil {
			t.Errorf("me org %s, org %s, plan details %v", me.Organization.ID, org.ID, org.PlanDetails)
		}
		_, err = c.Organization.Members.List(ctx)
		must(t, err)
		_, err = c.Usage.Get(ctx, &UsageParams{Granularity: "day"})
		must(t, err)
		inputs, err := c.Usage.Inputs(ctx, nil)
		must(t, err)
		if inputs.From == "" || inputs.To == "" {
			t.Errorf("input report %+v", inputs)
		}
		_, err = c.Billing.Get(ctx)
		must(t, err)
		_, err = c.Billing.Transactions(ctx, &CreditTransactionListParams{Limit: 5})
		must(t, err)
		_, err = c.Announcements.List(ctx, &AnnouncementListParams{Limit: 5})
		must(t, err)
	})

	t.Run("api keys", func(t *testing.T) {
		key, err := c.APIKeys.Create(ctx, &APIKeyCreateParams{Name: "sdk-go " + suffix, Scopes: []string{"jobs:read"}, Mode: "test"})
		must(t, err)
		if key.Secret == nil || !strings.HasPrefix(*key.Secret, "tdk_test_") {
			t.Fatalf("secret = %v", key.Secret)
		}
		found, err := c.APIKeys.Get(ctx, key.ID)
		must(t, err)
		if found.Name != key.Name || found.Secret != nil {
			t.Errorf("found %+v", found)
		}
		must(t, c.APIKeys.Revoke(ctx, key.ID))
		if _, err := c.APIKeys.Get(ctx, key.ID); !IsNotFound(err) {
			t.Errorf("revoked key still found: %v", err)
		}
	})

	t.Run("idempotency", func(t *testing.T) {
		idem := "sdk-go-" + suffix
		params := &WebhookCreateParams{URL: "https://example.com/idem/" + suffix, Events: []string{EventJobFailed}}
		first, err := c.Webhooks.Create(ctx, params, WithIdempotencyKey(idem))
		must(t, err)
		defer func() { must(t, c.Webhooks.Delete(ctx, first.ID)) }()
		again, err := c.Webhooks.Create(ctx, params, WithIdempotencyKey(idem))
		must(t, err)
		if again.ID != first.ID || again.Secret == nil || *again.Secret != *first.Secret {
			t.Errorf("replay made %s (first %s)", again.ID, first.ID)
		}
		_, err = c.Webhooks.Create(ctx, &WebhookCreateParams{URL: "https://example.com/other/" + suffix}, WithIdempotencyKey(idem))
		if e, ok := AsError(err); !ok || e.Code != "idempotency_key_reused" {
			t.Errorf("reused key: %v", err)
		}
	})

	t.Run("presets", func(t *testing.T) {
		system, err := c.Presets.Get(ctx, "hls-av1-abr")
		must(t, err)
		if !system.System || system.Output.Mode != "hls" {
			t.Errorf("system preset %+v", system)
		}
		p, err := c.Presets.Create(ctx, &PresetCreateParams{
			Name: "sdk-go cbr " + suffix,
			Output: RawOutputSpec([]byte(`{"mode":"hls","codec":"h264","quality":{"target":"cbr","bitrate":"4M","buffer_ms":1500},
				"renditions":[{"width":1920,"height":1080,"bitrate":"6M"},{"width":1280,"height":720}]}`)),
		})
		must(t, err)
		defer func() { must(t, c.Presets.Delete(ctx, p.ID)) }()
		if p.Output.Quality.Target == nil || *p.Output.Quality.Target != QualityCBR || *p.Output.Renditions[0].Bitrate != "6M" {
			t.Errorf("output %s", p.Output.Raw())
		}
		updated, err := c.Presets.Update(ctx, p.ID, &PresetUpdateParams{
			Description: Value("updated"),
			Output:      &OutputSpecInput{Quality: &Quality{Bitrate: String("5M")}},
			Metadata:    Value(Metadata{"k": "v"}),
		})
		must(t, err)
		if updated.Description != "updated" || *updated.Output.Quality.Bitrate != "5M" || updated.Metadata["k"] != "v" {
			t.Errorf("updated %+v", updated)
		}
		cleared, err := c.Presets.Update(ctx, p.ID, &PresetUpdateParams{Description: Null[string](), Metadata: Null[Metadata]()})
		must(t, err)
		if cleared.Description != "" || len(cleared.Metadata) != 0 {
			t.Errorf("cleared %+v", cleared)
		}
		// Replace: the output is the whole spec, so the CBR quality and the
		// renditions go back to their defaults.
		replaced, err := c.Presets.Replace(ctx, p.ID, &PresetReplaceParams{
			Name: "sdk-go replaced " + suffix, Output: RawOutputSpec([]byte(`{"mode":"single","codec":"h264"}`)), Description: "whole",
		})
		must(t, err)
		if replaced.Slug != p.Slug || replaced.Description != "whole" || replaced.Output.Mode != "single" ||
			(replaced.Output.Quality.Target != nil && *replaced.Output.Quality.Target == QualityCBR) {
			t.Errorf("replaced %s", replaced.Output.Raw())
		}
		n := 0
		for _, err := range c.Presets.All(ctx, &ListParams{Limit: 5}) {
			must(t, err)
			n++
		}
		if n < 10 {
			t.Errorf("only %d presets iterated", n)
		}
	})

	t.Run("connections and destinations", func(t *testing.T) {
		conn, err := c.Connections.Create(ctx, &ConnectionCreateParams{
			Name: "sdk-go hook " + suffix, Kind: KindWebhook,
			Config: ConnectionConfig{URL: Value("https://example.com/hooks/" + suffix)},
		})
		must(t, err)
		defer func() { must(t, c.Connections.Delete(ctx, conn.ID)) }()
		if conn.Class != "messaging" || !conn.Capabilities.Events {
			t.Errorf("connection %+v", conn)
		}
		sqs, err := c.Connections.Create(ctx, &ConnectionCreateParams{
			Name: "sdk-go sqs " + suffix, Kind: KindSQS,
			Config:  ConnectionConfig{QueueURL: Value("https://sqs.us-east-1.amazonaws.com/000000000000/sdk-go-" + suffix), MessageGroupID: Value("g")},
			Secrets: &ConnectionSecrets{AccessKeyID: String("AKIAIOSFODNN7EXAMPLE"), SecretAccessKey: String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
		})
		if err == nil {
			defer func() { must(t, c.Connections.Delete(ctx, sqs.ID)) }()
			fp := sqs.Secrets["secret_access_key"]
			if !fp.Set || !strings.HasPrefix(fp.Fingerprint, "hmac-sha256:") {
				t.Errorf("secrets %+v", sqs.Secrets)
			}
			rotated, err := c.Connections.Update(ctx, sqs.ID, &ConnectionUpdateParams{
				Secrets: &ConnectionSecrets{SecretAccessKey: String("je7MtGbClwBF/2Zp9Utk/h3yCo8nvbEXAMPLEKEY")},
				Config:  &ConnectionConfig{MessageGroupID: Null[string]()},
			})
			must(t, err)
			if !SecretChanged(sqs.Secrets, rotated.Secrets, "secret_access_key") || SecretChanged(sqs.Secrets, rotated.Secrets, "access_key_id") {
				t.Errorf("fingerprints %+v -> %+v", sqs.Secrets, rotated.Secrets)
			}
			if _, ok := rotated.Config.MessageGroupID.Get(); ok {
				t.Errorf("message_group_id not cleared: %+v", rotated.Config)
			}
		} else if e, ok := AsError(err); !ok || e.Status != 422 {
			// A queue the API cannot reach is refused (422); anything else is a failure.
			t.Errorf("sqs connection: %v", err)
		} else {
			t.Logf("sqs connection refused, fingerprints not checked: %v", err)
		}

		off, err := c.Connections.Disable(ctx, conn.ID)
		must(t, err)
		if off.Enabled || off.DisabledReason == nil {
			t.Errorf("disabled %+v", off)
		}
		_, err = c.Connections.Enable(ctx, conn.ID)
		must(t, err)

		w, err := c.Webhooks.Create(ctx, &WebhookCreateParams{URL: "https://example.com/events/" + suffix, Events: []string{EventJobCompleted}})
		must(t, err)
		defer func() { must(t, c.Webhooks.Delete(ctx, w.ID)) }()
		if w.Secret == nil || !strings.HasPrefix(*w.Secret, "whsec_") {
			t.Fatalf("secret = %v", w.Secret)
		}
		rotated, err := c.Webhooks.RotateSecret(ctx, w.ID)
		must(t, err)
		if rotated.Secret == nil || *rotated.Secret == *w.Secret {
			t.Error("the secret did not change")
		}
		if !SecretChanged(w.Secrets, rotated.Secrets, "secret") {
			t.Errorf("fingerprint did not change: %+v -> %+v", w.Secrets, rotated.Secrets)
		}
		described, err := c.Webhooks.Update(ctx, w.ID, &WebhookUpdateParams{Description: Value("desc")})
		must(t, err)
		undescribed, err := c.Webhooks.Update(ctx, w.ID, &WebhookUpdateParams{Description: Null[string]()})
		must(t, err)
		if described.Description != "desc" || undescribed.Description != "" {
			t.Errorf("description %q then %q", described.Description, undescribed.Description)
		}
		via, err := c.Webhooks.Create(ctx, &WebhookCreateParams{ConnectionID: conn.ID})
		must(t, err)
		must(t, c.Webhooks.Delete(ctx, via.ID))
		if via.ConnectionID == nil || *via.ConnectionID != conn.ID {
			t.Errorf("via connection %+v", via)
		}
	})

	t.Run("uploads and jobs", func(t *testing.T) {
		data := bytes.Repeat([]byte("transcdr"), 64*1024)
		var last UploadProgress
		asset, err := c.Uploads.UploadFile(ctx, bytes.NewReader(data), int64(len(data)), &UploadFileOptions{
			Filename: "sdk-go-" + suffix + ".mp4",
			OnProgress: func(p UploadProgress) {
				if p.Loaded < last.Loaded {
					t.Errorf("progress went back: %d after %d", p.Loaded, last.Loaded)
				}
				last = p
			},
		})
		must(t, err)
		defer func() { must(t, c.Assets.Delete(ctx, asset.ID)) }()
		if asset.Status != "ready" || asset.SizeBytes != int64(len(data)) || last.Percent != 100 {
			t.Errorf("asset %+v, last progress %+v", asset, last)
		}
		if _, err := c.Assets.ContentURL(ctx, asset.ID); err != nil {
			t.Error(err)
		}

		job, err := c.Jobs.Create(ctx, &JobCreateParams{
			Input: AssetInput(asset.ID), Preset: String("web-av1-720p"),
			Metadata: Metadata{"sdk": "go-" + suffix},
		})
		must(t, err)
		page, err := c.Jobs.List(ctx, &JobListParams{Metadata: Metadata{"sdk": "go-" + suffix}})
		must(t, err)
		if len(page.Data) != 1 || page.Data[0].ID != job.ID {
			t.Errorf("metadata filter found %d jobs", len(page.Data))
		}
		events, err := c.Jobs.Events(ctx, job.ID)
		must(t, err)
		if len(events) == 0 {
			t.Error("no job events")
		}
		canceled, err := c.Jobs.Cancel(ctx, job.ID)
		must(t, err)
		if canceled.Status != JobCanceled {
			t.Fatalf("status %s", canceled.Status)
		}
		done, err := c.Jobs.WaitFor(ctx, job.ID, &WaitForOptions{PollInterval: 100 * time.Millisecond, Timeout: 10 * time.Second})
		must(t, err)
		if !done.Status.IsTerminal() {
			t.Errorf("status %s", done.Status)
		}
		must(t, c.Jobs.Delete(ctx, job.ID))
		if _, err := c.Jobs.Get(ctx, job.ID); !IsNotFound(err) {
			t.Errorf("deleted job: %v", err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		_, err := c.Presets.Create(ctx, &PresetCreateParams{Name: "sdk-go bad", Output: RawOutputSpec([]byte(`{"quality":{"target":"high","bitrate":"5M"}}`))})
		e, ok := AsError(err)
		if !ok || e.Status != 422 || e.RequestID == "" || e.Message == "" {
			t.Errorf("err = %v", err)
		}
		bad := NewClient(WithAPIKey("tdk_test_example"))
		if _, err := bad.Auth.Me(ctx); !IsAuthentication(err) {
			t.Errorf("bad key: %v", err)
		}
	})
}
