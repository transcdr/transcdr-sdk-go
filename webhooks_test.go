package transcdr

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWebhooksCreateBodies(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	// HTTPS without a type (backwards compatible).
	f.reply(201, `{"id":"whk_1","type":"https","url":"https://example.com/hooks","secret":"whsec_example","events":["*"]}`)
	w, err := c.Webhooks.Create(ixCtx, &WebhookCreateParams{URL: "https://example.com/hooks", Events: []string{EventJobCompleted}, Description: "Jobs"})
	must(t, err)
	if w.Secret == nil || *w.Secret != "whsec_example" {
		t.Fatalf("secret = %v", w.Secret)
	}
	ixJSONEq(t, f.expect("POST", "/v1/webhooks").Body, `{"url":"https://example.com/hooks","events":["job.completed"],"description":"Jobs"}`)

	// Through a messaging connection.
	_, err = c.Webhooks.Create(ixCtx, &WebhookCreateParams{ConnectionID: "con_1"})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"connection_id":"con_1"}`)

	// SNS with its AWS settings.
	_, err = c.Webhooks.Create(ixCtx, &WebhookCreateParams{
		Type:     WebhookSNS,
		TopicARN: "arn:aws:sns:us-east-1:123456789012:transcdr-events",
		AWS:      &WebhookAWSParams{AccessKeyID: String("AKIAEXAMPLE"), SecretAccessKey: String("secret-example")},
	})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"type":"sns","topic_arn":"arn:aws:sns:us-east-1:123456789012:transcdr-events",
		"aws":{"access_key_id":"AKIAEXAMPLE","secret_access_key":"secret-example"}}`)

	// SQS FIFO with region, endpoint and message group.
	_, err = c.Webhooks.Create(ixCtx, &WebhookCreateParams{
		Type:     WebhookSQS,
		QueueURL: "https://sqs.eu-west-1.amazonaws.com/123456789012/events.fifo",
		AWS: &WebhookAWSParams{
			AccessKeyID: String("AKIAEXAMPLE"), SecretAccessKey: String("secret-example"), Region: String("eu-west-1"),
			Endpoint: Value("https://queue.example.com"), MessageGroupID: Value("transcdr"),
		},
	})
	must(t, err)
	ixJSONEq(t, f.last().Body, `{"type":"sqs","queue_url":"https://sqs.eu-west-1.amazonaws.com/123456789012/events.fifo",
		"aws":{"access_key_id":"AKIAEXAMPLE","secret_access_key":"secret-example","region":"eu-west-1",
		"endpoint":"https://queue.example.com","message_group_id":"transcdr"}}`)
	if f.last().Header.Get("Idempotency-Key") != "" {
		t.Error("webhook creation carries no idempotency key")
	}
}

func TestWebhooksReadUpdateDelete(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, fixture(t, "webhooks.json"))
	page, err := c.Webhooks.List(ixCtx, &ListParams{Limit: 2})
	must(t, err)
	if qv(f.expect("GET", "/v1/webhooks").Query, "limit") != "2" || len(page.Data) != 2 {
		t.Fatalf("list = %+v", page)
	}
	f.reply(200, `{"data":[{"id":"whk_1"}],"has_more":true,"next_cursor":"whk_1"}`).reply(200, `{"data":[{"id":"whk_2"}],"has_more":false}`)
	all, err := Collect(c.Webhooks.All(ixCtx, nil), 0)
	must(t, err)
	if len(all) != 2 {
		t.Fatalf("all = %v", all)
	}

	_, err = c.Webhooks.Get(ixCtx, "whk_1")
	must(t, err)
	f.expect("GET", "/v1/webhooks/whk_1")

	// Update AWS settings without resending the secret; clear the endpoint.
	_, err = c.Webhooks.Update(ixCtx, "whk_1", &WebhookUpdateParams{
		AWS:     &WebhookAWSParams{AccessKeyID: String("AKIANEW"), Endpoint: Null[string](), MessageGroupID: Null[string]()},
		Enabled: Bool(false),
		Events:  []string{"*"},
	})
	must(t, err)
	ixJSONEq(t, f.expect("PATCH", "/v1/webhooks/whk_1").Body,
		`{"aws":{"access_key_id":"AKIANEW","endpoint":null,"message_group_id":null},"events":["*"],"enabled":false}`)

	must(t, c.Webhooks.Delete(ixCtx, "whk_1"))
	f.expect("DELETE", "/v1/webhooks/whk_1")

	f.reply(200, `{"id":"whk_1","secret":"whsec_rotated"}`)
	w, err := c.Webhooks.RotateSecret(ixCtx, "whk_1")
	must(t, err)
	f.expect("POST", "/v1/webhooks/whk_1/rotate-secret")
	if *w.Secret != "whsec_rotated" {
		t.Fatalf("secret = %v", *w.Secret)
	}
}

func TestWebhooksTestCheckDeliveries(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()

	f.reply(200, `{"object":"webhook_delivery","id":"whd_1","status":"succeeded","response_status":200}`)
	raw, err := c.Webhooks.Test(ixCtx, "whk_1")
	must(t, err)
	f.expect("POST", "/v1/webhooks/whk_1/test")
	var d WebhookDelivery
	must(t, json.Unmarshal(raw, &d))
	if d.ID != "whd_1" || *d.ResponseStatus != 200 {
		t.Fatalf("delivery = %+v", d)
	}

	f.reply(200, `{"object":"webhook_check","ok":true,"steps":[{"id":"identity","label":"Credentials","status":"passed"},{"id":"publish","label":"Publish","status":"passed","duration_ms":42}],"identity":{"provider":"aws","arn":"arn:aws:iam::123456789012:user/x","account":"123456789012"},"setup":null,"roles":{"notifications":true}}`)
	check, err := c.Webhooks.Check(ixCtx, &WebhookCreateParams{Type: WebhookSNS, TopicARN: "arn:aws:sns:us-east-1:123456789012:t",
		AWS: &WebhookAWSParams{AccessKeyID: String("AKIAEXAMPLE"), SecretAccessKey: String("secret-example")}})
	must(t, err)
	ixJSONEq(t, f.expect("POST", "/v1/webhooks/check").Body, `{"type":"sns","topic_arn":"arn:aws:sns:us-east-1:123456789012:t","aws":{"access_key_id":"AKIAEXAMPLE","secret_access_key":"secret-example"}}`)
	if !check.OK || len(check.Steps) != 2 || *check.Steps[1].DurationMs != 42 || !*check.Roles.Notifications || check.Endpoint != nil {
		t.Fatalf("check = %+v", check)
	}

	f.reply(200, `{"object":"webhook_check","ok":false,"steps":[{"id":"deliver","label":"Deliver","status":"failed","hint":"Answer 2xx."}],"identity":null,"setup":null,"roles":{"notifications":false},"endpoint":{"id":"whk_1"}}`)
	saved, err := c.Webhooks.CheckSaved(ixCtx, "whk_1")
	must(t, err)
	f.expect("POST", "/v1/webhooks/whk_1/check")
	if saved.OK || saved.Endpoint.ID != "whk_1" || *saved.Steps[0].Hint != "Answer 2xx." {
		t.Fatalf("saved = %+v", saved)
	}

	f.reply(200, `{"data":[{"id":"whd_1","endpoint_id":"whk_1","event_id":"evt_1","event_type":"job.completed","status":"failed","attempts":3,"response_status":null,"response_body":null,"duration_ms":null,"next_retry_at":"2026-09-27T12:00:00Z","created_at":"2026-09-27T11:00:00Z"}],"has_more":false}`)
	dels, err := c.Webhooks.Deliveries(ixCtx, "whk_1", &ListParams{Limit: 1})
	must(t, err)
	if qv(f.expect("GET", "/v1/webhooks/whk_1/deliveries").Query, "limit") != "1" || dels.Data[0].NextRetryAt == nil || dels.Data[0].ResponseStatus != nil {
		t.Fatalf("deliveries = %+v", dels)
	}
	f.reply(200, `{"data":[{"id":"whd_1"}],"has_more":true,"next_cursor":"whd_1"}`).reply(200, `{"data":[{"id":"whd_0"}],"has_more":false}`)
	alld, err := Collect(c.Webhooks.AllDeliveries(ixCtx, "whk_1", nil), 0)
	must(t, err)
	if len(alld) != 2 || qv(f.last().Query, "cursor") != "whd_1" {
		t.Fatalf("all deliveries = %v", alld)
	}

	f.reply(200, `{"id":"whd_2","status":"pending"}`)
	re, err := c.Webhooks.Redeliver(ixCtx, "whd_1")
	must(t, err)
	f.expect("POST", "/v1/webhook-deliveries/whd_1/redeliver")
	if re.ID != "whd_2" {
		t.Fatalf("redeliver = %+v", re)
	}
}

func TestWebhooksServiceVerifyHelpers(t *testing.T) {
	c := NewClient(WithAPIKey("tdk_test_example"))
	now := time.Unix(sigTimestamp, 0)
	if !c.Webhooks.VerifySignature([]byte(sigBody), sigHeader, sigSecret, WithNow(now)) {
		t.Error("VerifySignature")
	}
	attrs := map[string]any{"transcdr-signature": map[string]any{"StringValue": sigHeader}}
	if !c.Webhooks.VerifySNSSQSSignature([]byte(sigBody), attrs, sigSecret, WithNow(now)) {
		t.Error("VerifySNSSQSSignature")
	}
	e, err := c.Webhooks.ConstructEvent([]byte(sigBody), sigHeader, sigSecret, WithNow(now))
	if err != nil || e.ID != "evt_1" {
		t.Fatalf("ConstructEvent = %v, %v", e, err)
	}
}

func TestDecodeWebhooksFixture(t *testing.T) {
	var page Page[WebhookEndpoint]
	must(t, json.Unmarshal([]byte(fixture(t, "webhooks.json")), &page))
	if len(page.Data) != 2 {
		t.Fatalf("webhooks = %d", len(page.Data))
	}
	q, h := page.Data[0], page.Data[1]
	if q.ID != "whk_IU3Gzfu0tau7hMKGRF1MV0" || q.Type != WebhookSQS || *q.ConnectionID != "con_tPnA0CKgVVoSDXJiK5eMIC" ||
		q.QueueURL == nil || q.AWS == nil || q.AWS.Endpoint != nil || q.Secret != nil || q.Events[0] != "*" {
		t.Fatalf("sqs endpoint = %+v", q)
	}
	if h.Type != WebhookHTTPS || h.URL != "https://example.com/transcdr-hooks" || h.ConnectionID != nil || len(h.Events) != 2 {
		t.Fatalf("https endpoint = %+v", h)
	}
}

func TestDecodeEventsFixture(t *testing.T) {
	var page Page[Event]
	must(t, json.Unmarshal([]byte(fixture(t, "events.json")), &page))
	if len(page.Data) != 10 {
		t.Fatalf("events = %d", len(page.Data))
	}
	e := page.Data[0]
	if e.ID != "evt_Fae2PdyrEU4LTs8Kq83eIK" || e.Type != EventJobCompleted || e.CreatedAt.IsZero() {
		t.Fatalf("event = %+v", e)
	}
	job, err := e.Data.Job()
	must(t, err)
	if job.ID != "job_zTyZl9x2xJzbyp8ScuyQRm" || job.Status != JobCompleted || !job.Status.IsTerminal() || job.Billing == nil || job.Billing.AmountCents != 1 {
		t.Fatalf("job = %+v", job)
	}
	started, err := page.Data[1].Data.Job()
	must(t, err)
	if started.Status != JobRunning || started.Status.IsTerminal() {
		t.Fatalf("started = %+v", started.Status)
	}
	var generic map[string]any
	must(t, e.Data.Decode(&generic))
	if generic["id"] != job.ID {
		t.Fatal("Decode")
	}
}

func TestEventDataConnectionAndAsset(t *testing.T) {
	d := EventData{Object: json.RawMessage(`{"object":"connection","id":"con_1","name":"Ingest","kind":"s3","enabled":false,
		"disabled_at":"2026-09-27T10:00:00Z","disabled_reason":"listing: access denied","activity":"listing","error":"access denied",
		"explanation":"The key lost s3:ListBucket.","automations":["aut_1"]}`)}
	c, err := d.Connection()
	must(t, err)
	if c.ID != "con_1" || c.Enabled || c.Activity != "listing" || len(c.Automations) != 1 {
		t.Fatalf("connection = %+v", c)
	}
	a, err := EventData{Object: json.RawMessage(`{"id":"ast_1","status":"ready","size_bytes":5}`)}.Asset()
	must(t, err)
	if a.ID != "ast_1" || a.SizeBytes != 5 {
		t.Fatalf("asset = %+v", a)
	}
}
