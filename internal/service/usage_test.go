package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"MyApi/internal/config"
)

func TestComputeCost(t *testing.T) {
	// input 1000 (cached 400), output 500
	// pIn=30, pOut=60, pCached=15 per 1M
	got := computeCost(1000, 500, 400, 30, 60, 15)
	want := (float64(600)*30 + float64(400)*15 + float64(500)*60) / 1e6
	if got != want {
		t.Fatalf("computeCost=%v want %v", got, want)
	}
	if computeCost(0, 0, 0, 1, 1, 1) != 0 {
		t.Fatal("zero usage should be zero cost")
	}
}

func TestParseUsageJSON(t *testing.T) {
	body := []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":4}}}`)
	in, out, cached, content, ok := parseUsageJSON(body)
	if !ok || in != 10 || out != 3 || cached != 4 || content != "hello" {
		t.Fatalf("got in=%d out=%d cached=%d content=%q ok=%v", in, out, cached, content, ok)
	}
	if _, _, _, content, ok := parseUsageJSON([]byte(`{"choices":[{"message":{"content":"abcdef"}}]}`)); ok || content != "abcdef" {
		t.Fatalf("expected no usage, content=abcdef, got ok=%v content=%q", ok, content)
	}
}

func makeSSEChunk(content string) string {
	return `data: {"choices":[{"delta":{"content":"` + content + `"}}]}` + "\n\n"
}

const sseUsageOnly = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":1}}}\n\n"

func TestSSETeeStripsInjectedUsage(t *testing.T) {
	stream := makeSSEChunk("Hel") + makeSSEChunk("lo") + sseUsageOnly + "data: [DONE]\n\n"
	meta := &usageMeta{startedAt: time.Now(), count: func(s string) int { return len(s) }}
	tee := newSSEUsageTee(strings.NewReader(stream), meta, true)
	out, err := io.ReadAll(tee)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"usage"`) {
		t.Fatalf("usage-only frame should be stripped, got: %s", out)
	}
	if !strings.Contains(string(out), "[DONE]") {
		t.Fatal("DONE should be preserved")
	}
	if !meta.usageKnown || meta.inputTokens != 9 || meta.outputTokens != 2 || meta.cachedTokens != 1 {
		t.Fatalf("usage not captured: %+v", meta)
	}
	if meta.outputTokensEst != 5 {
		t.Fatalf("outputTokensEst=%d want 5", meta.outputTokensEst)
	}
}

func TestRewriteResponseModel(t *testing.T) {
	body := []byte(`{"id":"x","model":"deepseek-flash","system_fingerprint":"abc","choices":[]}`)
	out := rewriteResponseModel(body, "gpt-4")
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "gpt-4" {
		t.Fatalf("model not rewritten: %v", m["model"])
	}
	if _, ok := m["system_fingerprint"]; ok {
		t.Fatal("system_fingerprint should be removed")
	}
	// 无 model 字段的响应（如错误体）原样返回
	plain := []byte(`{"error":{"message":"x"}}`)
	if string(rewriteResponseModel(plain, "gpt-4")) != string(plain) {
		t.Fatal("body without model should be unchanged")
	}
}

func TestSSETeeRewritesModel(t *testing.T) {
	stream := makeSSEChunk("hi") + sseUsageOnly + "data: [DONE]\n\n"
	meta := &usageMeta{startedAt: time.Now(), model: "gpt-4"}
	tee := newSSEUsageTee(strings.NewReader(stream), meta, true)
	out, _ := io.ReadAll(tee)
	s := string(out)
	if strings.Contains(s, "deepseek") || strings.Contains(s, `"model":"gpt-4"`) == false {
		t.Fatalf("stream chunk model should be rewritten: %s", s)
	}
	if strings.Contains(s, `"usage"`) {
		t.Fatal("usage-only frame should still be stripped")
	}
}

func TestSSETeeKeepsRequestedUsage(t *testing.T) {
	stream := makeSSEChunk("hi") + sseUsageOnly + "data: [DONE]\n\n"
	meta := &usageMeta{startedAt: time.Now()}
	tee := newSSEUsageTee(strings.NewReader(stream), meta, false)
	out, _ := io.ReadAll(tee)
	if !strings.Contains(string(out), `"usage"`) {
		t.Fatal("usage frame should be kept when downstream requested it")
	}
}

// ---- flusher fakes ----

type fakeDeltaStore struct {
	mu   sync.Mutex
	live map[string]map[int64]usageDelta
	max  map[string]int64
	proc map[string]*fakeProc
	seq  int
}

type fakeProc struct {
	date string
	data map[int64]usageDelta
	max  int64
}

func newFakeDeltaStore() *fakeDeltaStore {
	return &fakeDeltaStore{live: map[string]map[int64]usageDelta{}, max: map[string]int64{}, proc: map[string]*fakeProc{}}
}

func (f *fakeDeltaStore) Add(ctx context.Context, date string, uid int64, d usageDelta, maxID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live[date] == nil {
		f.live[date] = map[int64]usageDelta{}
	}
	cur := f.live[date][uid]
	cur.add(d)
	f.live[date][uid] = cur
	if maxID > f.max[date] {
		f.max[date] = maxID
	}
	return nil
}

func (f *fakeDeltaStore) ActiveDates(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for d, m := range f.live {
		if len(m) > 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDeltaStore) Rename(ctx context.Context, date string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.live[date]) == 0 {
		return "", false, nil
	}
	f.seq++
	key := date + ":proc"
	f.proc[key] = &fakeProc{date: date, data: f.live[date], max: f.max[date]}
	delete(f.live, date)
	return key, true, nil
}

func (f *fakeDeltaStore) Read(ctx context.Context, processing string) (map[int64]usageDelta, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.proc[processing]
	if p == nil {
		return nil, 0, errors.New("proc gone")
	}
	out := map[int64]usageDelta{}
	for k, v := range p.data {
		out[k] = v
	}
	return out, p.max, nil
}

func (f *fakeDeltaStore) Drop(ctx context.Context, date, processing string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.proc, processing)
	return nil
}

func (f *fakeDeltaStore) MergeBack(ctx context.Context, date, processing string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.proc[processing]
	if p == nil {
		return nil
	}
	if f.live[date] == nil {
		f.live[date] = map[int64]usageDelta{}
	}
	for uid, d := range p.data {
		cur := f.live[date][uid]
		cur.add(d)
		f.live[date][uid] = cur
	}
	if p.max > f.max[date] {
		f.max[date] = p.max
	}
	delete(f.proc, processing)
	return nil
}

type dailyCall struct {
	date  string
	per   map[int64]usageDelta
	maxID int64
}

type fakeDaily struct {
	mu        sync.Mutex
	calls     []dailyCall
	failTimes int
}

func (f *fakeDaily) Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failTimes > 0 {
		f.failTimes--
		return errors.New("db down")
	}
	f.calls = append(f.calls, dailyCall{date: date, per: per, maxID: maxID})
	return nil
}

func newFlushService(delta deltaStore, daily dailyAggregator) *Service {
	s := &Service{cfg: config.Default(), log: discardLog(), delta: delta, daily: daily}
	s.statsLoc = time.UTC
	return s
}

func TestFlushOnceAppliesAndDrops(t *testing.T) {
	delta := newFakeDeltaStore()
	daily := &fakeDaily{}
	s := newFlushService(delta, daily)
	_ = delta.Add(context.Background(), "2026-09-09", 1, usageDelta{Input: 10, Output: 5, CostMicro: 123, Req: 1, OK: 1}, 42)

	s.flushOnce()

	if len(daily.calls) != 1 {
		t.Fatalf("expected 1 apply, got %d", len(daily.calls))
	}
	c := daily.calls[0]
	if c.date != "2026-09-09" || c.maxID != 42 || c.per[1].Input != 10 || c.per[1].CostMicro != 123 {
		t.Fatalf("unexpected daily call: %+v", c)
	}
	dates, _ := delta.ActiveDates(context.Background())
	if len(dates) != 0 {
		t.Fatalf("live should be empty after flush, got %v", dates)
	}
}

func TestFlushOnceMergesBackOnFailure(t *testing.T) {
	delta := newFakeDeltaStore()
	daily := &fakeDaily{failTimes: 1}
	s := newFlushService(delta, daily)
	_ = delta.Add(context.Background(), "2026-09-09", 1, usageDelta{Input: 10, Req: 1, OK: 1}, 42)

	s.flushOnce() // fails -> merge back

	if len(daily.calls) != 0 {
		t.Fatal("no successful apply expected")
	}
	dates, _ := delta.ActiveDates(context.Background())
	if len(dates) != 1 {
		t.Fatalf("delta should be merged back, got %v", dates)
	}
	// second attempt succeeds
	s.flushOnce()
	if len(daily.calls) != 1 || daily.calls[0].per[1].Input != 10 {
		t.Fatalf("expected retry to apply, got %+v", daily.calls)
	}
}

func TestDeltaRenameSnapshot(t *testing.T) {
	delta := newFakeDeltaStore()
	_ = delta.Add(context.Background(), "d", 1, usageDelta{Input: 1, Req: 1, OK: 1}, 1)
	proc, ok, _ := delta.Rename(context.Background(), "d")
	if !ok {
		t.Fatal("rename should succeed")
	}
	_ = delta.Add(context.Background(), "d", 1, usageDelta{Input: 2, Req: 1, OK: 1}, 2)
	old, _, _ := delta.Read(context.Background(), proc)
	if old[1].Input != 1 {
		t.Fatalf("processing snapshot should be frozen, got %+v", old[1])
	}
	live, _ := delta.ActiveDates(context.Background())
	if len(live) != 1 {
		t.Fatal("new live delta should exist independently")
	}
	_ = delta.Drop(context.Background(), "d", proc)
	per, _, _ := delta.Read(context.Background(), proc)
	if per != nil {
		t.Fatal("proc should be dropped")
	}
}
