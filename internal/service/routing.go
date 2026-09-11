package service

import (
	"math/rand"

	"MyApi/internal/channelmanager"
)

// planCandidates 生成候选尝试顺序：
//  1. 粘性渠道（若可用）置顶；
//  2. 其余按 priority 降序分组，组内按 weight 无放回洗牌；
//  3. 逐组展开，整组失败才降级；受 maxPerPriority / maxTotal 限制（0=不限）。
func planCandidates(cands []channelmanager.ChannelInfo, stickyID int64, maxPerPriority, maxTotal int, tryNextPriority bool) []channelmanager.ChannelInfo {
	byID := make(map[int64]channelmanager.ChannelInfo, len(cands))
	for _, c := range cands {
		byID[c.ID] = c
	}

	var ordered []channelmanager.ChannelInfo
	used := map[int64]bool{}
	if stickyID != 0 {
		if c, ok := byID[stickyID]; ok {
			ordered = append(ordered, c)
			used[stickyID] = true
		}
	}

	// 分组：priority 降序
	groups := map[int][]channelmanager.ChannelInfo{}
	var priorities []int
	for _, c := range cands {
		if used[c.ID] {
			continue
		}
		if _, ok := groups[c.Priority]; !ok {
			priorities = append(priorities, c.Priority)
		}
		groups[c.Priority] = append(groups[c.Priority], c)
	}
	sortIntDesc(priorities)

	for gi, p := range priorities {
		if gi > 0 && !tryNextPriority {
			break
		}
		pool := weightedShuffle(groups[p])
		limit := len(pool)
		if maxPerPriority > 0 && maxPerPriority < limit {
			limit = maxPerPriority
		}
		for i := 0; i < limit; i++ {
			ordered = append(ordered, pool[i])
			if maxTotal > 0 && len(ordered) >= maxTotal {
				return ordered
			}
		}
	}
	if maxTotal > 0 && len(ordered) > maxTotal {
		ordered = ordered[:maxTotal]
	}
	return ordered
}

func sortIntDesc(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] > a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// weightedShuffle 按 weight 无放回抽取，返回尝试顺序。
func weightedShuffle(pool []channelmanager.ChannelInfo) []channelmanager.ChannelInfo {
	remaining := make([]channelmanager.ChannelInfo, len(pool))
	copy(remaining, pool)
	out := make([]channelmanager.ChannelInfo, 0, len(pool))
	for len(remaining) > 0 {
		total := 0
		for _, c := range remaining {
			total += weightOf(c)
		}
		pick := rand.Intn(total)
		idx := 0
		for i, c := range remaining {
			w := weightOf(c)
			if pick < w {
				idx = i
				break
			}
			pick -= w
		}
		out = append(out, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return out
}

func weightOf(c channelmanager.ChannelInfo) int {
	if c.Weight <= 0 {
		return 100
	}
	return c.Weight
}
