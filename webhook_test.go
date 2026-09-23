package ifax

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"
)

func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookValid(t *testing.T) {
	const secret = "whsec_test"
	body := []byte(`{"id":"abc","status":"sent","pages":2}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	header := "t=" + ts + ",v1=" + sign(secret, ts, body)

	if err := VerifyWebhook(secret, header, body, 0); err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
}

func TestVerifyWebhookFailures(t *testing.T) {
	const secret = "whsec_test"
	body := []byte(`{"id":"abc"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	if err := VerifyWebhook("wrong", "t="+ts+",v1="+sign(secret, ts, body), body, 0); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("wrong secret: got %v, want ErrInvalidSignature", err)
	}

	old := "1700000000"
	if err := VerifyWebhook(secret, "t="+old+",v1="+sign(secret, old, body), body, time.Minute); !errors.Is(err, ErrTimestampOutsideTolerance) {
		t.Errorf("stale: got %v, want ErrTimestampOutsideTolerance", err)
	}

	if err := VerifyWebhook(secret, "garbage", body, 0); !errors.Is(err, ErrMalformedSignatureHeader) {
		t.Errorf("malformed: got %v, want ErrMalformedSignatureHeader", err)
	}

	if err := VerifyWebhook("", "t="+ts+",v1=x", body, 0); err == nil {
		t.Error("empty secret: expected error")
	}
}

func TestVerifyWebhookTamperedBody(t *testing.T) {
	const secret = "whsec_test"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	header := "t=" + ts + ",v1=" + sign(secret, ts, []byte(`{"id":"abc"}`))
	if err := VerifyWebhook(secret, header, []byte(`{"id":"evil"}`), 0); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("tampered body: got %v, want ErrInvalidSignature", err)
	}
}

func TestParseEvent(t *testing.T) {
	e, err := ParseEvent([]byte(`{"id":"abc","status":"failed","error":"no answer","completed_at":"2026-09-23T06:17:37Z"}`))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if e.ID != "abc" || e.Status != "failed" || e.Error != "no answer" || e.CompletedAt == nil {
		t.Errorf("unexpected event: %+v", e)
	}
}
