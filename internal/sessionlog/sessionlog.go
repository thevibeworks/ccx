package sessionlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider/codex"
)

const (
	Kind                  = "ccx.log.v1"
	maxScannerBufferBytes = 10 * 1024 * 1024
)

type Source struct {
	Provider string
	Home     string
}

type Options struct {
	Start         time.Time
	End           time.Time
	ScopeName     string
	ScopeLabel    string
	TimeZone      string
	Provider      string
	WorkspacePath string
	ProjectName   string
	Limit         int
	IncludeRaw    bool
	Now           time.Time
	// Kinds keeps only records of these kinds (empty = all). Match
	// keeps only records whose raw transcript line matches (nil =
	// all); it sees the raw line, not the bounded Text, so a term
	// past the text budget still matches — grep parity. Sessions,
	// their Kinds, and Metrics.Records stay scope-wide; only the
	// records list (and the aggregates built from it) narrow.
	Kinds []string
	Match func(rawLine string) bool
}

type Bundle struct {
	Kind        string       `json:"kind"`
	Scope       ScopeSummary `json:"scope"`
	GeneratedAt time.Time    `json:"generated_at"`
	Metrics     Metrics      `json:"metrics"`
	// Days/Providers/Workspaces pre-aggregate the in-scope records so
	// consumers (insight reports, the ccx-insight skill) can answer
	// "what happened when/where" without re-bucketing thousands of
	// records themselves. Computed before any record limit is applied.
	Days       []Aggregate    `json:"days,omitempty"`
	Providers  []Aggregate    `json:"providers,omitempty"`
	Workspaces []Aggregate    `json:"workspaces,omitempty"`
	Sessions   []SessionSlice `json:"sessions"`
	Records    []Record       `json:"records"`
}

// Aggregate is one bucket of activity: a calendar day (in the scope's
// timezone), a provider, or a workspace, identified by Key.
type Aggregate struct {
	Key               string `json:"key"`
	Sessions          int    `json:"sessions"`
	Records           int    `json:"records"`
	UserPrompts       int    `json:"user_prompts"`
	AssistantMessages int    `json:"assistant_messages"`
	ToolCalls         int    `json:"tool_calls"`
	Sidechains        int    `json:"sidechains"`
}

type ScopeSummary struct {
	Name      string    `json:"name"`
	Label     string    `json:"label"`
	TimeZone  string    `json:"time_zone"`
	Provider  string    `json:"provider,omitempty"`
	Project   string    `json:"project,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Generated string    `json:"generated"`
}

type Metrics struct {
	Sessions            int  `json:"sessions"`
	LongRunningSessions int  `json:"long_running_sessions"`
	Workspaces          int  `json:"workspaces"`
	SourceFiles         int  `json:"source_files"`
	Records             int  `json:"records"`
	RecordsMatched      int  `json:"records_matched,omitempty"` // after --kind/--match, before limit
	RecordsReturned     int  `json:"records_returned"`
	Limit               int  `json:"limit,omitempty"`
	Truncated           bool `json:"truncated"`
	UserPrompts         int  `json:"user_prompts"`
	AssistantMessages   int  `json:"assistant_messages"`
	ToolCalls           int  `json:"tool_calls"`
	ToolResults         int  `json:"tool_results"`
	Reasoning           int  `json:"reasoning"`
	Sidechains          int  `json:"sidechains"`
}

type SessionSlice struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	Project     string         `json:"project,omitempty"`
	Workspace   string         `json:"workspace,omitempty"`
	SourceFile  string         `json:"source_file"`
	Start       time.Time      `json:"start"`
	End         time.Time      `json:"end"`
	FirstRecord time.Time      `json:"first_record"`
	LastRecord  time.Time      `json:"last_record"`
	Records     int            `json:"records"`
	Kinds       map[string]int `json:"kinds,omitempty"`
	Preview     string         `json:"preview,omitempty"`
	Relation    ScopeRelation  `json:"relation"`
}

type ScopeRelation struct {
	OverlapsScope      bool `json:"overlaps_scope"`
	StartedBeforeScope bool `json:"started_before_scope"`
	StartedInScope     bool `json:"started_in_scope"`
	EndedInScope       bool `json:"ended_in_scope"`
	EndedAfterScope    bool `json:"ended_after_scope"`
	SpansWholeScope    bool `json:"spans_whole_scope"`
}

type Record struct {
	matched bool // passed Options.Kinds/Match; unexported, filtered out in Collect

	Timestamp   time.Time       `json:"timestamp"`
	Provider    string          `json:"provider"`
	SessionID   string          `json:"session_id"`
	Workspace   string          `json:"workspace,omitempty"`
	Project     string          `json:"project,omitempty"`
	SourceFile  string          `json:"source_file"`
	Line        int             `json:"line"`
	Type        string          `json:"type"`
	Kind        string          `json:"kind"`
	Role        string          `json:"role,omitempty"`
	TurnID      string          `json:"turn_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	UUID        string          `json:"uuid,omitempty"`
	ParentUUID  string          `json:"parent_uuid,omitempty"`
	IsSidechain bool            `json:"is_sidechain,omitempty"`
	IsSubagent  bool            `json:"is_subagent,omitempty"`
	Text        string          `json:"text,omitempty"`
	RawJSON     json.RawMessage `json:"raw_json,omitempty"`
}

type rawLine struct {
	Timestamp        string          `json:"timestamp"`
	Type             string          `json:"type"`
	SessionID        string          `json:"sessionId"`
	UUID             string          `json:"uuid"`
	ParentUUID       string          `json:"parentUuid"`
	IsSidechain      bool            `json:"isSidechain"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	CWD              string          `json:"cwd"`
	Summary          string          `json:"summary"`
	Message          json.RawMessage `json:"message"`
	Payload          json.RawMessage `json:"payload"`
}

type fileScan struct {
	SessionID  string
	Provider   string
	Project    string
	Workspace  string
	SourceFile string
	Start      time.Time
	End        time.Time
	Records    []Record
	Kinds      map[string]int
	Preview    string
}

func Collect(sources []Source, opts Options) (*Bundle, error) {
	if opts.End.IsZero() {
		return nil, fmt.Errorf("end time is required")
	}
	if opts.Start.IsZero() {
		return nil, fmt.Errorf("start time is required")
	}
	if !opts.Start.Before(opts.End) {
		return nil, fmt.Errorf("start must be before end")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	var scans []fileScan
	for _, source := range sources {
		source.Provider = strings.TrimSpace(source.Provider)
		source.Home = strings.TrimSpace(source.Home)
		if source.Home == "" {
			continue
		}
		if opts.Provider != "" && source.Provider != opts.Provider {
			continue
		}
		files, err := discoverFiles(source)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			scan, err := scanFile(source, file, opts)
			if err != nil {
				return nil, err
			}
			if scan == nil || len(scan.Records) == 0 {
				continue
			}
			if !matchesWorkspace(*scan, opts.WorkspacePath) {
				continue
			}
			if !matchesProject(*scan, opts.ProjectName) {
				continue
			}
			scans = append(scans, *scan)
		}
	}

	bundle := &Bundle{
		Kind:        Kind,
		GeneratedAt: now,
		Scope: ScopeSummary{
			Name:      emptyDefault(opts.ScopeName, "custom"),
			Label:     emptyDefault(opts.ScopeLabel, "Custom range"),
			TimeZone:  opts.TimeZone,
			Provider:  opts.Provider,
			Project:   opts.ProjectName,
			Workspace: opts.WorkspacePath,
			Start:     opts.Start,
			End:       opts.End,
			Generated: now.Format(time.RFC3339),
		},
	}

	sort.SliceStable(scans, func(i, j int) bool {
		if scans[i].Records[0].Timestamp.Equal(scans[j].Records[0].Timestamp) {
			return scans[i].SourceFile < scans[j].SourceFile
		}
		return scans[i].Records[0].Timestamp.Before(scans[j].Records[0].Timestamp)
	})

	for _, scan := range scans {
		session := SessionSlice{
			ID:          scan.SessionID,
			Provider:    scan.Provider,
			Project:     scan.Project,
			Workspace:   scan.Workspace,
			SourceFile:  scan.SourceFile,
			Start:       scan.Start,
			End:         scan.End,
			FirstRecord: scan.Records[0].Timestamp,
			LastRecord:  scan.Records[len(scan.Records)-1].Timestamp,
			Records:     len(scan.Records),
			Kinds:       scan.Kinds,
			Preview:     scan.Preview,
			Relation:    relationFor(scan.Start, scan.End, opts.Start, opts.End),
		}
		bundle.Sessions = append(bundle.Sessions, session)
		bundle.Records = append(bundle.Records, scan.Records...)
	}
	sort.SliceStable(bundle.Records, func(i, j int) bool {
		if bundle.Records[i].Timestamp.Equal(bundle.Records[j].Timestamp) {
			if bundle.Records[i].SourceFile == bundle.Records[j].SourceFile {
				return bundle.Records[i].Line < bundle.Records[j].Line
			}
			return bundle.Records[i].SourceFile < bundle.Records[j].SourceFile
		}
		return bundle.Records[i].Timestamp.Before(bundle.Records[j].Timestamp)
	})
	filtered := len(opts.Kinds) > 0 || opts.Match != nil
	if filtered {
		bundle.Records = filterRecords(bundle.Records, opts.Kinds)
	}
	matched := len(bundle.Records)
	bundle.Days, bundle.Providers, bundle.Workspaces = aggregateRecords(bundle.Records, now.Location())
	if opts.Limit > 0 && len(bundle.Records) > opts.Limit {
		bundle.Records = bundle.Records[:opts.Limit]
	}
	bundle.Metrics = metricsFor(bundle.Sessions, len(bundle.Records), opts.Limit, opts.Start, opts.End)
	if filtered {
		bundle.Metrics.RecordsMatched = matched
	}
	return bundle, nil
}

// filterRecords keeps records that passed Match and are of one of the
// kinds (kind demotion has already run, so "user_prompt" means the
// visible conversation on every provider).
func filterRecords(records []Record, kinds []string) []Record {
	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		if k = strings.TrimSpace(k); k != "" {
			want[k] = true
		}
	}
	out := records[:0]
	for _, r := range records {
		if !r.matched {
			continue
		}
		if len(want) > 0 && !want[r.Kind] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// aggregateRecords buckets records by calendar day (in loc), provider,
// and workspace. Days sort chronologically; providers and workspaces
// sort by record volume, busiest first.
func aggregateRecords(records []Record, loc *time.Location) (days, providers, workspaces []Aggregate) {
	type bucket struct {
		agg      Aggregate
		sessions map[string]struct{}
	}
	tally := func(m map[string]*bucket, key string, r Record) {
		b := m[key]
		if b == nil {
			b = &bucket{agg: Aggregate{Key: key}, sessions: make(map[string]struct{})}
			m[key] = b
		}
		b.sessions[r.SessionID] = struct{}{}
		b.agg.Records++
		switch r.Kind {
		case "user_prompt":
			b.agg.UserPrompts++
		case "assistant_message":
			b.agg.AssistantMessages++
		case "tool_call":
			b.agg.ToolCalls++
		}
		if r.IsSidechain || r.IsSubagent {
			b.agg.Sidechains++
		}
	}

	dayBuckets := make(map[string]*bucket)
	providerBuckets := make(map[string]*bucket)
	workspaceBuckets := make(map[string]*bucket)
	for _, r := range records {
		tally(dayBuckets, r.Timestamp.In(loc).Format("2006-01-02"), r)
		tally(providerBuckets, r.Provider, r)
		workspace := r.Workspace
		if workspace == "" {
			workspace = r.Project
		}
		if workspace == "" {
			workspace = "(unknown)"
		}
		tally(workspaceBuckets, workspace, r)
	}

	collect := func(m map[string]*bucket) []Aggregate {
		out := make([]Aggregate, 0, len(m))
		for _, b := range m {
			b.agg.Sessions = len(b.sessions)
			out = append(out, b.agg)
		}
		return out
	}

	days = collect(dayBuckets)
	sort.Slice(days, func(i, j int) bool { return days[i].Key < days[j].Key })
	providers = collect(providerBuckets)
	workspaces = collect(workspaceBuckets)
	byVolume := func(s []Aggregate) func(int, int) bool {
		return func(i, j int) bool {
			if s[i].Records != s[j].Records {
				return s[i].Records > s[j].Records
			}
			return s[i].Key < s[j].Key
		}
	}
	sort.Slice(providers, byVolume(providers))
	sort.Slice(workspaces, byVolume(workspaces))
	return days, providers, workspaces
}

func discoverFiles(source Source) ([]string, error) {
	var roots []string
	switch source.Provider {
	case "claude-code":
		roots = []string{filepath.Join(source.Home, "projects")}
	case "codex":
		roots = []string{
			filepath.Join(source.Home, "sessions"),
			filepath.Join(source.Home, "archived_sessions"),
		}
	default:
		return nil, fmt.Errorf("unknown provider %q", source.Provider)
	}

	var files []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".jsonl") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func scanFile(source Source, filePath string, opts Options) (*fileScan, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scan := &fileScan{
		SessionID:  fallbackSessionID(filePath),
		Provider:   source.Provider,
		SourceFile: filePath,
		Kinds:      make(map[string]int),
	}
	if source.Provider == "claude-code" {
		scan.Workspace, scan.Project = claudeWorkspaceForFile(source.Home, filePath)
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferBytes)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw rawLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		timestamp := recordTimestamp(raw)
		if timestamp.IsZero() {
			continue
		}
		recordSessionBounds(scan, timestamp)
		updateFileMetadata(scan, source.Provider, raw)

		if timestamp.Before(opts.Start) || !timestamp.Before(opts.End) {
			continue
		}
		record := normalizeRecord(source.Provider, raw)
		record.Timestamp = timestamp
		record.Provider = source.Provider
		record.SessionID = firstNonEmpty(record.SessionID, scan.SessionID)
		record.Workspace = firstNonEmpty(record.Workspace, scan.Workspace)
		record.Project = firstNonEmpty(record.Project, scan.Project)
		record.SourceFile = filePath
		record.Line = lineNo
		if strings.Contains(filePath, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
			record.IsSubagent = true
		}
		if opts.IncludeRaw {
			record.RawJSON = json.RawMessage([]byte(line))
		}
		record.matched = opts.Match == nil || opts.Match(line)
		scan.Records = append(scan.Records, record)
		scan.Kinds[record.Kind]++
		if scan.Preview == "" && record.Text != "" && (record.Kind == "user_prompt" || record.Kind == "assistant_message") {
			scan.Preview = record.Text
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(scan.Records) == 0 {
		return nil, nil
	}
	if source.Provider == "codex" && demoteLegacyCodexMessages(scan.Records) {
		// Kinds and preview were tallied while streaming; redo them
		// against the demoted kinds so metrics count the conversation
		// once and the preview is a visible prompt, not an envelope.
		scan.Kinds = make(map[string]int)
		scan.Preview = ""
		for _, record := range scan.Records {
			scan.Kinds[record.Kind]++
			if scan.Preview == "" && record.Text != "" && (record.Kind == "user_prompt" || record.Kind == "assistant_message") {
				scan.Preview = record.Text
			}
		}
	}
	for i := range scan.Records {
		if (scan.Records[i].SessionID == "" || scan.Records[i].SessionID == fallbackSessionID(filePath)) && scan.SessionID != "" {
			scan.Records[i].SessionID = scan.SessionID
		}
		if scan.Records[i].Workspace == "" {
			scan.Records[i].Workspace = scan.Workspace
		}
		if scan.Records[i].Project == "" {
			scan.Records[i].Project = scan.Project
		}
	}
	return scan, nil
}

func recordTimestamp(raw rawLine) time.Time {
	if t := parseTimestamp(raw.Timestamp); !t.IsZero() {
		return t
	}
	if len(raw.Payload) > 0 {
		obj := decodeObject(raw.Payload)
		if t := parseTimestamp(stringField(obj, "timestamp")); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func recordSessionBounds(scan *fileScan, timestamp time.Time) {
	if scan.Start.IsZero() || timestamp.Before(scan.Start) {
		scan.Start = timestamp
	}
	if scan.End.IsZero() || timestamp.After(scan.End) {
		scan.End = timestamp
	}
}

func updateFileMetadata(scan *fileScan, provider string, raw rawLine) {
	switch provider {
	case "claude-code":
		if raw.SessionID != "" {
			scan.SessionID = raw.SessionID
		}
		if raw.CWD != "" {
			scan.Workspace = raw.CWD
			scan.Project = projectNameForWorkspace(raw.CWD)
		}
	case "codex":
		obj := decodeObject(raw.Payload)
		switch raw.Type {
		case "session_meta":
			if id := stringField(obj, "id"); id != "" {
				scan.SessionID = id
			}
			if cwd := stringField(obj, "cwd"); cwd != "" {
				scan.Workspace = cwd
				scan.Project = projectNameForWorkspace(cwd)
			}
		case "turn_context":
			if scan.Workspace == "" {
				if cwd := stringField(obj, "cwd"); cwd != "" {
					scan.Workspace = cwd
					scan.Project = projectNameForWorkspace(cwd)
				}
			}
		}
	}
}

func normalizeRecord(provider string, raw rawLine) Record {
	switch provider {
	case "claude-code":
		return normalizeClaudeRecord(raw)
	case "codex":
		return normalizeCodexRecord(raw)
	default:
		return Record{Type: raw.Type, Kind: emptyDefault(raw.Type, "unknown")}
	}
}

func normalizeClaudeRecord(raw rawLine) Record {
	record := Record{
		SessionID:   raw.SessionID,
		Workspace:   raw.CWD,
		Project:     projectNameForWorkspace(raw.CWD),
		Type:        raw.Type,
		Kind:        emptyDefault(raw.Type, "unknown"),
		UUID:        raw.UUID,
		ParentUUID:  raw.ParentUUID,
		IsSidechain: raw.IsSidechain,
	}
	msg := decodeObject(raw.Message)
	record.Role = stringField(msg, "role")
	content := msg["content"]
	record.Text = truncateText(cleanText(contentPreview(content)), 1000)

	switch raw.Type {
	case "user":
		// Same rules as the full parser (parser.classifyMessage): a
		// user-role line is a human prompt only when it is not a
		// compaction carrier, an injected meta message (skill bodies,
		// system instructions), a tool result, or a harness XML
		// wrapper (slash-command markers, local command echoes,
		// task notifications). Otherwise "the humans in the loop"
		// shows things no human typed.
		switch {
		case raw.IsCompactSummary:
			record.Kind = "compact_summary"
		case raw.IsMeta:
			record.Kind = "meta"
		case contentHasType(content, "tool_result"):
			record.Kind = "tool_result"
			if parser.IsToolDenial(record.Text) {
				record.Kind = "tool_denied"
			}
		default:
			if kind, ok := parser.ClassifyUserText(userTextForClassify(content)); ok {
				record.Kind = string(kind)
			} else {
				record.Kind = "user_prompt"
			}
		}
	case "assistant":
		if contentHasType(content, "tool_use") {
			record.Kind = "tool_call"
		} else {
			record.Kind = "assistant_message"
		}
	case "summary":
		record.Kind = "summary"
		if record.Text == "" {
			record.Text = truncateText(cleanText(raw.Summary), 1000)
		}
	case "system":
		record.Kind = "system"
	}
	return record
}

// userTextForClassify returns the leading text of a user-role content
// value the way the parser sees it: the first text block, or the raw
// string content.
func userTextForClassify(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if stringField(block, "type") == "text" {
				return stringField(block, "text")
			}
			return ""
		}
	}
	return ""
}

func normalizeCodexRecord(raw rawLine) Record {
	payload := decodeObject(raw.Payload)
	payloadType := stringField(payload, "type")
	record := Record{
		Type: raw.Type,
		Kind: emptyDefault(raw.Type, "unknown"),
	}

	switch raw.Type {
	case "session_meta":
		record.Kind = "session_meta"
		record.SessionID = stringField(payload, "id")
		record.Workspace = stringField(payload, "cwd")
		record.Project = projectNameForWorkspace(record.Workspace)
		record.Text = joinNonEmpty(" ", "session", record.SessionID, record.Workspace)
	case "turn_context":
		record.Kind = "turn_context"
		record.Workspace = stringField(payload, "cwd")
		record.Project = projectNameForWorkspace(record.Workspace)
		record.TurnID = stringField(payload, "turn_id")
		record.Text = joinNonEmpty(" ", "turn", record.TurnID, stringField(payload, "model"), record.Workspace)
	case "event_msg":
		record.Type = joinNonEmpty(":", raw.Type, payloadType)
		if payloadType == "item_completed" {
			// Codex 0.147: the visible transcript is item_completed
			// UserMessage/AgentMessage TurnItems (docs/design/0004).
			if message, ok := codex.DecodeCompletedTurnMessage(raw.Payload); ok {
				record.UUID = message.ID
				record.Role = message.Role
				record.Kind = "assistant_message"
				if message.Role == "user" {
					record.Kind = "user_prompt"
				}
				record.Text = truncateText(cleanText(message.Text), 1000)
				break
			}
		}
		normalizeCodexEvent(&record, payload, payloadType)
	case "response_item":
		record.Type = joinNonEmpty(":", raw.Type, payloadType)
		normalizeCodexResponseItem(&record, payload, payloadType)
	default:
		if payloadType != "" {
			record.Type = joinNonEmpty(":", raw.Type, payloadType)
		}
		record.Text = truncateText(cleanText(contentPreview(payload)), 1000)
	}
	return record
}

func normalizeCodexEvent(record *Record, payload map[string]any, payloadType string) {
	switch payloadType {
	case "user_message":
		record.Kind = "user_prompt"
		record.Role = "user"
		record.Text = truncateText(cleanText(stringField(payload, "message")), 1000)
	case "agent_message":
		record.Kind = "assistant_message"
		record.Role = "assistant"
		record.Text = truncateText(cleanText(stringField(payload, "message")), 1000)
	case "agent_reasoning":
		record.Kind = "reasoning"
		record.Text = truncateText(cleanText(stringField(payload, "text")), 1000)
	case "exec_command_end":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.TurnID = stringField(payload, "turn_id")
		record.Workspace = stringField(payload, "cwd")
		record.Project = projectNameForWorkspace(record.Workspace)
		record.Text = truncateText(cleanText(execCommandPreview(payload)), 1000)
	case "patch_apply_end":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.TurnID = stringField(payload, "turn_id")
		record.Text = truncateText(cleanText("apply_patch "+statusPreview(payload)), 1000)
	case "web_search_end":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.Text = truncateText(cleanText(joinNonEmpty(" ", "web_search", stringField(payload, "query"))), 1000)
	case "mcp_tool_call_end":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.Text = truncateText(cleanText("mcp_tool_call "+contentPreview(payload["invocation"])), 1000)
	case "dynamic_tool_call_response", "view_image_tool_call":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.Text = truncateText(cleanText(joinNonEmpty(" ", payloadType, stringField(payload, "tool"), stringField(payload, "path"), statusPreview(payload))), 1000)
	case "token_count":
		record.Kind = "usage"
		record.Text = truncateText(cleanText(contentPreview(payload["info"])), 1000)
	case "thread_name_updated":
		record.Kind = "session_update"
		if threadName, ok := payload["thread_name"].(string); ok {
			record.Text = truncateText(cleanText(threadName), 1000)
		}
	case "compacted":
		record.Kind = "compaction"
		record.Text = truncateText(cleanText(stringField(payload, "message")), 1000)
	default:
		record.Kind = emptyDefault(payloadType, "event")
		record.Text = truncateText(cleanText(contentPreview(payload)), 1000)
	}
}

func normalizeCodexResponseItem(record *Record, payload map[string]any, payloadType string) {
	switch payloadType {
	case "message":
		record.Role = stringField(payload, "role")
		switch record.Role {
		case "user":
			record.Kind = "user_prompt"
		case "assistant":
			record.Kind = "assistant_message"
		case "developer", "system":
			record.Kind = "instruction"
		default:
			record.Kind = "message"
		}
		record.Text = truncateText(cleanText(contentPreview(payload["content"])), 1000)
	case "function_call", "custom_tool_call", "web_search_call":
		record.Kind = "tool_call"
		record.CallID = stringField(payload, "call_id")
		record.Text = truncateText(cleanText(toolCallPreview(payload, payloadType)), 1000)
	case "function_call_output", "custom_tool_call_output":
		record.Kind = "tool_result"
		record.CallID = stringField(payload, "call_id")
		record.Text = truncateText(cleanText(contentPreview(payload["output"])), 1000)
	case "reasoning":
		record.Kind = "reasoning"
		record.Text = truncateText(cleanText(contentPreview(payload["summary"])), 1000)
	default:
		record.Kind = emptyDefault(payloadType, "response_item")
		record.Text = truncateText(cleanText(contentPreview(payload)), 1000)
	}
}

// demoteLegacyCodexMessages applies the parser's one-source-per-rollout
// rule (docs/design/0004) to a rollout's records: when the file carries
// completed TurnItem messages, the legacy event_msg user_message /
// agent_message events and the raw response_item messages are model
// I/O or duplicates, not the conversation. They stay in the log as
// records — nothing is dropped — but no longer count as prompts or
// replies, so a 0.147 rollout is not double-counted and an injected
// instruction envelope is not shown as something the human typed.
func demoteLegacyCodexMessages(records []Record) bool {
	hasCompleted := false
	for i := range records {
		if records[i].Type == "event_msg:item_completed" && (records[i].Kind == "user_prompt" || records[i].Kind == "assistant_message") {
			hasCompleted = true
			break
		}
	}
	if !hasCompleted {
		return false
	}
	for i := range records {
		r := &records[i]
		switch r.Type {
		case "event_msg:user_message", "event_msg:agent_message":
			r.Kind = "legacy_message"
		case "response_item:message":
			switch r.Kind {
			case "user_prompt":
				r.Kind = "model_input"
			case "assistant_message":
				r.Kind = "model_output"
			}
		}
	}
	return true
}

func relationFor(sessionStart, sessionEnd, scopeStart, scopeEnd time.Time) ScopeRelation {
	return ScopeRelation{
		OverlapsScope:      sessionStart.Before(scopeEnd) && !sessionEnd.Before(scopeStart),
		StartedBeforeScope: sessionStart.Before(scopeStart),
		StartedInScope:     !sessionStart.Before(scopeStart) && sessionStart.Before(scopeEnd),
		EndedInScope:       !sessionEnd.Before(scopeStart) && sessionEnd.Before(scopeEnd),
		EndedAfterScope:    sessionEnd.After(scopeEnd),
		SpansWholeScope:    !sessionStart.After(scopeStart) && !sessionEnd.Before(scopeEnd),
	}
}

func metricsFor(sessions []SessionSlice, recordsReturned, limit int, scopeStart, scopeEnd time.Time) Metrics {
	workspaces := make(map[string]struct{})
	sourceFiles := make(map[string]struct{})
	var m Metrics
	m.Sessions = len(sessions)
	m.RecordsReturned = recordsReturned
	m.Limit = limit
	for _, session := range sessions {
		if session.Workspace != "" {
			workspaces[parser.CanonicalizeWorkspacePath(session.Workspace)] = struct{}{}
		}
		sourceFiles[session.SourceFile] = struct{}{}
		m.Records += session.Records
		if session.Relation.StartedBeforeScope || session.Relation.EndedAfterScope || session.Relation.SpansWholeScope ||
			session.Start.Before(scopeStart) || session.End.After(scopeEnd) {
			m.LongRunningSessions++
		}
		m.UserPrompts += session.Kinds["user_prompt"]
		m.AssistantMessages += session.Kinds["assistant_message"]
		m.ToolCalls += session.Kinds["tool_call"]
		m.ToolResults += session.Kinds["tool_result"]
		m.Reasoning += session.Kinds["reasoning"]
		if strings.Contains(session.SourceFile, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
			m.Sidechains += session.Records
		}
	}
	m.Truncated = m.RecordsReturned < m.Records
	m.Workspaces = len(workspaces)
	m.SourceFiles = len(sourceFiles)
	return m
}

func matchesWorkspace(scan fileScan, workspacePath string) bool {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return true
	}
	want := parser.CanonicalizeWorkspacePath(workspacePath)
	got := parser.CanonicalizeWorkspacePath(scan.Workspace)
	return want != "" && got == want
}

func matchesProject(scan fileScan, projectName string) bool {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return true
	}
	queryCanonical := parser.CanonicalizeWorkspacePath(projectName)
	scanCanonical := parser.CanonicalizeWorkspacePath(scan.Workspace)
	if queryCanonical != "" && scanCanonical == queryCanonical {
		return true
	}
	query := strings.ToLower(projectName)
	return strings.Contains(strings.ToLower(scan.Project), query) ||
		strings.Contains(strings.ToLower(scan.Workspace), query) ||
		strings.Contains(strings.ToLower(scan.SourceFile), query)
}

func claudeWorkspaceForFile(home, filePath string) (string, string) {
	projectsDir := filepath.Join(home, "projects")
	rel, err := filepath.Rel(projectsDir, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	workspace := parser.DecodePath(parts[0])
	return workspace, parser.GetProjectDisplayName(parts[0])
}

func fallbackSessionID(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func projectNameForWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(workspace))
}

func parseTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

func decodeObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	switch value := obj[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func contentHasType(value any, kind string) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if ok && stringField(obj, "type") == kind {
				return true
			}
		}
	case map[string]any:
		return stringField(v, "type") == kind
	}
	return false
}

func contentPreview(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			part := contentPreview(item)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		for _, key := range []string{"text", "message", "output_text", "input_text", "summary", "name", "query", "command", "path"} {
			if s := stringField(v, key); s != "" {
				return s
			}
		}
		// tool_result blocks carry their payload under "content" (a
		// string or nested blocks); without this a Claude tool result
		// previewed as the literal word "tool_result".
		if nested, ok := v["content"]; ok {
			if s := contentPreview(nested); s != "" {
				return s
			}
		}
		if t := stringField(v, "type"); t != "" {
			return t
		}
		encoded, err := json.Marshal(v)
		if err == nil {
			return string(encoded)
		}
	case []string:
		return strings.Join(v, " ")
	}
	return ""
}

func execCommandPreview(payload map[string]any) string {
	var command string
	if values, ok := payload["command"].([]any); ok {
		var parts []string
		for _, value := range values {
			if s, ok := value.(string); ok {
				parts = append(parts, s)
			}
		}
		command = strings.Join(parts, " ")
	}
	if command == "" {
		command = stringField(payload, "command")
	}
	status := statusPreview(payload)
	output := firstNonEmpty(stringField(payload, "aggregated_output"), stringField(payload, "formatted_output"), stringField(payload, "stdout"), stringField(payload, "stderr"))
	return joinNonEmpty(" ", command, status, output)
}

func statusPreview(payload map[string]any) string {
	status := stringField(payload, "status")
	if status != "" {
		return status
	}
	if success, ok := payload["success"].(bool); ok {
		if success {
			return "success"
		}
		return "failed"
	}
	if exitCode, ok := payload["exit_code"].(float64); ok {
		return fmt.Sprintf("exit=%d", int(exitCode))
	}
	return ""
}

func toolCallPreview(payload map[string]any, payloadType string) string {
	name := firstNonEmpty(stringField(payload, "name"), stringField(payload, "tool"), stringField(payload, "status"))
	action := contentPreview(payload["action"])
	return joinNonEmpty(" ", payloadType, name, action)
}

func cleanText(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if r == '\uFFFD' {
			continue
		}
		if r == '\t' || r == '\n' || r == '\r' || unicode.IsSpace(r) {
			if !lastSpace && out.Len() > 0 {
				out.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		out.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(out.String())
}

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	var out strings.Builder
	count := 0
	for _, r := range s {
		if count >= max-3 {
			break
		}
		out.WriteRune(r)
		count++
	}
	return strings.TrimSpace(out.String()) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func joinNonEmpty(sep string, values ...string) string {
	var parts []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, sep)
}
