package server

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/db"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/github"
)

type Server struct {
	db       *db.DB
	gh       *github.Client
	bus      *event.Bus
	branches []string
	tmpl     *template.Template
}

func New(database *db.DB, gh *github.Client, bus *event.Bus, branches []string, tmpl *template.Template) *Server {
	return &Server{
		db:       database,
		gh:       gh,
		bus:      bus,
		branches: branches,
		tmpl:     tmpl,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /api/prs", s.handleAddPR)
	mux.HandleFunc("GET /api/prs", s.handleListPRs)
	mux.HandleFunc("DELETE /api/prs/{number}", s.handleDeletePR)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	prs, err := s.db.ListPRs()
	if err != nil {
		log.Printf("server: listing PRs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "index.html", prs); err != nil {
		log.Printf("server: rendering template: %v", err)
	}
}

func (s *Server) handleAddPR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PRNumber int `json:"pr_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.PRNumber <= 0 {
		http.Error(w, `{"error":"pr_number must be positive"}`, http.StatusBadRequest)
		return
	}

	// Verify PR exists on GitHub
	info, err := s.gh.GetPR(r.Context(), req.PRNumber)
	if err != nil {
		log.Printf("server: fetching PR #%d: %v", req.PRNumber, err)
		http.Error(w, `{"error":"could not fetch PR from GitHub"}`, http.StatusBadGateway)
		return
	}

	if err := s.db.AddPR(req.PRNumber); err != nil {
		log.Printf("server: adding PR #%d: %v", req.PRNumber, err)
		http.Error(w, `{"error":"could not add PR"}`, http.StatusInternalServerError)
		return
	}

	// Set initial status from GitHub
	status := "open"
	mergeCommit := ""
	if info.Merged {
		status = "merged"
		mergeCommit = info.MergeCommit
	} else if info.State == "closed" {
		status = "closed"
	}
	if err := s.db.UpdatePRStatus(req.PRNumber, status, mergeCommit, info.Title, info.Author); err != nil {
		log.Printf("server: updating PR #%d status: %v", req.PRNumber, err)
	}

	s.bus.Publish(event.Event{
		Type:      event.PRAdded,
		PRNumber:  req.PRNumber,
		Title:     info.Title,
		Author:    info.Author,
		Timestamp: time.Now(),
	})

	// Emit notifications for gates already passed
	allLanded := false
	if info.Merged {
		s.bus.Publish(event.Event{
			Type:      event.PRMerged,
			PRNumber:  req.PRNumber,
			Title:     info.Title,
			Author:    info.Author,
			Timestamp: time.Now(),
		})

		// Check each branch and emit + record if already landed
		landedCount := 0
		for _, branch := range s.branches {
			inBranch, err := s.gh.IsCommitInBranch(r.Context(), info.MergeCommit, branch)
			if err != nil {
				log.Printf("server: checking PR #%d in %s: %v", req.PRNumber, branch, err)
				continue
			}
			if inBranch {
				if err := s.db.UpdateBranchLanded(req.PRNumber, branch); err != nil {
					log.Printf("server: updating branch status for PR #%d: %v", req.PRNumber, err)
				}
				s.bus.Publish(event.Event{
					Type:      event.PRLandedBranch,
					PRNumber:  req.PRNumber,
					Title:     info.Title,
					Author:    info.Author,
					Branch:    branch,
					Timestamp: time.Now(),
				})
				landedCount++
			}
		}
		allLanded = landedCount == len(s.branches)
	}

	// Auto-remove if already landed in all branches
	if allLanded {
		log.Printf("PR #%d has already landed in all branches, removing", req.PRNumber)
		if err := s.db.RemovePR(req.PRNumber); err != nil {
			log.Printf("server: removing PR #%d: %v", req.PRNumber, err)
		}
		s.bus.Publish(event.Event{
			Type:      event.PRRemoved,
			PRNumber:  req.PRNumber,
			Title:     info.Title,
			Author:    info.Author,
			Timestamp: time.Now(),
		})
	}

	pr, err := s.db.GetPR(req.PRNumber)
	if err != nil {
		log.Printf("server: fetching added PR #%d: %v", req.PRNumber, err)
		http.Error(w, `{"error":"PR added but could not fetch"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pr)
}

func (s *Server) handleListPRs(w http.ResponseWriter, r *http.Request) {
	prs, err := s.db.ListPRs()
	if err != nil {
		log.Printf("server: listing PRs: %v", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prs)
}

func (s *Server) handleDeletePR(w http.ResponseWriter, r *http.Request) {
	numStr := r.PathValue("number")
	num, err := strconv.Atoi(numStr)
	if err != nil {
		http.Error(w, `{"error":"invalid PR number"}`, http.StatusBadRequest)
		return
	}

	pr, err := s.db.GetPR(num)
	if err != nil {
		log.Printf("server: fetching PR #%d for removal: %v", num, err)
	}

	if err := s.db.RemovePR(num); err != nil {
		log.Printf("server: removing PR #%d: %v", num, err)
		http.Error(w, `{"error":"could not remove PR"}`, http.StatusInternalServerError)
		return
	}

	evt := event.Event{
		Type:      event.PRRemoved,
		PRNumber:  num,
		Timestamp: time.Now(),
	}
	if pr != nil {
		evt.Title = pr.Title
		evt.Author = pr.Author
	}
	s.bus.Publish(evt)

	w.WriteHeader(http.StatusNoContent)
}
