package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// RateLimitError is returned when GitHub responds with a rate limit (403 or 429)
// and the X-RateLimit-Remaining header is 0.
type RateLimitError struct {
	RetryAfter time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("GitHub API rate limited, resets at %s", e.RetryAfter.Format(time.RFC3339))
}

type PRInfo struct {
	Number      int
	Title       string
	Author      string
	State       string // "open", "closed"
	Merged      bool
	MergeCommit string
}

type Client struct {
	httpClient *http.Client
	token      string
	BaseURL    string
}

func New(token string) *Client {
	return &Client{
		httpClient: &http.Client{},
		token:      token,
		BaseURL:    "https://api.github.com",
	}
}

func (c *Client) doRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil && n < 100 {
			log.Printf("GitHub API rate limit low: %d remaining", n)
		}
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
			resp.Body.Close()
			var resetTime time.Time
			if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
				if epoch, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					resetTime = time.Unix(epoch, 0)
				}
			}
			return nil, &RateLimitError{RetryAfter: resetTime}
		}
	}
	return resp, nil
}

func (c *Client) GetPR(ctx context.Context, prNumber int) (*PRInfo, error) {
	url := fmt.Sprintf("%s/repos/NixOS/nixpkgs/pulls/%d", c.BaseURL, prNumber)
	resp, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching PR %d: %w", prNumber, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d for PR %d", resp.StatusCode, prNumber)
	}

	var data struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		State          string `json:"state"`
		Merged         bool   `json:"merged"`
		MergeCommitSHA string `json:"merge_commit_sha"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding PR %d response: %w", prNumber, err)
	}

	return &PRInfo{
		Number:      data.Number,
		Title:       data.Title,
		Author:      data.User.Login,
		State:       data.State,
		Merged:      data.Merged,
		MergeCommit: data.MergeCommitSHA,
	}, nil
}

func (c *Client) IsCommitInBranch(ctx context.Context, sha string, branch string) (bool, error) {
	url := fmt.Sprintf("%s/repos/NixOS/nixpkgs/compare/%s...%s", c.BaseURL, branch, sha)
	resp, err := c.doRequest(ctx, url)
	if err != nil {
		return false, fmt.Errorf("comparing %s to %s: %w", sha, branch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GitHub API returned %d for compare", resp.StatusCode)
	}

	var data struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, fmt.Errorf("decoding compare response: %w", err)
	}

	// "behind" means sha is behind branch (i.e., branch contains sha)
	// "identical" means they point to the same commit
	return data.Status == "behind" || data.Status == "identical", nil
}
