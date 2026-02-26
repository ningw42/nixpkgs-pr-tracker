package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
)

func TestWebhookName(t *testing.T) {
	w := NewWebhook("http://example.com")
	if w.Name() != "webhook" {
		t.Errorf("Name() = %q, want %q", w.Name(), "webhook")
	}
}

func TestWebhookNotifySuccess(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL)
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	err := w.Notify(context.Background(), event.Event{
		Type:      event.PRMerged,
		PRNumber:  42,
		Title:     "test",
		Author:    "user1",
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if receivedBody["event"] != "pr_merged" {
		t.Errorf("event = %v, want pr_merged", receivedBody["event"])
	}
	if int(receivedBody["pr_number"].(float64)) != 42 {
		t.Errorf("pr_number = %v, want 42", receivedBody["pr_number"])
	}
	if receivedBody["title"] != "test" {
		t.Errorf("title = %v, want test", receivedBody["title"])
	}
	if receivedBody["author"] != "user1" {
		t.Errorf("author = %v, want user1", receivedBody["author"])
	}
}

func TestWebhookNotifyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL)
	err := w.Notify(context.Background(), event.Event{Type: event.PRAdded, PRNumber: 1})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestWebhookNotifyConnectionRefused(t *testing.T) {
	w := NewWebhook("http://127.0.0.1:1") // port 1 — nothing listening
	err := w.Notify(context.Background(), event.Event{Type: event.PRAdded, PRNumber: 1})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestWebhookNotifyCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := w.Notify(ctx, event.Event{Type: event.PRAdded, PRNumber: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
