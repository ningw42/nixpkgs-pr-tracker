package topology

import "time"

// NodeStatus represents the state of a branch in the pipeline.
type NodeStatus string

const (
	StatusLanded  NodeStatus = "landed"
	StatusPending NodeStatus = "pending"
	StatusUnknown NodeStatus = "unknown"
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
