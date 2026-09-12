package routing

import (
	"testing"

	"MyApi/internal/channelmanager"
)

func ci(id int64, priority, weight int) channelmanager.ChannelInfo {
	return channelmanager.ChannelInfo{ID: id, Name: "c", Priority: priority, Weight: weight}
}

func ids(list []channelmanager.ChannelInfo) []int64 {
	out := make([]int64, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

func contains(list []channelmanager.ChannelInfo, id int64) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestPlanCandidatesGroupOrder(t *testing.T) {
	cands := []channelmanager.ChannelInfo{ci(1, 5, 100), ci(2, 10, 100), ci(3, 10, 100), ci(4, 5, 100)}
	ordered := planCandidates(cands, 0, 0, 0, true)
	if len(ordered) != 4 {
		t.Fatalf("expected 4, got %v", ids(ordered))
	}
	if ordered[0].Priority != 10 || ordered[1].Priority != 10 {
		t.Fatalf("high priority group should come first: %v", ids(ordered))
	}
	if ordered[2].Priority != 5 || ordered[3].Priority != 5 {
		t.Fatalf("lower priority should come later: %v", ids(ordered))
	}
}

func TestPlanCandidatesNoDegrade(t *testing.T) {
	cands := []channelmanager.ChannelInfo{ci(1, 10, 100), ci(2, 1, 100)}
	ordered := planCandidates(cands, 0, 0, 0, false)
	if len(ordered) != 1 || ordered[0].ID != 1 {
		t.Fatalf("try_next_priority=false should only keep top group, got %v", ids(ordered))
	}
}

func TestPlanCandidatesStickyFirstEvenLowerPriority(t *testing.T) {
	cands := []channelmanager.ChannelInfo{ci(1, 10, 100), ci(2, 1, 100)}
	ordered := planCandidates(cands, 2, 0, 0, true)
	if ordered[0].ID != 2 {
		t.Fatalf("sticky channel should be first, got %v", ids(ordered))
	}
	if !contains(ordered, 1) {
		t.Fatal("non-sticky candidate should remain")
	}
}

func TestPlanCandidatesStickyMissing(t *testing.T) {
	cands := []channelmanager.ChannelInfo{ci(1, 10, 100)}
	ordered := planCandidates(cands, 999, 0, 0, true)
	if len(ordered) != 1 || ordered[0].ID != 1 {
		t.Fatalf("missing sticky should be ignored, got %v", ids(ordered))
	}
}

func TestPlanCandidatesLimits(t *testing.T) {
	cands := []channelmanager.ChannelInfo{ci(1, 10, 100), ci(2, 10, 100), ci(3, 10, 100), ci(4, 1, 100)}
	ordered := planCandidates(cands, 0, 2, 0, true)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 (2 high + 1 low), got %v", ids(ordered))
	}
	if ordered[0].Priority != 10 || ordered[1].Priority != 10 || ordered[2].Priority != 1 {
		t.Fatalf("unexpected order: %v", ids(ordered))
	}
	ordered2 := planCandidates(cands, 0, 0, 2, true)
	if len(ordered2) != 2 {
		t.Fatalf("expected total cap 2, got %v", ids(ordered2))
	}
}

func TestWeightedShuffleIsPermutation(t *testing.T) {
	pool := []channelmanager.ChannelInfo{ci(1, 0, 1), ci(2, 0, 2), ci(3, 0, 3)}
	seen := map[int64]bool{}
	for i := 0; i < 50; i++ {
		out := weightedShuffle(pool)
		if len(out) != 3 {
			t.Fatalf("len=%d", len(out))
		}
		for _, c := range out {
			seen[c.ID] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("shuffle should include all channels, seen=%v", seen)
	}
}
