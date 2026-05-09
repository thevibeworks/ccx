# Long-session load profile — baseline + #4 fix

## Baseline (before #4)

Captured 2026-04-14 on `goos: linux / goarch: arm64` (benchmark host).

### `ParseSession` (full parser) on synthetic realistic fixtures

Fixture shape: mix of user prompts, assistant messages with thinking +
text + tool_use + usage fields, tool results with 2KB output, plus a
sidechain every 20 messages. See
`internal/parser/session_test.go#writeLargeFixture`.

```
BenchmarkParseSession_1500Messages-10     190     18_852_310 ns/op    80.28 MB/s    36_463_806 B/op    461_295 allocs/op
BenchmarkParseSession_5000Messages-10      66     53_955_465 ns/op    93.50 MB/s   121_460_452 B/op  1_537_578 allocs/op
```

Translated: a 1500-message session takes **~19 ms** to cold-parse, a
5000-message session takes **~54 ms**. Throughput caps around 80–93 MB/s,
limited by `encoding/json.Unmarshal` decoding per line.

Allocation density is ~300 allocs per message. The dominant allocators
are:

1. `json.Unmarshal` unpacking each line into `rawMessage` + `any`-typed
   `content` field
2. `parseContent` iterating `[]any` and converting each block to a
   `ContentBlock`
3. `Message` struct construction in `parseMessage`

None of the above was low-hanging fruit — the existing structure is
already reasonable. The real waste was at a higher level: **ccx was
re-parsing the same session file on every page view**. Clicking between
sessions A→B→A→B paid the full ~20 ms cost four times over.

## Fix: in-memory LRU cache at `provider.Multi`

A bounded LRU (cap 16) keyed by absolute file path, invalidated by
`(mtime, size)`. Sits in front of backend dispatch in
`Multi.ParseSession`. Transparent to callers, automatic for all web
handlers and CLI commands that flow through `Multi`.

### After (`BenchmarkSessionCache_ColdVsWarm`, 1500-msg fixture)

```
Cold (no cache)    675     3_601_948 ns/op   4_050_219 B/op     30_793 allocs/op
Warm (cache hit) 6_954_960       329.6 ns/op       256 B/op          2 allocs/op
```

(Cache benchmark uses a simpler fixture shape than the parser bench,
hence the 3.6 ms cold baseline vs 19 ms for the full-shape bench — the
relative speedup is the point, not the absolute cold number.)

Cache hit is **~11_000× faster** than cold parse on this fixture. On
the full-shape 1500-message bench the warm-vs-cold delta is closer to
**60_000×** (20 ms → 330 ns).

### What this means in the web UI

- First view of a session: unchanged (cold parse cost stands).
- Every subsequent view within the same `ccx web` process: near-zero
  parse overhead. The remaining request latency is HTML render only.
- Memory ceiling: 16 entries × per-session tree size. Typical sessions
  are 10–20 MB each, so ~200–300 MB worst case. Sessions are evicted
  LRU once the cap is hit.

## What's NOT in this fix (explicit scope)

- **No SQLite persistence.** Cache is process-local. `ccx web` restart
  flushes it. Acceptable tradeoff for a local dev tool; persistence
  adds serialization/deserialization cost that partially negates the
  in-memory win.
- **No streaming render.** The HTML template still renders in one
  pass. If/when a session's HTML render itself becomes the bottleneck
  (not parse), that's a separate issue.
- **No lazy-load endpoint.** "Load earlier" still reloads with
  `?all=1`. The cache makes that reload cheap, which is most of what
  users actually notice.
- **Parser allocation reduction.** ~300 allocs/message is higher than
  strictly necessary but the cache sidesteps the cost entirely for
  repeat views. A cold-path optimization pass can come later if real
  user sessions (captured, not synthetic) show the cold-path matters.

## How to reproduce

```bash
go test -run '^$' -bench 'BenchmarkParseSession' -benchtime=3s ./internal/parser/
go test -run '^$' -bench 'BenchmarkSessionCache' -benchtime=2s ./internal/provider/
```

## Invalidation notes

The cache compares `(mtime, size)` to the on-disk file on every `Get`.
If the session file is rewritten (e.g., Claude Code appends new turns
during a live session), the next access re-parses. Mid-session live
tail still works: ccx watches the file for changes via `tail`, and the
cache auto-invalidates when the file grows.
