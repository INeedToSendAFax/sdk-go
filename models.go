package ifax

import (
	"errors"
	"io"
	"os"
	"time"
)

// maxFileBytes mirrors the API's per-file limit (32 MB).
const maxFileBytes = 32 << 20

// ErrFileTooLarge is returned when a file exceeds 32 MB.
var ErrFileTooLarge = errors.New("ifax: file exceeds 32 MB")

// File is a document to fax. Create one with FilePath, FileBytes or
// FileReader. Files are read into memory when the fax is sent so that retries
// can replay the request body.
type File struct {
	Filename string

	data   []byte
	path   string
	reader io.Reader
}

// FilePath references a file on disk, read when the fax is sent.
func FilePath(path string) File {
	return File{Filename: baseName(path), path: path}
}

// FileBytes wraps an in-memory document.
func FileBytes(name string, b []byte) File {
	return File{Filename: name, data: b}
}

// FileReader wraps an io.Reader; it is read fully when the fax is sent.
func FileReader(name string, r io.Reader) File {
	return File{Filename: name, reader: r}
}

func (f *File) resolve() ([]byte, error) {
	if f.data != nil {
		return f.data, nil
	}
	switch {
	case f.path != "":
		b, err := os.ReadFile(f.path)
		if err != nil {
			return nil, err
		}
		if len(b) > maxFileBytes {
			return nil, ErrFileTooLarge
		}
		f.data = b
		return b, nil
	case f.reader != nil:
		b, err := io.ReadAll(io.LimitReader(f.reader, maxFileBytes+1))
		if err != nil {
			return nil, err
		}
		if len(b) > maxFileBytes {
			return nil, ErrFileTooLarge
		}
		f.data = b
		return b, nil
	}
	return nil, errors.New("ifax: empty file")
}

// SendFaxParams describes a fax to send. To and Files are required.
type SendFaxParams struct {
	To    string
	Files []File

	CallbackURL string
	Cover       bool
	CoverText   string

	RecipientName    string
	RecipientCompany string
	SenderName       string
	SenderCompany    string
	SenderPhone      string

	// IdempotencyKey makes retries safe. If empty, one is generated
	// automatically (a retried send will not deliver the fax twice).
	IdempotencyKey string
}

// Fax is a fax job.
type Fax struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	To          string     `json:"to"`
	Pages       int        `json:"pages"`
	PriceCents  int        `json:"price_cents"`
	Bitrate     int        `json:"bitrate"`
	Mode        string     `json:"mode"`
	Error       string     `json:"error"`
	CreatedAt   *time.Time `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// FaxList is a page of recent faxes plus the account balance.
type FaxList struct {
	Data         []Fax `json:"data"`
	BalanceCents int   `json:"balance_cents"`
}

// Account is the response from GET /v1/me.
type Account struct {
	KeyPrefix         string `json:"key_prefix"`
	Email             string `json:"email"`
	BalanceCents      int    `json:"balance_cents"`
	PriceCents        int    `json:"price_cents"`
	MaxPages          int    `json:"max_pages"`
	RateLimitPerMin   int    `json:"rate_limit_per_min"`
	DailyCap          int    `json:"daily_cap"`
	FaxesLast24h      int    `json:"faxes_last_24h"`
	CallbackSecretSet bool   `json:"callback_secret_set"`
}

// Pricing is the response from GET /v1/pricing.
type Pricing struct {
	PriceCents        int      `json:"price_cents"`
	Currency          string   `json:"currency"`
	MaxPages          int      `json:"max_pages"`
	MinCreditCents    int      `json:"min_credit_cents"`
	Countries         []string `json:"countries"`
	CoverAvailable    bool     `json:"cover_available"`
	Callbacks         bool     `json:"callbacks"`
	IdempotencyHeader string   `json:"idempotency_header"`
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
