package poller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/db"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/github"
)

type pollerEnv struct {
	db    *db.DB
	gh    *github.Client
	ghMux *http.ServeMux
	bus   *event.Bus
	p     *Poller
}

func setupPoller(t *testing.T, branches []string) *pollerEnv {
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

	p := New(database, ghClient, bus, time.Hour, branches)

	return &pollerEnv{
		db:    database,
		gh:    ghClient,
		ghMux: ghMux,
		bus:   bus,
		p:     p,
	}
}

func TestPollNoPRs(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})
	// Should not panic with no PRs
	env.p.poll(context.Background())
}

func TestPollOpenStaysOpen(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(1)

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 1, "title": "Still Open", "user": map[string]any{"login": "alice"},
			"state": "open", "merged": false,
		})
	})

	env.p.poll(context.Background())

	pr, _ := env.db.GetPR(1)
	if pr.Status != "open" {
		t.Errorf("Status = %q, want %q", pr.Status, "open")
	}
	if pr.Title != "Still Open" {
		t.Errorf("Title = %q, want %q", pr.Title, "Still Open")
	}
}

func TestPollOpenToMerged(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(2)

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/2", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 2, "title": "Now Merged", "user": map[string]any{"login": "bob"},
			"state": "closed", "merged": true, "merge_commit_sha": "mergesha",
		})
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...mergesha", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"}) // not yet landed
	})

	var mu sync.Mutex
	var events []event.Event
	env.bus.Subscribe(func(e event.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	env.p.poll(context.Background())

	pr, _ := env.db.GetPR(2)
	if pr.Status != "merged" {
		t.Errorf("Status = %q, want %q", pr.Status, "merged")
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range events {
		if e.Type == event.PRMerged && e.PRNumber == 2 {
			found = true
		}
	}
	if !found {
		t.Error("missing PRMerged event")
	}
}

func TestPollOpenToClosed(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(3)

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/3", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 3, "title": "Closed", "user": map[string]any{"login": "carol"},
			"state": "closed", "merged": false,
		})
	})

	env.p.poll(context.Background())

	pr, _ := env.db.GetPR(3)
	if pr.Status != "closed" {
		t.Errorf("Status = %q, want %q", pr.Status, "closed")
	}
}

func TestPollMergedChecksBranches(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(4)
	env.db.UpdatePRStatus(4, "merged", "commitABC", "Merged PR", "dave")

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...commitABC", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"}) // landed
	})

	var mu sync.Mutex
	var events []event.Event
	env.bus.Subscribe(func(e event.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	env.p.poll(context.Background())

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

	// PR should be removed
	_, err := env.db.GetPR(4)
	if err == nil {
		t.Error("expected PR to be auto-removed")
	}
}

func TestPollNotYetLanded(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(5)
	env.db.UpdatePRStatus(5, "merged", "commitDEF", "Not Landed", "eve")

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...commitDEF", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"})
	})

	env.p.poll(context.Background())

	// PR should still exist
	pr, err := env.db.GetPR(5)
	if err != nil {
		t.Fatalf("expected PR to still exist: %v", err)
	}
	if pr.Status != "merged" {
		t.Errorf("Status = %q, want %q", pr.Status, "merged")
	}
}

func TestPollSkipAlreadyLanded(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable", "nixos-24.11"})

	env.db.AddPR(6)
	env.db.UpdatePRStatus(6, "merged", "commitGHI", "Partial Landed", "frank")
	env.db.UpdateBranchLanded(6, "nixos-unstable") // already landed

	var compareCount int
	var compareMu sync.Mutex
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/", func(w http.ResponseWriter, r *http.Request) {
		compareMu.Lock()
		compareCount++
		compareMu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"}) // nixos-24.11 not landed
	})

	env.p.poll(context.Background())

	compareMu.Lock()
	defer compareMu.Unlock()
	// Should only check nixos-24.11 (skip nixos-unstable)
	if compareCount != 1 {
		t.Errorf("compare API calls = %d, want 1 (skipping already landed)", compareCount)
	}
}

func TestPollAllLandedAutoRemoves(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable", "nixos-24.11"})

	env.db.AddPR(7)
	env.db.UpdatePRStatus(7, "merged", "commitJKL", "All Landing", "grace")

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"}) // both landed
	})

	env.p.poll(context.Background())

	_, err := env.db.GetPR(7)
	if err == nil {
		t.Error("expected PR to be auto-removed after landing in all branches")
	}
}

func TestPollPartialLanding(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable", "nixos-24.11"})

	env.db.AddPR(8)
	env.db.UpdatePRStatus(8, "merged", "commitMNO", "Partial", "heidi")

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-unstable...commitMNO", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind"}) // landed
	})
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/compare/nixos-24.11...commitMNO", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ahead"}) // not landed
	})

	env.p.poll(context.Background())

	pr, err := env.db.GetPR(8)
	if err != nil {
		t.Fatalf("expected PR to still exist: %v", err)
	}

	statuses, _ := env.db.GetBranchStatus(8)
	landed := make(map[string]bool)
	for _, s := range statuses {
		landed[s.Branch] = s.Landed
	}
	if !landed["nixos-unstable"] {
		t.Error("nixos-unstable should be landed")
	}
	if landed["nixos-24.11"] {
		t.Error("nixos-24.11 should not be landed")
	}
	_ = pr
}

func TestPollGitHubErrorGraceful(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(9)

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/9", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Should not panic
	env.p.poll(context.Background())

	// PR should still exist with original status
	pr, err := env.db.GetPR(9)
	if err != nil {
		t.Fatalf("expected PR to still exist: %v", err)
	}
	if pr.Status != "open" {
		t.Errorf("Status = %q, want %q (unchanged)", pr.Status, "open")
	}
}

func TestPollContextCancellation(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(10)
	env.db.AddPR(11)

	callCount := 0
	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(map[string]any{
			"number": 10, "title": "X", "user": map[string]any{"login": "x"},
			"state": "open", "merged": false,
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before poll

	env.p.poll(ctx)
	// With cancelled context, should return early
}

func TestStartPollsImmediately(t *testing.T) {
	env := setupPoller(t, []string{"nixos-unstable"})

	env.db.AddPR(20)

	env.ghMux.HandleFunc("/repos/NixOS/nixpkgs/pulls/20", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"number": 20, "title": "Immediate", "user": map[string]any{"login": "ivan"},
			"state": "open", "merged": false,
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	env.p.Start(ctx)

	// Give the goroutine time to complete the immediate poll
	time.Sleep(100 * time.Millisecond)
	cancel()

	pr, _ := env.db.GetPR(20)
	if pr.Title != "Immediate" {
		t.Errorf("Title = %q, want %q (Start should poll immediately)", pr.Title, "Immediate")
	}
}
