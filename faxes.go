package ifax

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxFiles mirrors the API's limit of 10 combined attachments.
const maxFiles = 10

// SendFax submits a fax. It returns a Fax with status "queued"; poll GetFax or
// use a callback_url to learn when it is delivered. A retried call is safe:
// if IdempotencyKey is empty one is generated automatically.
func (c *Client) SendFax(ctx context.Context, p *SendFaxParams) (*Fax, error) {
	if p == nil {
		return nil, errors.New("ifax: nil SendFaxParams")
	}
	if strings.TrimSpace(p.To) == "" {
		return nil, errors.New("ifax: To is required")
	}
	if len(p.Files) == 0 {
		return nil, errors.New("ifax: at least one file is required")
	}
	if len(p.Files) > maxFiles {
		return nil, fmt.Errorf("ifax: at most %d files, got %d", maxFiles, len(p.Files))
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("to", p.To); err != nil {
		return nil, err
	}
	set := func(field, value string) {
		if value != "" {
			_ = mw.WriteField(field, value)
		}
	}
	set("callback_url", p.CallbackURL)
	set("cover_text", p.CoverText)
	set("recipient_name", p.RecipientName)
	set("recipient_company", p.RecipientCompany)
	set("sender_name", p.SenderName)
	set("sender_company", p.SenderCompany)
	set("sender_phone", p.SenderPhone)
	if p.Cover {
		_ = mw.WriteField("cover", "1")
	}

	for i := range p.Files {
		data, err := p.Files[i].resolve()
		if err != nil {
			return nil, fmt.Errorf("ifax: reading %q: %w", p.Files[i].Filename, err)
		}
		fw, err := mw.CreateFormFile("file", p.Files[i].Filename)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	key := p.IdempotencyKey
	if key == "" {
		key = randomKey()
	}
	headers := map[string]string{"Idempotency-Key": key}

	var fax Fax
	if err := c.do(ctx, http.MethodPost, "/v1/faxes", buf.Bytes(), mw.FormDataContentType(), headers, &fax); err != nil {
		return nil, err
	}
	return &fax, nil
}

// GetFax returns a single fax by id. A 404 is returned as an *APIError that
// satisfies IsNotFound.
func (c *Client) GetFax(ctx context.Context, id string) (*Fax, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("ifax: id is required")
	}
	var fax Fax
	if err := c.do(ctx, http.MethodGet, "/v1/faxes/"+url.PathEscape(id), nil, "", nil, &fax); err != nil {
		return nil, err
	}
	return &fax, nil
}

// ListFaxes returns recent faxes for the account. limit is clamped by the API
// to 1..200; pass 0 for the default (50).
func (c *Client) ListFaxes(ctx context.Context, limit int) (*FaxList, error) {
	path := "/v1/faxes"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out FaxList
	if err := c.do(ctx, http.MethodGet, path, nil, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
