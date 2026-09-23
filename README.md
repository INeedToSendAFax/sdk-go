# INeedToSendAFax Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/INeedToSendAFax/sdk-go.svg)](https://pkg.go.dev/github.com/INeedToSendAFax/sdk-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Official Go client for the [INeedToSendAFax API](https://api.ineedtosendafax.com/docs). Send faxes, track delivery, and verify webhook callbacks.

> Using Python? The Python SDK is [ineedtosendafax on PyPI](https://pypi.org/project/ineedtosendafax/) (`pip install ineedtosendafax`), source at [INeedToSendAFax/sdk-python](https://github.com/INeedToSendAFax/sdk-python).

- Zero dependencies (standard library only)
- Context-first, safe for concurrent use
- Automatic idempotency keys and retries for transient failures
- Typed errors and webhook signature verification

## Install

```bash
go get github.com/INeedToSendAFax/sdk-go
```

The module path ends in `sdk-go`, but the package name is `ifax`, so it imports as:

```go
import "github.com/INeedToSendAFax/sdk-go"
```

## Quick start

Grab an API key from [api.ineedtosendafax.com](https://api.ineedtosendafax.com/) and set it as `IFAX_API_KEY`.

```go
package main

import (
	"context"
	"log"

	ifax "github.com/INeedToSendAFax/sdk-go"
)

func main() {
	client, err := ifax.NewFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	fax, err := client.SendFax(context.Background(), &ifax.SendFaxParams{
		To:    "15551234567",
		Files: []ifax.File{ifax.FilePath("invoice.pdf")},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("queued:", fax.ID)
}
```

## Sending a fax

`SendFax` accepts PDF, Word (DOC/DOCX), PNG/JPEG, PostScript, and TIFF, up to 10 files combined into one fax and 32 MB each. Pass a callback URL to be notified on completion, and set `Cover` for a generated cover page.

```go
fax, err := client.SendFax(ctx, &ifax.SendFaxParams{
	To:           "15551234567",
	Files:        []ifax.File{ifax.FilePath("report.pdf"), ifax.FilePath("appendix.pdf")},
	CallbackURL:  "https://example.com/fax-webhook",
	Cover:        true,
	CoverText:    "Confidential",
	SenderName:   "Acme Billing",
	RecipientName: "Dr. Smith",
})
```

Files can come from memory or any reader:

```go
ifax.FileBytes("note.txt", []byte("hello"))
ifax.FileReader("note.txt", myReader)
```

## Tracking status

```go
fax, err := client.GetFax(ctx, "b1a2c3d4-...")
// fax.Status is queued, sending, sent, or failed.

list, err := client.ListFaxes(ctx, 50)   // list.Data, list.BalanceCents
acct, err := client.Me(ctx)              // acct.BalanceCents, acct.MaxPages, ...
price, err := client.Pricing(ctx)
```

## Verifying webhooks

When you set `callback_url`, we POST the final status with an `X-Fax-Signature` header. Verify it against the raw request body using your webhook signing secret.

```go
http.HandleFunc("/fax-webhook", func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	if err := ifax.VerifyWebhook(secret, r.Header.Get("X-Fax-Signature"), body, 0); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	event, _ := ifax.ParseEvent(body)
	log.Printf("fax %s is %s", event.ID, event.Status)
	w.WriteHeader(http.StatusOK)
})
```

`VerifyWebhook` uses `DefaultTolerance` (5 minutes) when passed `0`, and uses a constant-time comparison. It returns `ErrInvalidSignature`, `ErrTimestampOutsideTolerance`, or `ErrMalformedSignatureHeader`, so you can tell a bad signature from a replayed request.

## Retries and idempotency

The client retries `429` and `5xx` responses and network errors with backoff, honoring `Retry-After`. `SendFax` always sends an `Idempotency-Key` (generated when you do not provide one), so a retried send never delivers a fax twice.

```go
client := ifax.New(key, ifax.WithMaxRetries(3), ifax.WithTimeout(90*time.Second))
```

## Errors

All non-2xx responses become an `*APIError`:

```go
if ifax.IsInsufficientCredits(err) {
	// buy more credit
}
if ifax.IsRateLimited(err) {
	// back off and retry
}
if ifax.IsNotFound(err) {
	// unknown fax id
}

var apiErr *ifax.APIError
if errors.As(err, &apiErr) {
	log.Println(apiErr.StatusCode, apiErr.Code, apiErr.Message)
}
```

## License

MIT
