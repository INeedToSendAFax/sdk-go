# Changelog

## 1.0.0

Initial release.

- `SendFax`, `GetFax`, `ListFaxes`, `Me`, `Pricing`
- Automatic idempotency keys and retries with backoff
- Typed `*APIError` with `IsNotFound`, `IsInsufficientCredits`, `IsRateLimited`
- `VerifyWebhook` and `ParseEvent` for signed callbacks
