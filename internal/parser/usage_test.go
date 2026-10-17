package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// One Opus 5 response written as three records: thinking (usage as it
// stood at message_start), text, tool_use (final usage). Plus one
// Fable-style response whose two records repeat the final figure, and
// one record with no response id at all.
const responseUsageFixture = `{"type":"user","timestamp":"2026-09-08T19:00:00.000Z","uuid":"u1","message":{"role":"user","content":"go"}}
{"type":"assistant","timestamp":"2026-09-08T19:00:01.000Z","uuid":"a1","parentUuid":"u1","message":{"id":"msg_A","model":"claude-opus-5","role":"assistant","content":[{"type":"thinking","thinking":"t"}],"usage":{"input_tokens":3,"output_tokens":1,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":200}}}}
{"type":"assistant","timestamp":"2026-09-08T19:00:02.000Z","uuid":"a2","parentUuid":"a1","message":{"id":"msg_A","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":3,"output_tokens":1,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":200}}}}
{"type":"assistant","timestamp":"2026-09-08T19:00:03.000Z","uuid":"a3","parentUuid":"a2","message":{"id":"msg_A","model":"claude-opus-5","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":3,"output_tokens":342,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":200}}}}
{"type":"user","timestamp":"2026-09-08T19:00:04.000Z","uuid":"u2","parentUuid":"a3","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","timestamp":"2026-09-08T19:00:05.000Z","uuid":"a4","parentUuid":"u2","message":{"id":"msg_B","model":"claude-fable-5-1","role":"assistant","content":[{"type":"thinking","thinking":"t"}],"usage":{"input_tokens":2,"output_tokens":874,"cache_read_input_tokens":500,"cache_creation_input_tokens":100}}}
{"type":"assistant","timestamp":"2026-09-08T19:00:06.000Z","uuid":"a5","parentUuid":"a4","message":{"id":"msg_B","model":"claude-fable-5-1","role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":2,"output_tokens":874,"cache_read_input_tokens":500,"cache_creation_input_tokens":100}}}
{"type":"assistant","timestamp":"2026-09-08T19:00:07.000Z","uuid":"a6","parentUuid":"a5","message":{"model":"claude-fable-5-1","role":"assistant","content":[{"type":"text","text":"legacy record, no id"}],"usage":{"input_tokens":7,"output_tokens":9,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`

func writeResponseUsageFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(responseUsageFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCollapseResponseUsage_LastRecordWins(t *testing.T) {
	session, err := ParseSession(writeResponseUsageFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var flat []*Message
	var walk func(*Message)
	walk = func(m *Message) {
		flat = append(flat, m)
		for _, c := range m.Children {
			walk(c)
		}
	}
	for _, r := range session.RootMessages {
		walk(r)
	}
	byUUID := map[string]*Message{}
	for _, m := range flat {
		byUUID[m.UUID] = m
	}
	for _, uuid := range []string{"a1", "a2", "a4"} {
		if byUUID[uuid].Usage != nil {
			t.Errorf("%s: earlier record of a response still carries usage %+v", uuid, *byUUID[uuid].Usage)
		}
	}
	if got := byUUID["a3"].Usage; got == nil || got.OutputTokens != 342 || got.CacheCreate1hTokens != 200 {
		t.Fatalf("a3 usage = %+v, want the final figure (342 out, 200 1h cache write)", got)
	}
	if got := byUUID["a5"].Usage; got == nil || got.OutputTokens != 874 {
		t.Fatalf("a5 usage = %+v, want 874 out", got)
	}
	if got := byUUID["a6"].Usage; got == nil || got.OutputTokens != 9 {
		t.Fatalf("a6 (no response id) usage = %+v, want its own 9 out", got)
	}
	if byUUID["a3"].ResponseID != "msg_A" || byUUID["a6"].ResponseID != "" {
		t.Errorf("ResponseID not carried: a3=%q a6=%q", byUUID["a3"].ResponseID, byUUID["a6"].ResponseID)
	}

	// Session totals are one figure per response, not per record.
	s := session.Stats
	if s.InputTokens != 3+2+7 || s.OutputTokens != 342+874+9 || s.CacheReadTokens != 1000+500 || s.CacheCreateTokens != 200+100 {
		t.Fatalf("stats = in %d out %d cr %d cw %d; want 12 / 1225 / 1500 / 300",
			s.InputTokens, s.OutputTokens, s.CacheReadTokens, s.CacheCreateTokens)
	}
}

func TestQuickParseMatchesFullParseAfterCollapse(t *testing.T) {
	path := writeResponseUsageFixture(t)
	full, err := ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, quick, _ := quickParseSession(path)
	f, q := full.Stats, quick
	if f.InputTokens != q.InputTokens || f.OutputTokens != q.OutputTokens ||
		f.CacheReadTokens != q.CacheReadTokens || f.CacheCreateTokens != q.CacheCreateTokens {
		t.Fatalf("token totals differ: full %+v quick %+v", f, q)
	}
	if diff := f.CostUSD - q.CostUSD; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost differs: full %.6f quick %.6f", f.CostUSD, q.CostUSD)
	}
	if f.CostUSD <= 0 {
		t.Fatalf("cost should be priced, got %.6f", f.CostUSD)
	}
}

func TestOneHourCacheWriteIsPricedAtTheOneHourRate(t *testing.T) {
	session, err := ParseSession(writeResponseUsageFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	// msg_A: opus-5, 3 in, 342 out, 1000 cache read, 200 cache write all 1h.
	// Rates: 5/25, read 0.5, 1h write 10 (= 2x input).
	want := (3*5.0 + 342*25.0 + 1000*0.5 + 200*10.0) / 1e6
	var got float64
	var walk func(*Message)
	walk = func(m *Message) {
		if m.ResponseID == "msg_A" && m.Usage != nil {
			got = m.Usage.CostUSD
		}
		for _, c := range m.Children {
			walk(c)
		}
	}
	for _, r := range session.RootMessages {
		walk(r)
	}
	if diff := got - want; diff > 1e-12 || diff < -1e-12 {
		t.Fatalf("msg_A cost = %.9f, want %.9f (1h cache write at 2x input)", got, want)
	}
}
