package service

import "sort"

// Models 返回当前对下游发布的网关模型名（仅启用渠道/映射），并按 Key 白名单过滤。
func (s *Service) Models(ident *KeyIdentity) []string {
	if s.Channels == nil {
		return nil
	}
	snap := s.Channels.Snapshot()
	out := make([]string, 0, len(snap.Models))
	for name := range snap.Models {
		if !ident.AllowAll {
			allowed := false
			for _, m := range ident.Models {
				if m == name {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
