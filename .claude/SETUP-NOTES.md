# Claude Code Setup Notes

Reference for how this repo's Claude Code config is structured and why. Based on the [Claude Code best practices guide](https://claude.com/blog/how-claude-code-works-in-large-codebases-best-practices-and-where-to-start).

## What lives where

| Path | Purpose | Committed? |
|---|---|---|
| `CLAUDE.md` | Workflow rules + non-obvious gotchas (~47 lines, lean by design) | yes |
| `.claude/settings.json` | Team-shared permission allowlist + hook wiring | yes |
| `.claude/settings.local.json` | Personal/machine allowlist overrides | **no** (gitignored) |
| `.claude/hooks/protect-generated.sh` | PreToolUse: blocks edits to generated/vendored/asset files | yes |
| `.claude/hooks/gofmt-on-edit.sh` | PostToolUse: runs `gofmt -w` on every `.go` edit | yes |
| `.claude/hooks/capture-session.sh` | SessionEnd: captures non-trivial session transcripts | yes |
| `.claude/skills/curate-learnings/SKILL.md` | Skill: distills session transcripts into learnings | yes |
| `docs-internal/knowledge/dcm/` | Deep architecture / component docs (loaded on demand, NOT every session) | yes |
| `docs-internal/knowledge/learnings.md` | Curated learnings from session transcripts | yes |
| `.claude/kb_source/_pending/` | Raw session transcripts awaiting curation | **no** (gitignored) |
| `.claude/specs/` | Design specs and implementation plans | yes |
| `.ignore` | Ripgrep exclusions (vendor, assets, gen) | yes |
| `CLAUDE.local.md` | Personal project notes (if you create one) | **no** (gitignored) |

## Design rules we chose

1. **CLAUDE.md stays small.** Anything Claude can derive from code, the README, or a kb_source file does NOT go here. Only non-guessable bash commands, gotchas, and "don't touch" rules.
2. **Architecture/components live in `docs-internal/knowledge/dcm/`, not CLAUDE.md.** Loaded on demand by Claude when actually needed; doesn't burn context every session.
3. **`settings.json` is team-shared, `settings.local.json` is personal.** The shared file only allowlists read-only or local-only operations (no `git push`, no `gh pr create`, no `docker run`).
4. **Hooks are deterministic guardrails, not advice.** Rules that MUST happen every time (gofmt, blocking generated-code edits) go in hooks, not CLAUDE.md.
5. **Containerized builds only for Claude.** CLAUDE.md steers toward `make docker-shell` because CGo links against AMD SMI/DRM libs with hardcoded paths that don't exist on the host.
6. **Skills use `<name>/SKILL.md` naming.** Auto-discovery works either casing but consistency makes the listing readable.

## Permission allowlist (shared)

In `.claude/settings.json`. Read-only: git status/diff/log/show/branch/stash list/ls-files/blame; gh pr view/list/diff/checks, gh issue view/list, gh api, gh run view/list; docker ps/images/logs/inspect. Go toolchain (test/vet/build/mod tidy/mod download, gofmt, goimports, golangci-lint). Project: `Bash(make:*)` (broad — all targets are in-repo Makefile).

**NOT allowlisted (still prompts):** `git commit/push/checkout/reset`, `gh pr create/merge/close`, `docker run/build/rm`, anything that hits lab hardware.

## Hook coverage

- **Block edits to:** `*.pb.go`, `gen/**`, `vendor/**`, `assets/**`, `assets6.3/**`. The hook only intercepts Write/Edit tool calls — shell commands (Bash) are not blocked, so asset refresh workflows via make/cp/tar work fine.
- **Auto-format:** `gofmt -w` on every `.go` edit. Skips silently if `gofmt` not on PATH (e.g., inside a container without Go).
- **Session-end capture:** non-trivial sessions (>=5 tool uses) get their JSONL transcript copied to `.claude/kb_source/_pending/` (gitignored) for later review. Run `/curate-learnings` to distill durable, non-obvious findings into `docs-internal/knowledge/learnings.md`. Most sessions produce zero kept entries — that's correct.

## Maintenance checklist (when adding stuff later)

- New skill → `.claude/skills/<name>/SKILL.md`. Use a `description` that starts with "Use when..." for reliable auto-dispatch.
- New non-obvious gotcha → add to `CLAUDE.md` (~2 lines). If it's >5 lines, put it in `docs-internal/knowledge/dcm/` and link from CLAUDE.md.
- New tool/CLI you want auto-approved → add to `.claude/settings.json` `permissions.allow`. Keep entries scoped (`Bash(tool subcommand:*)` not `Bash(tool *)`).
- New deterministic rule ("always do X after Y") → write a hook in `.claude/hooks/`, register it in `settings.json`, test it with synthetic input.
- New search exclusion → add to `.ignore` at repo root.

## What we deliberately did NOT do

- No subdirectory CLAUDE.md files — 16 Go files don't warrant per-directory guidance.
- No `/build` skill yet — deferred to a future session. Will orchestrate containerized build flow.
- No `goimports` hook — it can fight the build-container's vendored toolchain on import order. Plain `gofmt` is safer.
- No `go vet` PostToolUse hook — too slow, too noisy, runs per-edit. Belongs in pre-commit or CI.
- No `SessionStart` git-status print — Claude can ask for it on demand, no need to burn context every session.
- No WebFetch domain allowlists in shared settings — add per personal need in `settings.local.json`.
