package transcdr

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// The vector shared with the TypeScript SDK (test/signature.test.ts):
// hmac_sha256("whsec_test_secret", '1700000000.{"id":"evt_1","type":"job.completed"}').
const (
	sigSecret    = "whsec_test_secret"
	sigTimestamp = int64(1_700_000_000)
	sigBody      = `{"id":"evt_1","type":"job.completed"}`
	sigExpected  = "76323e66a6eb95011512d61a834db013378975ecf841d3d43eb14fcb08fbb0e4"
	sigHeader    = "t=1700000000,v1=" + sigExpected
)

func sigAt(offset int64) VerifyOption { return WithNow(time.Unix(sigTimestamp+offset, 0)) }

func TestComputeSignatureVector(t *testing.T) {
	if got := ComputeSignature([]byte(sigBody), sigSecret, sigTimestamp); got != sigExpected {
		t.Fatalf("ComputeSignature = %s", got)
	}
	if got := SignPayload([]byte(sigBody), sigSecret, sigTimestamp); got != sigHeader {
		t.Fatalf("SignPayload = %s", got)
	}
}

func TestVerifyWithinTolerance(t *testing.T) {
	if !VerifySignature([]byte(sigBody), sigHeader, sigSecret, sigAt(100)) {
		t.Error("valid header 100 s later")
	}
	if !VerifySignature([]byte(sigBody), sigHeader, sigSecret, sigAt(-100)) {
		t.Error("valid header 100 s earlier")
	}
	if !VerifySignature([]byte(sigBody), sigHeader, sigSecret, sigAt(0)) {
		t.Error("valid header at its time")
	}
	if !VerifySignature([]byte(sigBody), sigHeader, sigSecret, sigAt(1000), WithTolerance(time.Hour)) {
		t.Error("custom tolerance")
	}
}

func TestVerifyRejects(t *testing.T) {
	if VerifySignature([]byte(sigBody), sigHeader, sigSecret, sigAt(301)) {
		t.Error("stale timestamp accepted")
	}
	if VerifySignature([]byte(sigBody), sigHeader, "whsec_other", sigAt(0)) {
		t.Error("wrong secret accepted")
	}
	if VerifySignature([]byte(strings.Replace(sigBody, "evt_1", "evt_2", 1)), sigHeader, sigSecret, sigAt(0)) {
		t.Error("tampered body accepted")
	}
	if VerifySignature([]byte(sigBody), sigHeader, "", sigAt(0)) {
		t.Error("empty secret accepted")
	}
}

func TestVerifyRejectsMalformedHeaders(t *testing.T) {
	for _, header := range []string{"", "garbage", "t=1700000000", "v1=" + sigExpected, "t=abc,v1=" + sigExpected} {
		if VerifySignature([]byte(sigBody), header, sigSecret, sigAt(0)) {
			t.Errorf("accepted %q", header)
		}
	}
}

func TestVerifyAnyOfSeveral(t *testing.T) {
	header := "t=1700000000,v1=" + strings.Repeat("0", 64) + ",v1=" + sigExpected
	if !VerifySignature([]byte(sigBody), header, sigSecret, sigAt(0)) {
		t.Error("rotation: a matching second v1 must verify")
	}
	// Upper-case hex and spaces are tolerated, as in the TS parser.
	if !VerifySignature([]byte(sigBody), " t=1700000000 , v1="+strings.ToUpper(sigExpected), sigSecret, sigAt(0)) {
		t.Error("case and spacing")
	}
}

func TestVerifyDefaultsToNow(t *testing.T) {
	header := SignPayload([]byte(sigBody), sigSecret, 0)
	if !VerifySignature([]byte(sigBody), header, sigSecret) {
		t.Error("a fresh signature must verify against the current time")
	}
	if VerifySignature([]byte(sigBody), sigHeader, sigSecret) {
		t.Error("the 2023 vector is stale now")
	}
}

func TestParseSignatureHeader(t *testing.T) {
	ts, sigs, ok := ParseSignatureHeader("t=1700000000,v1=AB,v1=cd,x=1,junk")
	if !ok || ts != sigTimestamp || len(sigs) != 2 || sigs[0] != "ab" || sigs[1] != "cd" {
		t.Fatalf("parse = %d %v %v", ts, sigs, ok)
	}
	if _, _, ok := ParseSignatureHeader("v1=ab"); ok {
		t.Error("no timestamp must not parse")
	}
}

func TestConstructEvent(t *testing.T) {
	e, err := ConstructEvent([]byte(sigBody), sigHeader, sigSecret, sigAt(0))
	if err != nil || e.ID != "evt_1" || e.Type != EventJobCompleted {
		t.Fatalf("event = %+v, err = %v", e, err)
	}
	if _, err := ConstructEvent([]byte(sigBody), sigHeader, "nope", sigAt(0)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v", err)
	}
	// A verified payload that is not JSON is a decoding error, not a signature one.
	bad := []byte("not json")
	if _, err := ConstructEvent(bad, SignPayload(bad, sigSecret, sigTimestamp), sigSecret, sigAt(0)); err == nil || errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignatureFromAttributesShapes(t *testing.T) {
	type sdkValue struct {
		DataType    *string
		StringValue *string
	}
	str := "String"
	value := sigHeader
	cases := map[string]any{
		"SQS ReceiveMessage": map[string]any{"transcdr-signature": map[string]any{"DataType": "String", "StringValue": sigHeader}},
		"Lambda SQS event":   map[string]any{"transcdr-signature": map[string]any{"stringValue": sigHeader, "dataType": "String"}},
		"SNS notification":   map[string]any{"transcdr-signature": map[string]any{"Type": "String", "Value": sigHeader}},
		"case-insensitive":   map[string]any{"Transcdr-Signature": map[string]any{"StringValue": sigHeader}},
		"plain map":          map[string]string{"transcdr-signature": sigHeader},
		"plain value in map": map[string]any{"transcdr-signature": sigHeader},
		"raw JSON":           []byte(`{"transcdr-signature":{"DataType":"String","StringValue":"` + sigHeader + `"}}`),
		"json.RawMessage":    json.RawMessage(`{"transcdr-signature":{"Type":"String","Value":"` + sigHeader + `"}}`),
		"bare value":         sigHeader,
		"AWS SDK struct map": map[string]sdkValue{"transcdr-signature": {DataType: &str, StringValue: &value}},
	}
	for name, attrs := range cases {
		if got := SignatureFromAttributes(attrs); got != sigHeader {
			t.Errorf("%s: got %q", name, got)
		}
		if !VerifySNSSQSSignature([]byte(sigBody), attrs, sigSecret, sigAt(0)) {
			t.Errorf("%s: does not verify", name)
		}
	}
	for name, attrs := range map[string]any{
		"nil":           nil,
		"missing":       map[string]any{"transcdr-event-type": map[string]any{"StringValue": "job.completed"}},
		"empty map":     map[string]string{},
		"no value keys": map[string]any{"transcdr-signature": map[string]any{"DataType": "String"}},
	} {
		if got := SignatureFromAttributes(attrs); got != "" {
			t.Errorf("%s: got %q", name, got)
		}
		if VerifySNSSQSSignature([]byte(sigBody), attrs, sigSecret, sigAt(0)) {
			t.Errorf("%s: verified", name)
		}
	}
}

func TestVerifySNSSQSRejects(t *testing.T) {
	attrs := map[string]any{"transcdr-signature": map[string]any{"StringValue": sigHeader}}
	if VerifySNSSQSSignature([]byte(sigBody), attrs, "whsec_other", sigAt(0)) {
		t.Error("wrong secret")
	}
	if VerifySNSSQSSignature([]byte(sigBody+" "), attrs, sigSecret, sigAt(0)) {
		t.Error("tampered message")
	}
	if VerifySNSSQSSignature([]byte(sigBody), attrs, sigSecret, sigAt(301)) {
		t.Error("stale")
	}
}
