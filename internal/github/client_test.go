package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("")
	c.BaseURL = srv.URL
	return c, srv
}

func TestGetPRMerged(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number":           42,
			"title":            "Fix stuff",
			"user":             map[string]any{"login": "alice"},
			"state":            "closed",
			"merged":           true,
			"merge_commit_sha": "abc123",
		})
	})

	pr, err := c.GetPR(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("Number = %d, want 42", pr.Number)
	}
	if pr.Title != "Fix stuff" {
		t.Errorf("Title = %q, want %q", pr.Title, "Fix stuff")
	}
	if pr.Author != "alice" {
		t.Errorf("Author = %q, want %q", pr.Author, "alice")
	}
	if !pr.Merged {
		t.Error("expected Merged = true")
	}
	if pr.MergeCommit != "abc123" {
		t.Errorf("MergeCommit = %q, want %q", pr.MergeCommit, "abc123")
	}
}

func TestGetPROpen(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 99,
			"title":  "WIP",
			"user":   map[string]any{"login": "bob"},
			"state":  "open",
			"merged": false,
		})
	})

	pr, err := c.GetPR(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.State != "open" {
		t.Errorf("State = %q, want %q", pr.State, "open")
	}
	if pr.Merged {
		t.Error("expected Merged = false")
	}
}

func TestGetPRWithToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{
			"number": 1,
			"user":   map[string]any{"login": "x"},
			"state":  "open",
		})
	}))
	defer srv.Close()

	c := New("ghp_secret")
	c.BaseURL = srv.URL

	_, err := c.GetPR(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if gotAuth != "Bearer ghp_secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer ghp_secret")
	}
}

func TestGetPRWithoutToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{
			"number": 1,
			"user":   map[string]any{"login": "x"},
			"state":  "open",
		})
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL

	_, err := c.GetPR(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization should be empty, got %q", gotAuth)
	}
}

func TestGetPR404(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetPR(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestGetPRInvalidJSON(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	_, err := c.GetPR(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestIsCommitInBranchBehind(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"})
	})

	in, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err != nil {
		t.Fatalf("IsCommitInBranch: %v", err)
	}
	if !in {
		t.Error("expected true for 'behind' status")
	}
}

func TestIsCommitInBranchIdentical(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "identical"})
	})

	in, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err != nil {
		t.Fatalf("IsCommitInBranch: %v", err)
	}
	if !in {
		t.Error("expected true for 'identical' status")
	}
}

func TestIsCommitInBranchAhead(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"})
	})

	in, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err != nil {
		t.Fatalf("IsCommitInBranch: %v", err)
	}
	if in {
		t.Error("expected false for 'ahead' status")
	}
}

func TestIsCommitInBranchDiverged(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "diverged"})
	})

	in, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err != nil {
		t.Fatalf("IsCommitInBranch: %v", err)
	}
	if in {
		t.Error("expected false for 'diverged' status")
	}
}

func TestIsCommitInBranchHTTPError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestRateLimitHeader(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "50")
		json.NewEncoder(w).Encode(map[string]any{
			"number": 1,
			"user":   map[string]any{"login": "x"},
			"state":  "open",
		})
	})

	// Should not panic; the low rate limit just logs
	_, err := c.GetPR(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
}

func TestRateLimitedResponse(t *testing.T) {
	resetTime := time.Now().Add(30 * time.Minute).Unix()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime))
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := c.GetPR(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for rate-limited 403")
	}
	var rlErr *RateLimitError
	if !errors.As(err, &rlErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rlErr.RetryAfter.Unix() != resetTime {
		t.Errorf("RetryAfter = %v, want unix %d", rlErr.RetryAfter, resetTime)
	}
}

func TestRateLimited429(t *testing.T) {
	resetTime := time.Now().Add(10 * time.Minute).Unix()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime))
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.IsCommitInBranch(context.Background(), "abc123", "nixos-unstable")
	if err == nil {
		t.Fatal("expected error for rate-limited 429")
	}
	var rlErr *RateLimitError
	if !errors.As(err, &rlErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
}

func TestNonRateLimited403(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := c.GetPR(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var rlErr *RateLimitError
	if errors.As(err, &rlErr) {
		t.Fatal("expected regular error, not RateLimitError, for 403 without rate limit headers")
	}
}
