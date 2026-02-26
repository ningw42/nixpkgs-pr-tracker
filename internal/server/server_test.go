package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/db"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/github"
)

const testTemplate = `{{define "index.html"}}<!DOCTYPE html><html><body>{{if .}}{{range .}}#{{.PRNumber}}{{end}}{{else}}empty{{end}}</body></html>{{end}}`

type testEnv struct {
	db     *db.DB
	gh     *github.Client
	ghMux  *http.ServeMux
	bus    *event.Bus
	srv    *Server
	router http.Handler
}

func setupTest(t *testing.T, branches []string) *testEnv {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	database, err := db.New(dsn)
	if err != nil {
		t.Fatalf("opening DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	ghMux := http.NewServeMux()
	ghServer := httptest.NewServer(ghMux)
	t.Cleanup(ghServer.Close)

	ghClient := github.New("")
	ghClient.BaseURL = ghServer.URL

	bus := event.New()

	tmpl := template.Must(template.New("").Parse(testTemplate))

	s := New(database, ghClient, bus, branches, tmpl)

	return &testEnv{
		db:     database,
		gh:     ghClient,
		ghMux:  ghMux,
		bus:    bus,
		srv:    s,
		router: s.Routes(),
	}
}

func TestListPRsEmpty(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("GET", "/api/prs", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestAddPRSuccess(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 42, "title": "Test PR", "user": map[string]any{"login": "alice"},
			"state": "open", "merged": false,
		})
	})

	body := strings.NewReader(`{"pr_number": 42}`)
	req := httptest.NewRequest("POST", "/api/prs", body)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	pr, err := env.db.GetPR(42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Title != "Test PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Test PR")
	}
}

func TestAddMergedPR(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/50", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 50, "title": "Merged PR", "user": map[string]any{"login": "bob"},
			"state": "closed", "merged": true, "merge_commit_sha": "sha123",
		})
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...sha123", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"}) // not yet landed
	})

	body := strings.NewReader(`{"pr_number": 50}`)
	req := httptest.NewRequest("POST", "/api/prs", body)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	pr, _ := env.db.GetPR(50)
	if pr.Status != "merged" {
		t.Errorf("Status = %q, want %q", pr.Status, "merged")
	}
}

func TestAddPRInvalidJSON(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAddPRZeroNumber(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 0}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAddPRNegativeNumber(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": -5}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAddPRGitHubError(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 999}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestAddPREventEmission(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/10", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 10, "title": "Event Test", "user": map[string]any{"login": "carol"},
			"state": "open", "merged": false,
		})
	})

	var mu sync.Mutex
	var events []event.Event
	env.bus.Subscribe(func(e event.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 10}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Type != event.PRAdded {
		t.Errorf("event type = %q, want %q", events[0].Type, event.PRAdded)
	}
}

func TestAddMergedPREventEmission(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/11", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 11, "title": "Merged Event", "user": map[string]any{"login": "dave"},
			"state": "closed", "merged": true, "merge_commit_sha": "sha456",
		})
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...sha456", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"})
	})

	var mu sync.Mutex
	var events []event.Event
	env.bus.Subscribe(func(e event.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 11}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	mu.Lock()
	defer mu.Unlock()
	// PRAdded + PRMerged
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	types := make(map[event.Type]bool)
	for _, e := range events {
		types[e.Type] = true
	}
	if !types[event.PRAdded] {
		t.Error("missing PRAdded event")
	}
	if !types[event.PRMerged] {
		t.Error("missing PRMerged event")
	}
}

func TestAddPRLandedBranchEvent(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/12", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 12, "title": "Landed", "user": map[string]any{"login": "eve"},
			"state": "closed", "merged": true, "merge_commit_sha": "sha789",
		})
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...sha789", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"}) // landed
	})

	var mu sync.Mutex
	var events []event.Event
	env.bus.Subscribe(func(e event.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 12}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	mu.Lock()
	defer mu.Unlock()
	types := make(map[event.Type]bool)
	for _, e := range events {
		types[e.Type] = true
	}
	if !types[event.PRLandedBranch] {
		t.Error("missing PRLandedBranch event")
	}
	if !types[event.PRRemoved] {
		t.Error("missing PRRemoved event (auto-remove)")
	}
}

func TestAutoRemoveAllLanded(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/13", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 13, "title": "All Landed", "user": map[string]any{"login": "frank"},
			"state": "closed", "merged": true, "merge_commit_sha": "shaAll",
		})
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...shaAll", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"})
	})

	req := httptest.NewRequest("POST", "/api/prs", strings.NewReader(`{"pr_number": 13}`))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	// PR should be auto-removed
	_, err := env.db.GetPR(13)
	if err == nil {
		t.Error("expected PR to be auto-removed, but it still exists")
	}
}

func TestDeletePR(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.db.AddPR(77)
	env.db.UpdatePRStatus(77, "open", "", "Delete Me", "user")

	req := httptest.NewRequest("DELETE", "/api/prs/77", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}

	_, err := env.db.GetPR(77)
	if err == nil {
		t.Error("expected PR to be deleted")
	}
}

func TestDeletePRInvalidNumber(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("DELETE", "/api/prs/abc", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeletePREvent(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	env.db.AddPR(88)
	env.db.UpdatePRStatus(88, "open", "", "To Remove", "tester")

	var received event.Event
	env.bus.Subscribe(func(e event.Event) {
		received = e
	})

	req := httptest.NewRequest("DELETE", "/api/prs/88", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if received.Type != event.PRRemoved {
		t.Errorf("event type = %q, want %q", received.Type, event.PRRemoved)
	}
	if received.PRNumber != 88 {
		t.Errorf("event PRNumber = %d, want 88", received.PRNumber)
	}
}

func TestIndexPage(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty") {
		t.Error("expected 'empty' in response for no PRs")
	}
}

func TestNotFoundPage(t *testing.T) {
	env := setupTest(t, []string{"nixos-unstable"})

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
