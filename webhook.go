package ifax

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// DefaultTolerance is the recommended replay window for webhook timestamps.
const DefaultTolerance = 5 * time.Minute

// Webhook verification errors.
var (
	ErrMalformedSignatureHeader  = errors.New("ifax: malformed X-Fax-Signature header")
	ErrInvalidSignature          = errors.New("ifax: webhook signature does not match")
	ErrTimestampOutsideTolerance = errors.New("ifax: webhook timestamp outside tolerance")
)

// Event is the JSON body of a fax status callback.
type Event struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	To          string     `json:"to"`
	Pages       int        `json:"pages"`
	Bitrate     int        `json:"bitrate"`
	Mode        string     `json:"mode"`
	Error       string     `json:"error"`
	CompletedAt *time.Time `json:"completed_at"`
}

// VerifyWebhook checks the X-Fax-Signature header against the raw request body
// using your webhook signing secret. Pass the header value verbatim and the
// unmodified body. A tolerance of 0 uses DefaultTolerance; use a negative value
// to skip the timestamp check.
//
// Return values are ErrMalformedSignatureHeader, ErrTimestampOutsideTolerance,
// or ErrInvalidSignature, so callers can distinguish a bad shape from a bad
// signature from an old replay.
func VerifyWebhook(secret, signatureHeader string, body []byte, tolerance time.Duration) error {
	if secret == "" {
		return errors.New("ifax: empty webhook secret")
	}
	if tolerance == 0 {
		tolerance = DefaultTolerance
	}
	ts, sig, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return err
	}
	if tolerance > 0 {
		t, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return ErrMalformedSignatureHeader
		}
		age := time.Since(time.Unix(t, 0))
		if age > tolerance || age < -tolerance {
			return ErrTimestampOutsideTolerance
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return ErrInvalidSignature
	}
	return nil
}

// ParseEvent decodes a callback body. Call VerifyWebhook first when the request
// came from the network.
func ParseEvent(body []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func parseSignatureHeader(header string) (ts, sig string, err error) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sig = kv[1]
		}
	}
	if ts == "" || sig == "" {
		return "", "", ErrMalformedSignatureHeader
	}
	return ts, sig, nil
}
