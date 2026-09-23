package ifax_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	ifax "github.com/INeedToSendAFax/sdk-go"
)

// ExampleClient_SendFax shows how to send a fax with a callback.
func ExampleClient_SendFax() {
	client := ifax.New(os.Getenv("IFAX_API_KEY"))

	fax, err := client.SendFax(context.Background(), &ifax.SendFaxParams{
		To:          "15551234567",
		Files:       []ifax.File{ifax.FilePath("invoice.pdf")},
		CallbackURL: "https://example.com/fax-webhook",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("queued:", fax.ID)
}

// ExampleClient_GetFax polls a fax for its final status.
func ExampleClient_GetFax() {
	client := ifax.New(os.Getenv("IFAX_API_KEY"))

	fax, err := client.GetFax(context.Background(), "b1a2c3d4-0000-0000-0000-000000000000")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	if fax.Status == "sent" {
		fmt.Println("delivered")
	}
}

// ExampleVerifyWebhook verifies a signed callback, then parses it.
func ExampleVerifyWebhook() {
	const secret = "whsec_example"
	body := []byte(`{"id":"abc","status":"sent"}`)
	ts := "1700000000"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	header := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))

	// A negative tolerance skips the timestamp/replay check for this example.
	if err := ifax.VerifyWebhook(secret, header, body, -1); err != nil {
		fmt.Println("invalid:", err)
		return
	}
	event, _ := ifax.ParseEvent(body)
	fmt.Println("verified", event.ID, event.Status)
	// Output: verified abc sent
}

// ExampleVerifyWebhook_stale rejects an old signature to prevent replays.
func ExampleVerifyWebhook_stale() {
	const secret = "whsec_example"
	body := []byte(`{"id":"abc"}`)
	header := "t=1700000000,v1=0000"

	err := ifax.VerifyWebhook(secret, header, body, 5*time.Minute)
	fmt.Println(err == ifax.ErrTimestampOutsideTolerance)
	// Output: true
}
