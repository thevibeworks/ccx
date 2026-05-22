# HTML Report

Specification for `fold.html` — the human review surface for Context
Folding.

## Design principles

- **Self-contained**: inline CSS and JS, no CDN, no external deps.
  Single file that works offline and prints cleanly.
- **Scannable**: decision cards, not prose. The human should grasp
  the session's decisions in under 5 minutes.
- **Spatial**: attention tiers, color-coded provenance badges,
  collapsible excerpts. Leverage visual processing.
- **Audit-first**: every high-attention card must show provenance,
  confidence, citations, and whether it will be archived.

## Output location

Always write to the OS temp directory, never the repo:

```bash
TMPDIR="${TMPDIR:-/tmp}"
FOLD_HTML="$TMPDIR/ccx-context-fold-$(date +%Y%m%d-%H%M%S)-${SESSION_SLUG}.html"
```

After writing, open in the default browser (`open` on macOS,
`xdg-open` on Linux) and print the absolute path to the user.

## Generation method

Build the HTML as a single string using template substitution.
Structure:

1. Write the static HTML skeleton (doctype, head with inline CSS,
   body container, inline JS at the end)
2. For each decision, render a card using the appropriate template
   (high-attention scene card, mid-attention compact row, correction
   warning, discovery callout)
3. Populate the header stats, summary paragraph, evidence gaps, diff
   summary, and metadata footer from the evidence graph data

Do not use external templating libraries. The output is a single
string written to a file. CSS follows ccx's palette:

```css
:root {
  --primary: #da7756;
  --provenance-human: #3b82f6;
  --provenance-agent: #f59e0b;
  --provenance-joint: #8b5cf6;
  --provenance-correction: #ef4444;
  --discovery: #10b981;
}
```

Dark theme default. Light toggle via `data-theme` attribute on
`<html>`. Print styles: expand all `<details>`, black on white.

## Page sections (in order)

1. **Header**: session slug, date, duration, model, exchange count,
   commit count, decision count, warning count, token cost
2. **Evidence gaps**: trace warnings and missing context docs
3. **Summary**: 1 paragraph — what happened, what shipped, what's
   open. Scene style (tension, not changelog).
4. **High-attention decisions**: full cards with scene reconstruction
5. **Corrections**: warning-styled cards
6. **Discoveries**: callout boxes
7. **Mid-attention decisions**: compact expandable rows
8. **Open questions**: the live wires
9. **Not archived**: decisions that failed the three-gate bar
10. **Diff summary**: commits, dirty files, files changed by directory
11. **Metadata**: trace path, tokens, cost, compactions, sidechains, session ID

## Card templates

### High-attention decision

```html
<article class="decision high" id="d-001">
  <header>
    <span class="badge joint">joint</span>
    <span class="badge high">high</span>
    <span class="badge confidence">confidence: high</span>
    <span class="badge">arch</span>
    <h3><!-- decision summary --></h3>
  </header>
  <section class="scene">
    <h4>Tension</h4><p><!-- ... --></p>
    <h4>Decision</h4><p><!-- ... --></p>
    <h4>Rejected</h4><ul><!-- alternatives --></ul>
    <h4>Tradeoff</h4><p><!-- ... --></p>
  </section>
  <details class="excerpt">
    <summary>Conversation (exchanges #N-M)</summary>
    <div class="exchange"><!-- 2-3 key messages --></div>
  </details>
  <footer><!-- citations, linked commit SHAs, archive status --></footer>
</article>
```

### Correction

```html
<article class="decision correction" id="c-001">
  <header>
    <span class="badge correction">correction</span>
    <h3><!-- rule established --></h3>
  </header>
  <section>
    <p><strong>Agent tried:</strong> <!-- what agent did --></p>
    <p><strong>Human directed:</strong> <!-- user's correction --></p>
    <p><strong>Rule:</strong> <!-- the constraint --></p>
    <p><strong>Archive:</strong> <!-- yes/no + gate result --></p>
  </section>
</article>
```

### Discovery

```html
<aside class="discovery" id="v-001">
  <span class="badge discovery">discovery</span>
  <h4><!-- finding --></h4>
  <p><!-- context and implication --></p>
  <cite><!-- evidence citation --></cite>
</aside>
```

### Mid-attention (compact)

```html
<details class="decision mid" id="d-004">
  <summary>
    <span class="badge agent">agent</span>
    <!-- one-line decision summary -->
  </summary>
  <p><!-- reasoning + rejected alternatives --></p>
</details>
```

## Conversation excerpts

Include the 2-3 messages that contain the decision point. For each
message, show role (User/Assistant), timestamp, and the relevant
text (strip tool noise). Wrap in `<details>` — collapsed by default,
expandable on click.

Do not include the full exchange. If the exchange has 15 messages,
show only the decision moment. The user can find the full session
via the session ID in the metadata footer.

## Evidence Gap Template

```html
<aside class="evidence-gap">
  <span class="badge warning">warning</span>
  <h4><!-- warning kind --></h4>
  <p><!-- effect on confidence --></p>
</aside>
```

Warnings are not failures. They are part of the audit trail.
