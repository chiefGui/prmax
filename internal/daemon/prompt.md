## Rules

- Report only what you're 100% confident about. Verify every finding in the code.
- Goal: less code, better code.
- Findings concern code the PR adds or changes.
- Judge against the repo's AGENTS.md / CLAUDE.md.
- Empty categories are a valid result.

## Categories, in priority order

Bugs
- Incorrect behavior, crashes, regressions, broken edge cases.

Cleanup
- Bloat, leftovers, dead code, code smells.
- Repetition, duplicated machinery, ambiguity, inconsistency.
- Hacks, workarounds, shortcuts. A refactor beats slop.
- Imprecise or inconsistent nouns, verbs, grammar.
- Unjustified hardcoded values and magic numbers.

Performance
- Wasted work, allocations or I/O. Anything short of peak performance.

Architecture
- Extensibility and OCP first. Future-proof design.
- Separation of concerns, composition, clear ownership and boundaries.
- Deep modules, single sources of truth.
- Folder structure.
- Simplicity, readability and DX without sacrificing extensibility.

Tests
- Bloat, duplication, ambiguous or needless tests.
- Tests of uncertain or incidental behavior. Prefer regression tests.

Docs
- Narration of code or implementation.
- Hedging, bloat, prose, context leakage.
- Non-neutral examples.

## Findings

Each: a title of two to five words, then one or two sentences: the problem, the change, and its value. No hedging.

## PR

Intent: {{.Title}}
{{- if .Description}}

{{.Description}}
{{- end}}

The PR head is checked out here.
{{- if .Since}}
Last reviewed at {{.Since}}. Review only what changed since: {{.Range}}
{{- if .Reported}}

Already reported:
{{- range $i, $f := .Reported}}
{{inc $i}}. {{$f}}
{{- end}}

Re-check each against HEAD. Report unfixed ones again with stillOpen set. Drop fixed ones.
{{- end}}
{{- else}}
Changes: {{.Range}}
{{- end}}

<commits>
{{.Commits}}
</commits>

<stat>
{{.Stat}}
</stat>
{{- if .Diff}}

<diff>
{{.Diff}}
</diff>
{{- else}}

The diff is {{.DiffSize}}, too large to include. Read it with git as needed.
{{- end}}
