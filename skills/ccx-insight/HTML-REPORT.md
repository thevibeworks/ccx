# ccx-insight HTML Report

Write one self-contained HTML file. Keep it readable without JavaScript.

Use this structure:

1. Header
   - Title: `Session Intelligence`
   - Scope, timezone, project scope, generated time
   - Evidence command
2. TL;DR
   - One paragraph
   - Include confidence: high, medium, low
3. Metric band
   - Records matched / returned
   - Sessions
   - Long-running sessions
   - Workspaces
   - Tool results
4. Workstreams
   - Group by workspace/project
   - For each: what happened, claim status, evidence refs
5. Timeline
   - Important timestamped records
   - Show local scoped time
6. Claim ledger
   - Columns: claim, status, confidence, evidence
   - Status values: observed, agent-claimed, human-stated, inferred,
     unverified, contradicted
7. Needs closure
   - Open loops with exact evidence refs
8. Caveats
   - Truncated records, missing logs, skipped records, long-running sessions,
     ambiguous session boundaries

Citation format:

```text
<provider>:<session-prefix> <timestamp> <source-file>:<line>
```

Design notes:

- Use a quiet editorial layout like `reference/html-effectiveness/11-status-report.html`.
- Use a dark TL;DR band and timeline rhythm like `12-incident-report.html`.
- Use compact tables for evidence. Avoid giant transcript blocks.
- Cards are acceptable for repeated claims only; do not put cards inside cards.
- Do not use gradients, decorative blobs, or marketing hero layouts.
