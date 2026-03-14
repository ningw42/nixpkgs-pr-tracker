package topology

import (
	"testing"
	"time"
)

func TestEmptyInput(t *testing.T) {
	p := BuildPipeline(nil)

	for _, branch := range KnownBranches {
		node, ok := p.NodeMap[branch]
		if !ok {
			t.Errorf("missing node for %s", branch)
			continue
		}
		if node.Status != StatusUnknown {
			t.Errorf("%s: status = %q, want %q", branch, node.Status, StatusUnknown)
		}
	}
	if len(p.ExtraBranches) != 0 {
		t.Errorf("ExtraBranches = %d, want 0", len(p.ExtraBranches))
	}
}

func TestPartialTracking(t *testing.T) {
	p := BuildPipeline(map[string]*time.Time{
		"nixos-unstable": nil,
	})

	if node := p.NodeMap["nixos-unstable"]; node.Status != StatusPending {
		t.Errorf("nixos-unstable: status = %q, want %q", node.Status, StatusPending)
	}
	if node := p.NodeMap["master"]; node.Status != StatusUnknown {
		t.Errorf("master: status = %q, want %q", node.Status, StatusUnknown)
	}
}

func TestPartialLanding(t *testing.T) {
	now := time.Now()
	p := BuildPipeline(map[string]*time.Time{
		"master":         &now,
		"nixos-unstable": nil,
	})

	masterNode := p.NodeMap["master"]
	if masterNode.Status != StatusLanded {
		t.Errorf("master: status = %q, want %q", masterNode.Status, StatusLanded)
	}
	if masterNode.LandedAt == nil || !masterNode.LandedAt.Equal(now) {
		t.Errorf("master: LandedAt = %v, want %v", masterNode.LandedAt, now)
	}

	if node := p.NodeMap["nixos-unstable"]; node.Status != StatusPending {
		t.Errorf("nixos-unstable: status = %q, want %q", node.Status, StatusPending)
	}

	// staging and staging-next should be skipped since master landed but they're unknown
	if node := p.NodeMap["staging"]; node.Status != StatusSkipped {
		t.Errorf("staging: status = %q, want %q", node.Status, StatusSkipped)
	}
	if node := p.NodeMap["staging-next"]; node.Status != StatusSkipped {
		t.Errorf("staging-next: status = %q, want %q", node.Status, StatusSkipped)
	}
}

func TestSkippedUpstream(t *testing.T) {
	now := time.Now()

	// nixos-unstable landed, but only nixos-unstable and nixos-unstable-small are tracked
	p := BuildPipeline(map[string]*time.Time{
		"nixos-unstable":       &now,
		"nixos-unstable-small": &now,
	})

	// master, staging-next, staging are unknown but downstream landed → skipped
	if node := p.NodeMap["master"]; node.Status != StatusSkipped {
		t.Errorf("master: status = %q, want %q", node.Status, StatusSkipped)
	}
	if node := p.NodeMap["staging-next"]; node.Status != StatusSkipped {
		t.Errorf("staging-next: status = %q, want %q", node.Status, StatusSkipped)
	}
	if node := p.NodeMap["staging"]; node.Status != StatusSkipped {
		t.Errorf("staging: status = %q, want %q", node.Status, StatusSkipped)
	}

	// nixpkgs-unstable has no data and isn't downstream of nixos-unstable → stays unknown
	if node := p.NodeMap["nixpkgs-unstable"]; node.Status != StatusUnknown {
		t.Errorf("nixpkgs-unstable: status = %q, want %q", node.Status, StatusUnknown)
	}
}

func TestSkippedDoesNotOverrideLanded(t *testing.T) {
	now := time.Now()

	// Both staging and master landed
	p := BuildPipeline(map[string]*time.Time{
		"staging":      &now,
		"staging-next": nil, // tracked but not landed
		"master":       &now,
	})

	// staging is landed, should NOT be overridden to skipped
	if node := p.NodeMap["staging"]; node.Status != StatusLanded {
		t.Errorf("staging: status = %q, want %q", node.Status, StatusLanded)
	}
	// staging-next is tracked/pending, should NOT be overridden to skipped
	if node := p.NodeMap["staging-next"]; node.Status != StatusPending {
		t.Errorf("staging-next: status = %q, want %q", node.Status, StatusPending)
	}
}

func TestAllLanded(t *testing.T) {
	now := time.Now()
	input := make(map[string]*time.Time, len(KnownBranches))
	for _, b := range KnownBranches {
		ts := now
		input[b] = &ts
	}

	p := BuildPipeline(input)

	for _, branch := range KnownBranches {
		if node := p.NodeMap[branch]; node.Status != StatusLanded {
			t.Errorf("%s: status = %q, want %q", branch, node.Status, StatusLanded)
		}
	}
}

func TestExtraBranches(t *testing.T) {
	now := time.Now()
	p := BuildPipeline(map[string]*time.Time{
		"nixos-unstable": nil,
		"nixos-24.11":    &now,
	})

	if len(p.ExtraBranches) != 1 {
		t.Fatalf("ExtraBranches = %d, want 1", len(p.ExtraBranches))
	}
	extra := p.ExtraBranches[0]
	if extra.Branch != "nixos-24.11" {
		t.Errorf("extra branch = %q, want %q", extra.Branch, "nixos-24.11")
	}
	if extra.Status != StatusLanded {
		t.Errorf("extra status = %q, want %q", extra.Status, StatusLanded)
	}
}

func TestAllKnownBranchesAlwaysPresent(t *testing.T) {
	p := BuildPipeline(map[string]*time.Time{
		"nixos-unstable": nil,
	})

	if len(p.NodeMap) != len(KnownBranches) {
		t.Errorf("NodeMap has %d entries, want %d", len(p.NodeMap), len(KnownBranches))
	}
	for _, branch := range KnownBranches {
		if _, ok := p.NodeMap[branch]; !ok {
			t.Errorf("missing node for known branch %q", branch)
		}
	}
}
