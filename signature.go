package transcdr

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader carries the signature of every HTTPS delivery:
// "t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>".
const SignatureHeader = "Transcdr-Signature"

// SignatureAttribute is the message attribute carrying the same signature on
// SNS and SQS deliveries.
const SignatureAttribute = "transcdr-signature"

// DefaultTolerance is how far a signature's timestamp may be from now.
const DefaultTolerance = 300 * time.Second

// ComputeSignature is HMAC-SHA256 of "<timestamp>.<payload>" under secret,
// as lowercase hex.
func ComputeSignature(payload []byte, secret string, timestamp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignPayload builds a Transcdr-Signature header value, for testing a
// receiver. A zero timestamp means now.
func SignPayload(payload []byte, secret string, timestamp int64) string {
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	return fmt.Sprintf("t=%d,v1=%s", timestamp, ComputeSignature(payload, secret, timestamp))
}

var digits = regexp.MustCompile(`^\d+$`)

// ParseSignatureHeader reads "t=…,v1=…[,v1=…]". ok is false without a valid
// timestamp.
func ParseSignatureHeader(header string) (timestamp int64, signatures []string, ok bool) {
	ok = false
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "t":
			if digits.MatchString(value) {
				if t, err := strconv.ParseInt(value, 10, 64); err == nil {
					timestamp, ok = t, true
				}
			}
		case "v1":
			signatures = append(signatures, strings.ToLower(value))
		}
	}
	return timestamp, signatures, ok
}

type verifyConfig struct {
	tolerance time.Duration
	now       time.Time
}

// VerifyOption tunes signature verification.
type VerifyOption func(*verifyConfig)

// WithTolerance sets how far the timestamp may be from now (default 300 s).
func WithTolerance(d time.Duration) VerifyOption { return func(v *verifyConfig) { v.tolerance = d } }

// WithNow overrides the current time, for tests.
func WithNow(t time.Time) VerifyOption { return func(v *verifyConfig) { v.now = t } }

// VerifySignature reports whether header is a valid signature of the raw
// request body payload under secret. Pass the body exactly as received,
// before any JSON parsing. Any matching v1 (several during a rotation)
// verifies.
func VerifySignature(payload []byte, header, secret string, opts ...VerifyOption) bool {
	if header == "" || secret == "" {
		return false
	}
	cfg := verifyConfig{tolerance: DefaultTolerance}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.now.IsZero() {
		cfg.now = time.Now()
	}
	timestamp, signatures, ok := ParseSignatureHeader(header)
	if !ok || len(signatures) == 0 {
		return false
	}
	skew := cfg.now.Unix() - timestamp
	if skew < 0 {
		skew = -skew
	}
	if time.Duration(skew)*time.Second > cfg.tolerance {
		return false
	}
	expected := []byte(ComputeSignature(payload, secret, timestamp))
	for _, s := range signatures {
		if subtle.ConstantTimeCompare([]byte(s), expected) == 1 {
			return true
		}
	}
	return false
}

// ConstructEvent verifies a delivery, then parses it into an [Event]. It
// returns [ErrInvalidSignature] when the signature does not verify.
func ConstructEvent(payload []byte, header, secret string, opts ...VerifyOption) (*Event, error) {
	if !VerifySignature(payload, header, secret, opts...) {
		return nil, ErrInvalidSignature
	}
	var e Event
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// SignatureFromAttributes reads the transcdr-signature value out of an SNS or
// SQS message-attribute map in any AWS shape: SQS ReceiveMessage and the AWS
// SDKs (StringValue), Lambda SQS events (stringValue), SNS notification JSON
// (Value), or plain strings. attributes may be a map (decoded JSON, or
// map[string]string), raw JSON, or the attribute value itself as a string.
func SignatureFromAttributes(attributes any) string {
	switch a := attributes.(type) {
	case nil:
		return ""
	case string:
		return a
	case []byte:
		return signatureFromJSON(a)
	case json.RawMessage:
		return signatureFromJSON(a)
	case map[string]string:
		for k, v := range a {
			if strings.EqualFold(k, SignatureAttribute) {
				return v
			}
		}
	case map[string]any:
		for k, v := range a {
			if strings.EqualFold(k, SignatureAttribute) {
				return attributeValue(v)
			}
		}
	default:
		// Struct maps such as the AWS SDK's: go through JSON.
		if b, err := json.Marshal(a); err == nil {
			return signatureFromJSON(b)
		}
	}
	return ""
}

func signatureFromJSON(b []byte) string {
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		return SignatureFromAttributes(m)
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s
	}
	return ""
}

func attributeValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		for _, key := range []string{"StringValue", "stringValue", "Value"} {
			if s, ok := x[key].(string); ok {
				return s
			}
		}
	}
	return ""
}

// VerifySNSSQSSignature verifies an Amazon SNS or SQS delivery: the
// transcdr-signature attribute is "t=<unix>,v1=<hmac>" over
// "<t>.<message>". message is the SNS Message or the SQS body (Lambda:
// record.body) exactly as received; attributes is the message-attribute map
// in any AWS shape, or the attribute value (see [SignatureFromAttributes]).
// With raw message delivery off, an SNS to SQS subscription wraps the
// notification: parse the body and pass its Message and MessageAttributes.
func VerifySNSSQSSignature(message []byte, attributes any, secret string, opts ...VerifyOption) bool {
	return VerifySignature(message, SignatureFromAttributes(attributes), secret, opts...)
}
