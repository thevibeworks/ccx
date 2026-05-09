// Command ccx-audit-schema audits a Claude Code session JSONL file
// against ccx's known rawMessage schema. Reports unknown top-level
// fields, unknown message types/subtypes, content block types, and
// tool names.
//
// Usage:
//
//	go run ./cmd/ccx-audit-schema <session.jsonl>
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// knownFields mirrors the json tags on rawMessage in internal/parser/types.go.
// Keep in sync manually.
var knownFields = map[string]bool{
	"type": true, "subtype": true, "timestamp": true,
	"uuid": true, "parentUuid": true, "logicalParentUuid": true,
	"sessionId": true, "isCompactSummary": true, "isSidechain": true,
	"isMeta": true, "agentId": true, "message": true,
	"content": true, "summary": true, "leafUuid": true,
	"usage": true, "slug": true, "version": true,
	"gitBranch": true, "cwd": true, "aiTitle": true,
	"entrypoint": true, "userType": true, "stopReason": true,
	"durationMs": true, "url": true, "toolUseResult": true,
}

// knownTypes are message types ccx processes or explicitly skips.
var knownTypes = map[string]bool{
	"user": true, "assistant": true, "system": true, "summary": true,
	// Skipped but accounted for:
	"permission-mode": true, "attachment": true, "ai-title": true,
	"file-history-snapshot": true, "last-prompt": true, "agent-name": true,
	"custom-title": true, "queue-operation": true,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ccx-audit-schema <session.jsonl>\n")
		os.Exit(2)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fatal("open: %v", err)
	}
	defer f.Close()

	var (
		sessionID    string
		total        int
		typeCounts   = map[string]int{}
		unknownKeys  = map[string]int{} // field -> count
		unknownTypes = map[string]int{}
		subtypes     = map[string]int{}
		blockTypes   = map[string]int{}
		toolNames    = map[string]int{}
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		total++

		// Decode into generic map to find all keys.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "line %d: bad json: %v\n", total, err)
			continue
		}

		// Collect session ID from first message that has one.
		if sessionID == "" {
			if v, ok := raw["sessionId"]; ok {
				var s string
				_ = json.Unmarshal(v, &s)
				if s != "" {
					sessionID = s
				}
			}
		}

		// Unknown top-level fields.
		for k := range raw {
			if !knownFields[k] {
				unknownKeys[k]++
			}
		}

		// Message type.
		var msgType string
		if v, ok := raw["type"]; ok {
			_ = json.Unmarshal(v, &msgType)
		}
		if msgType != "" {
			typeCounts[msgType]++
			if !knownTypes[msgType] {
				unknownTypes[msgType]++
			}
		}

		// Subtype.
		var subtype string
		if v, ok := raw["subtype"]; ok {
			_ = json.Unmarshal(v, &subtype)
		}
		if subtype != "" {
			subtypes[subtype]++
		}

		// Content blocks (inside message.content array).
		if v, ok := raw["message"]; ok {
			var msg struct {
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(v, &msg) == nil && len(msg.Content) > 0 && msg.Content[0] == '[' {
				var blocks []struct {
					Type string `json:"type"`
					Name string `json:"name"`
				}
				if json.Unmarshal(msg.Content, &blocks) == nil {
					for _, b := range blocks {
						if b.Type != "" {
							blockTypes[b.Type]++
						}
						if b.Type == "tool_use" && b.Name != "" {
							toolNames[b.Name]++
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fatal("scan: %v", err)
	}

	// --- Output ---
	if sessionID != "" {
		fmt.Printf("session: %s\n", sessionID)
	}
	fmt.Printf("messages: %d (%s)\n", total, formatCounts(typeCounts))

	fmt.Printf("\nUNKNOWN FIELDS (not in rawMessage):\n")
	if len(unknownKeys) == 0 {
		fmt.Printf("  (none)\n")
	} else {
		for _, kv := range sortedKV(unknownKeys) {
			fmt.Printf("  %s (seen in %d messages)\n", kv.k, kv.v)
		}
	}

	fmt.Printf("\nUNKNOWN MESSAGE TYPES:\n")
	if len(unknownTypes) == 0 {
		fmt.Printf("  (none -- all types accounted for)\n")
	} else {
		for _, kv := range sortedKV(unknownTypes) {
			fmt.Printf("  %s (%d)\n", kv.k, kv.v)
		}
	}

	if len(subtypes) > 0 {
		fmt.Printf("\nSUBTYPES:\n")
		for _, kv := range sortedKV(subtypes) {
			fmt.Printf("  %s: %d\n", kv.k, kv.v)
		}
	}

	fmt.Printf("\nTOOL NAMES:\n")
	if len(toolNames) == 0 {
		fmt.Printf("  (none)\n")
	} else {
		fmt.Printf("  %s\n", formatCounts(toolNames))
	}

	fmt.Printf("\nCONTENT BLOCK TYPES:\n")
	if len(blockTypes) == 0 {
		fmt.Printf("  (none)\n")
	} else {
		fmt.Printf("  %s\n", formatCounts(blockTypes))
	}
}

type kv struct {
	k string
	v int
}

func sortedKV(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].k < out[j].k })
	return out
}

func formatCounts(m map[string]int) string {
	kvs := sortedKV(m)
	parts := make([]string, len(kvs))
	for i, kv := range kvs {
		parts[i] = fmt.Sprintf("%s:%d", kv.k, kv.v)
	}
	return strings.Join(parts, " ")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ccx-audit-schema: "+format+"\n", args...)
	os.Exit(2)
}
