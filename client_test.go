package ifax

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendFax(t *testing.T) {
	var gotAuth, gotIdem, gotTo, gotCover string
	var fileCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/faxes" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotency-Key")
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart: %v", err)
		}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("part: %v", err)
			}
			switch p.FormName() {
			case "file":
				fileCount++
			case "to":
				b, _ := io.ReadAll(p)
				gotTo = string(b)
			case "cover":
				b, _ := io.ReadAll(p)
				gotCover = string(b)
			}
			io.Copy(io.Discard, p)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"id":"abc-123","status":"queued","to":"15551234567","pages":1,"price_cents":149}`)
	}))
	defer srv.Close()

	c := New("ifx_live_test", WithBaseURL(srv.URL))
	fax, err := c.SendFax(context.Background(), &SendFaxParams{
		To:    "15551234567",
		Cover: true,
		Files: []File{
			FileBytes("a.pdf", []byte("hello")),
			FileBytes("b.pdf", []byte("world")),
		},
	})
	if err != nil {
		t.Fatalf("SendFax: %v", err)
	}
	if fax.ID != "abc-123" || fax.Status != "queued" || fax.PriceCents != 149 {
		t.Errorf("unexpected fax: %+v", fax)
	}
	if gotAuth != "Bearer ifx_live_test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotIdem == "" {
		t.Errorf("expected an auto-generated Idempotency-Key")
	}
	if gotTo != "15551234567" || gotCover != "1" || fileCount != 2 {
		t.Errorf("to=%q cover=%q files=%d", gotTo, gotCover, fileCount)
	}
}

func TestSendFaxValidation(t *testing.T) {
	c := New("k")
	if _, err := c.SendFax(context.Background(), nil); err == nil {
		t.Error("expected error for nil params")
	}
	if _, err := c.SendFax(context.Background(), &SendFaxParams{To: "1"}); err == nil {
		t.Error("expected error for no files")
	}
	tooMany := make([]File, maxFiles+1)
	for i := range tooMany {
		tooMany[i] = FileBytes("x.pdf", []byte("x"))
	}
	if _, err := c.SendFax(context.Background(), &SendFaxParams{To: "1", Files: tooMany}); err == nil {
		t.Error("expected error for too many files")
	}
}

func TestGetFaxAndNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":{"code":"not_found","message":"Fax not found."}}`)
			return
		}
		io.WriteString(w, `{"id":"abc","status":"sent","to":"1555","pages":2,"bitrate":9600,"mode":"V17"}`)
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	fax, err := c.GetFax(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetFax: %v", err)
	}
	if fax.Status != "sent" || fax.Pages != 2 || fax.Mode != "V17" {
		t.Errorf("unexpected fax: %+v", fax)
	}

	_, err = c.GetFax(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestInsufficientCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		io.WriteString(w, `{"error":{"code":"insufficient_credits","message":"Not enough credit."}}`)
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	_, err := c.SendFax(context.Background(), &SendFaxParams{To: "1", Files: []File{FileBytes("a", []byte("a"))}})
	if !IsInsufficientCredits(err) {
		t.Fatalf("expected IsInsufficientCredits, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 402 || apiErr.Message == "" {
		t.Errorf("unexpected APIError: %+v", apiErr)
	}
}

func TestRetryOnServerError(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"error":{"code":"server_error","message":"boom"}}`)
			return
		}
		io.WriteString(w, `{"id":"ok","status":"queued"}`)
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL), WithMaxRetries(2))
	fax, err := c.SendFax(context.Background(), &SendFaxParams{To: "1", Files: []File{FileBytes("a", []byte("a"))}})
	if err != nil {
		t.Fatalf("SendFax: %v", err)
	}
	if attempts != 2 || fax.ID != "ok" {
		t.Errorf("attempts=%d fax=%+v", attempts, fax)
	}
}

func TestListMePricing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/faxes":
			if r.URL.Query().Get("limit") != "5" {
				t.Errorf("limit = %q", r.URL.Query().Get("limit"))
			}
			io.WriteString(w, `{"data":[{"id":"a","status":"sent"}],"balance_cents":351}`)
		case "/v1/me":
			io.WriteString(w, `{"key_prefix":"ifx_live_ab","email":"a@b.c","balance_cents":351,"max_pages":25}`)
		case "/v1/pricing":
			io.WriteString(w, `{"price_cents":149,"currency":"usd","max_pages":25,"min_credit_cents":500,"countries":["US","CA"],"idempotency_header":"Idempotency-Key"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))

	list, err := c.ListFaxes(context.Background(), 5)
	if err != nil || len(list.Data) != 1 || list.BalanceCents != 351 {
		t.Fatalf("ListFaxes: %+v err=%v", list, err)
	}
	me, err := c.Me(context.Background())
	if err != nil || me.MaxPages != 25 || me.BalanceCents != 351 {
		t.Fatalf("Me: %+v err=%v", me, err)
	}
	pr, err := c.Pricing(context.Background())
	if err != nil || pr.PriceCents != 149 || len(pr.Countries) != 2 {
		t.Fatalf("Pricing: %+v err=%v", pr, err)
	}
}

func TestFilePathAndReader(t *testing.T) {
	missing := FilePath("/tmp/does-not-exist.pdf")
	data, err := missing.resolve()
	if err == nil || data != nil {
		t.Error("expected error reading missing file")
	}
	f := FileReader("r.pdf", strings.NewReader("streamed"))
	b, err := f.resolve()
	if err != nil || string(b) != "streamed" {
		t.Errorf("resolve = %q err=%v", b, err)
	}
	// Cached: a second resolve returns the same bytes.
	if b2, _ := f.resolve(); string(b2) != "streamed" {
		t.Errorf("cached resolve = %q", b2)
	}
}

func TestBackoffHonorsRetryAfter(t *testing.T) {
	e := &APIError{StatusCode: 429, Code: "rate_limited", RetryAfter: 3 * time.Second}
	if got := backoff(1, e); got != 3*time.Second {
		t.Errorf("backoff = %v, want 3s", got)
	}
	if got := backoff(1, nil); got != 500*time.Millisecond {
		t.Errorf("backoff = %v, want 500ms", got)
	}
}
