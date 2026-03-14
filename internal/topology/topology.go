package topology

import "time"

// NodeStatus represents the state of a branch in the pipeline.
type NodeStatus string

const (
	StatusLanded  NodeStatus = "landed"
	StatusPending NodeStatus = "pending"
	StatusUnknown NodeStatus = "unknown"
	StatusSkipped NodeStatus = "skipped"
)

// KnownBranches are the 6 nixpkgs branches in the unstable pipeline.
var KnownBranches = []string{
	"staging",
	"staging-next",
	"master",
	"nixos-unstable-small",
	"nixos-unstable",
	"nixpkgs-unstable",
}

// upstreamOf maps each branch to its direct upstream parent.
// This encodes the nixpkgs branch topology:
//
//	staging → staging-next → master → nixos-unstable-small → nixos-unstable
//	                         master → nixpkgs-unstable
var upstreamOf = map[string]string{
	"staging-next":         "staging",
	"master":               "staging-next",
	"nixos-unstable-small": "master",
	"nixos-unstable":       "nixos-unstable-small",
	"nixpkgs-unstable":     "master",
}

// Node represents a single branch in the pipeline.
type Node struct {
	Branch   string
	Status   NodeStatus
	LandedAt *time.Time
}

// Pipeline holds the topology result: known branches in NodeMap, extras in ExtraBranches.
type Pipeline struct {
	NodeMap       map[string]Node
	ExtraBranches []Node
}

// BuildPipeline creates a Pipeline from tracked branch data.
// The input map keys are branch names. A non-nil value means landed (with timestamp),
// a nil value means pending (tracked but not landed), and absent keys are unknown.
//
// If a downstream branch has landed but an upstream branch is unknown (not tracked),
// the upstream is marked as "skipped" — the PR bypassed it (e.g. merged directly
// to master without going through staging).
func BuildPipeline(trackedBranches map[string]*time.Time) Pipeline {
	known := make(map[string]bool, len(KnownBranches))
	for _, b := range KnownBranches {
		known[b] = true
	}

	p := Pipeline{
		NodeMap: make(map[string]Node, len(KnownBranches)),
	}

	// Populate all known branches with their status.
	for _, branch := range KnownBranches {
		landedAt, tracked := trackedBranches[branch]
		switch {
		case tracked && landedAt != nil:
			p.NodeMap[branch] = Node{Branch: branch, Status: StatusLanded, LandedAt: landedAt}
		case tracked:
			p.NodeMap[branch] = Node{Branch: branch, Status: StatusPending}
		default:
			p.NodeMap[branch] = Node{Branch: branch, Status: StatusUnknown}
		}
	}

	// Mark unknown upstream branches as skipped when a downstream has landed.
	for _, branch := range KnownBranches {
		if p.NodeMap[branch].Status != StatusLanded {
			continue
		}
		// Walk upstream and mark unknown ancestors as skipped.
		cur := branch
		for {
			parent, ok := upstreamOf[cur]
			if !ok {
				break
			}
			if p.NodeMap[parent].Status == StatusUnknown {
				p.NodeMap[parent] = Node{Branch: parent, Status: StatusSkipped}
			}
			cur = parent
		}
	}

	// Collect extra branches not in the known topology.
	for branch, landedAt := range trackedBranches {
		if known[branch] {
			continue
		}
		node := Node{Branch: branch, Status: StatusPending}
		if landedAt != nil {
			node.Status = StatusLanded
			node.LandedAt = landedAt
		}
		p.ExtraBranches = append(p.ExtraBranches, node)
	}

	return p
}

// IsUpstreamOf reports whether candidate is an upstream ancestor of branch
// in the known topology. For example, IsUpstreamOf("staging", "master") is true
// because staging → staging-next → master.
func IsUpstreamOf(candidate, branch string) bool {
	cur := branch
	for {
		parent, ok := upstreamOf[cur]
		if !ok {
			return false
		}
		if parent == candidate {
			return true
		}
		cur = parent
	}
}
