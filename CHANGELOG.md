# Changelog

All notable changes to ccx are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [Unreleased]

## [0.2.1] - 2026-01-04

### Changed
- README rewritten with web UI as primary feature, ASCII diagram
- CLI help (`ccx --help`, `ccx web --help`) emphasizes web UI

### Added
- Site footer with GitHub link and thevibeworks branding

## [0.2.0] - 2026-01-04

### Added
- **Session search**: In-session search with floating search bar, keyboard shortcuts (`/`, `f`, `Esc`), navigation (`Enter`, `Shift+Enter`), and filter chips (User, Response, Tools, Agents, Thinking)
- **Tool rendering**: Specialized preview/output formatting for Task, Skill, WebSearch, WebFetch, AskUserQuestion, LSP, TaskOutput, KillShell
- **Refresh button**: Toolbar refresh button (`r` shortcut) for manual page reload
- **Auto-expand on search**: Automatically unfolds collapsed sections when jumping to search matches

### Security
- URL sanitization: Only allow http/https URLs in rendered output
- Tabnabbing prevention: Added `rel="noopener noreferrer"` to all external links
- Deterministic preview: Fixed nondeterministic map iteration in tool parameter preview

## [0.1.1] - 2025-12-31

### Changed
- **Pure Go migration**: Replaced mattn/go-sqlite3 (CGO) with modernc.org/sqlite (pure Go) for true cross-platform single-binary distribution
- Updated Go 1.22 → 1.24.0, cobra 1.8 → 1.10.2, viper 1.18 → 1.21.0

### Added
- GitHub Actions CI workflow (test, lint on push/PR)
- GitHub Actions Release workflow (cross-compile darwin/linux arm64/amd64)
- CONTRIBUTING.md with dev setup and release workflow
- Skill bundle packaging (ccx.skill)

### Fixed
- Cobra duplicate error output
- Dead code removal
- Error handling for json.Encode, file.Seek

## [0.1.0] - 2025-12-30

### Added
- Core CLI commands: projects, sessions, view, export, search, config, doctor
- JSONL parser with tree-aware message structure (parentUuid, isCompactSummary, isSidechain)
- Web UI with project/session browser, collapsible blocks, syntax highlighting
- Export formats: HTML, MD, Org-mode, JSON
- Realtime watch mode via Server-Sent Events
- SQLite star/favorite system
- Global search across projects and sessions
- Dark/light theme toggle with persistence
- Keyboard navigation (j/k scroll, gg/G jump, / search, t theme, z collapse)

[Unreleased]: https://github.com/thevibeworks/ccx/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/thevibeworks/ccx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/thevibeworks/ccx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/thevibeworks/ccx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/thevibeworks/ccx/releases/tag/v0.1.0
