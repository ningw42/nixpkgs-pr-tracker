package poller

import (
	"context"
	"log"
	"time"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/db"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/github"
)

type Poller struct {
	db       *db.DB
	gh       *github.Client
	bus      *event.Bus
	interval time.Duration
	branches []string
}

func New(database *db.DB, gh *github.Client, bus *event.Bus, interval time.Duration, branches []string) *Poller {
	return &Poller{
		db:       database,
		gh:       gh,
		bus:      bus,
		interval: interval,
		branches: branches,
	}
}

func (p *Poller) Start(ctx context.Context) {
	go func() {
		p.poll(ctx)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll(ctx)
			}
		}
	}()
}

func (p *Poller) poll(ctx context.Context) {
	prs, err := p.db.ListPRs()
	if err != nil {
		log.Printf("poller: listing PRs: %v", err)
		return
	}

	for _, pr := range prs {
		if ctx.Err() != nil {
			return
		}
		p.pollPR(ctx, pr)
	}
}

func (p *Poller) pollPR(ctx context.Context, pr db.TrackedPR) {
	if pr.Status == "open" {
		info, err := p.gh.GetPR(ctx, pr.PRNumber)
		if err != nil {
			log.Printf("poller: fetching PR #%d: %v", pr.PRNumber, err)
			return
		}

		if info.Merged {
			if err := p.db.UpdatePRStatus(pr.PRNumber, "merged", info.MergeCommit, info.Title, info.Author); err != nil {
				log.Printf("poller: updating PR #%d status: %v", pr.PRNumber, err)
				return
			}
			p.bus.Publish(event.Event{
				Type:      event.PRMerged,
				PRNumber:  pr.PRNumber,
				Title:     info.Title,
				Author:    info.Author,
				Timestamp: time.Now(),
			})
			pr.Status = "merged"
			pr.MergeCommit = info.MergeCommit
			pr.Title = info.Title
			pr.Author = info.Author
		} else if info.State == "closed" {
			if err := p.db.UpdatePRStatus(pr.PRNumber, "closed", "", info.Title, info.Author); err != nil {
				log.Printf("poller: updating PR #%d status: %v", pr.PRNumber, err)
			}
			return
		} else {
			// Still open, update title/author
			if err := p.db.UpdatePRStatus(pr.PRNumber, "open", "", info.Title, info.Author); err != nil {
				log.Printf("poller: updating PR #%d info: %v", pr.PRNumber, err)
			}
			return
		}
	}

	if pr.Status == "merged" && pr.MergeCommit != "" {
		landedBranches := make(map[string]bool)
		for _, bs := range pr.Branches {
			if bs.Landed {
				landedBranches[bs.Branch] = true
			}
		}

		for _, branch := range p.branches {
			if landedBranches[branch] {
				continue
			}

			inBranch, err := p.gh.IsCommitInBranch(ctx, pr.MergeCommit, branch)
			if err != nil {
				log.Printf("poller: checking PR #%d in %s: %v", pr.PRNumber, branch, err)
				continue
			}

			if inBranch {
				if err := p.db.UpdateBranchLanded(pr.PRNumber, branch); err != nil {
					log.Printf("poller: updating branch status for PR #%d: %v", pr.PRNumber, err)
					continue
				}
				p.bus.Publish(event.Event{
					Type:      event.PRLandedBranch,
					PRNumber:  pr.PRNumber,
					Title:     pr.Title,
					Author:    pr.Author,
					Branch:    branch,
					Timestamp: time.Now(),
				})
				landedBranches[branch] = true
			}
		}

		// Remove PR once it has landed in all tracked branches
		allLanded := true
		for _, branch := range p.branches {
			if !landedBranches[branch] {
				allLanded = false
				break
			}
		}
		if allLanded {
			log.Printf("PR #%d has landed in all branches, removing", pr.PRNumber)
			if err := p.db.RemovePR(pr.PRNumber); err != nil {
				log.Printf("poller: removing PR #%d: %v", pr.PRNumber, err)
			}
			p.bus.Publish(event.Event{
				Type:      event.PRRemoved,
				PRNumber:  pr.PRNumber,
				Title:     pr.Title,
				Author:    pr.Author,
				Timestamp: time.Now(),
			})
		}
	}
}
