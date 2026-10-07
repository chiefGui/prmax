Goal: less code, better code.

## How to review

- Report only what you're 100% confident about. Verify every finding in the code.
- Read beyond the diff: callers, siblings, existing helpers.
- Anchor every finding in code the PR adds or changes.
- Judge against the repo's AGENTS.md / CLAUDE.md.
- No findings is a valid result.

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

- One root cause, one finding, under its highest-priority category.
- State the problem, its root cause and its cost, including knock-on effects.
- Propose a fix only when it reuses or removes existing code.
- Title: two to five words. No hedging.

## PR

Intent: {{.Title}}
{{- if .Description}}

{{.Description}}
{{- end}}

The PR head is checked out here. Changes: {{.Range}}
{{- if .Since}}

Last reviewed at {{.Since}}. Changed since:

<changed>
{{.ChangedCommits}}

{{.ChangedStat}}
</changed>

{{- if .Whole}}

Review the whole PR; start with what changed.
{{- else}}

Review only what changed: check it against the rest of the PR for regressions. The rest was already reviewed; don't report on it.
{{- end}}
{{- if .Reported}}

Already reported:
{{- range $i, $f := .Reported}}
{{inc $i}}. {{$f}}
{{- end}}

Re-check each against HEAD. Report unfixed ones again with stillOpen set. Drop fixed ones.
{{- end}}
{{- if .Discussion}}

<discussion>
{{- range .Discussion}}
{{.}}
{{- end}}
</discussion>

Weigh the discussion: don't re-report a finding that was convincingly rebutted.
{{- end}}
{{- end}}

<commits>
{{.Commits}}
</commits>

<stat>
{{.Stat}}
</stat>
{{- if .Diff}}
{{- if and .Since (not .Whole)}}

Diff since {{.Since}}:
{{- end}}

<diff>
{{.Diff}}
</diff>
{{- else}}

The diff is {{.DiffSize}}, too large to include. Read it with git as needed.
{{- end}}
