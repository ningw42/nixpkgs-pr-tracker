package poller

import (
	"context"
	"errors"
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
		p.runPollCycle(ctx)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.runPollCycle(ctx)
			}
		}
	}()
}

// runPollCycle runs a poll and, if rate-limited, waits until the reset time
// before returning so the next ticker tick doesn't fire too early.
func (p *Poller) runPollCycle(ctx context.Context) {
	rlErr := p.poll(ctx)
	if rlErr == nil {
		return
	}
	wait := time.Until(rlErr.RetryAfter)
	if wait <= 0 {
		return
	}
	log.Printf("poller: waiting %s until rate limit resets", wait.Round(time.Second))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (p *Poller) poll(ctx context.Context) *github.RateLimitError {
	prs, err := p.db.ListPRs()
	if err != nil {
		log.Printf("poller: listing PRs: %v", err)
		return nil
	}

	if len(prs) == 0 {
		log.Printf("poller: no PRs to check")
		return nil
	}

	prNumbers := make([]int, len(prs))
	for i, pr := range prs {
		prNumbers[i] = pr.PRNumber
	}
	log.Printf("poller: checking %d PRs: %v", len(prs), prNumbers)

	for _, pr := range prs {
		if ctx.Err() != nil {
			return nil
		}
		if err := p.pollPR(ctx, pr); err != nil {
			var rlErr *github.RateLimitError
			if errors.As(err, &rlErr) {
				log.Printf("poller: rate limited, resets at %s, skipping remaining PRs", rlErr.RetryAfter.Format("15:04:05"))
				return rlErr
			}
		}
	}
	return nil
}

func (p *Poller) pollPR(ctx context.Context, pr db.TrackedPR) error {
	if pr.Status == "open" {
		info, err := p.gh.GetPR(ctx, pr.PRNumber)
		if err != nil {
			log.Printf("poller: fetching PR #%d: %v", pr.PRNumber, err)
			return err
		}

		if info.Merged {
			if err := p.db.UpdatePRStatus(pr.PRNumber, "merged", info.MergeCommit, info.Title, info.Author); err != nil {
				log.Printf("poller: updating PR #%d status: %v", pr.PRNumber, err)
				return nil
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
			return nil
		} else {
			// Still open, update title/author
			if err := p.db.UpdatePRStatus(pr.PRNumber, "open", "", info.Title, info.Author); err != nil {
				log.Printf("poller: updating PR #%d info: %v", pr.PRNumber, err)
			}
			return nil
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
				return err
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
	return nil
}
