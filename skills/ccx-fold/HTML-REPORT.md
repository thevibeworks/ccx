# HTML Report

Specification for fold.html — the human review surface.

## Design principles

- **Self-contained**: inline CSS and JS, no CDN, no external deps.
  Single file that works offline and prints cleanly.
- **Scannable**: decision cards, not prose. The human should grasp
  the session's decisions in under 5 minutes.
- **Spatial**: attention tiers, color-coded provenance badges,
  collapsible excerpts. Leverage visual processing.

## Output location

Always write to the OS temp directory, never the repo:

```bash
TMPDIR="${TMPDIR:-/tmp}"
FOLD_HTML="$TMPDIR/ccx-fold-$(date +%Y%m%d-%H%M%S)-${SESSION_SLUG}.html"
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
3. Populate the header stats, summary paragraph, diff summary, and
   metadata footer from the evidence graph data

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

1. **Header**: session slug, date, duration, model, commit count,
   decision count, token cost
2. **Summary**: 1 paragraph — what happened, what shipped, what's
   open. Scene style (tension, not changelog).
3. **High-attention decisions**: full cards with scene reconstruction
4. **Corrections**: warning-styled cards
5. **Discoveries**: callout boxes
6. **Mid-attention decisions**: compact expandable rows
7. **Open questions**: the live wires
8. **Low-attention**: collapsed `<details>` list
9. **Diff summary**: files changed by directory, lines +/-
10. **Metadata**: tokens, cost, compactions, sidechains, session ID

## Card templates

### High-attention decision

```html
<article class="decision high" id="d-001">
  <header>
    <span class="badge joint">joint</span>
    <span class="badge high">high</span>
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
  <footer><!-- linked commit SHAs --></footer>
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
