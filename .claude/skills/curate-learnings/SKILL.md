---
name: curate-learnings
description: Use when the user asks to "curate learnings", "promote learnings", "process pending sessions", "review session captures", or invokes /curate-learnings. Distills pending session transcripts (captured by the SessionEnd hook into .claude/kb_source/_pending/) into curated entries in docs-internal/knowledge/learnings.md, with user approval per entry.
---

# Curate session learnings

Process pending session transcripts captured by `.claude/hooks/capture-session.sh` and promote durable, non-obvious learnings into `docs-internal/knowledge/learnings.md`.

## Inputs

- `.claude/kb_source/_pending/*.jsonl` — captured session transcripts (one per non-trivial session)
- `.claude/kb_source/_pending/*.meta.json` — sidecar metadata (session_id, timestamp, tool_use_count, reason)

If `_pending/` is empty or missing, tell the user "no pending sessions" and exit.

## Workflow

### Step 1 — Inventory
List the pending transcripts oldest-first. Show count, total size, oldest/newest timestamps. Confirm with the user before proceeding ("found N pending sessions from <date> to <date>, proceed?").

### Step 2 — Distill (one transcript at a time)
For each transcript:

1. Read the JSONL. Walk user messages, assistant text, and tool_use/tool_result pairs in order.
2. Read `docs-internal/knowledge/learnings.md` (if it exists) and `CLAUDE.md` so you can recognize what's already documented.
3. Extract candidate learnings using these rules:
   - **Keep only durable, non-obvious facts about THIS codebase** — gotchas hit, commands that worked unexpectedly, conventions discovered, hook/tool friction.
   - **Discard everything else** — task summaries, conversational turns, things Claude could re-derive from the code, anything restating CLAUDE.md or existing docs-internal/knowledge content.
   - **Cap at 3 entries per session.** If a session legitimately produced more, pick the highest-signal 3 and tell the user the rest were dropped for selectivity.
   - **Skip entirely if nothing is worth keeping.** Most sessions produce zero entries. That's the correct outcome — do not invent learnings to justify the run.

### Step 3 — Present and approve
For each candidate entry, show the user:
- A 1–2 line summary in the target format (see below)
- The session id + timestamp it came from
- A brief reason ("why this is durable / non-obvious")

Use `AskUserQuestion` with options: **keep / edit / drop**. For "edit", let the user supply a replacement line. Never silently keep — every entry needs an explicit accept.

### Step 4 — Append accepted entries
Append accepted entries to `docs-internal/knowledge/learnings.md` under a date heading (YYYY-MM-DD, UTC). Format:

```markdown
## 2026-05-27

- **<short headline in bold>** — one-sentence body. (Session: <session_id>)
```

Group multiple entries from the same day under one heading. If the date heading already exists, append under it; do not create duplicates.

If `learnings.md` doesn't exist, create it with a single H1 (`# Curated session learnings`) and a one-line description, then the date heading.

### Step 5 — Prune processed transcripts
For each transcript fully processed (regardless of whether anything was kept), delete the `.jsonl` and `.meta.json` from `_pending/`. Tell the user the count of deleted files at the end.

### Step 6 — Wrap-up summary
Report:
- N sessions processed
- M entries added to `learnings.md`
- K transcripts pruned
- Pointer to `learnings.md` so the user can review the file

## Output discipline

- **Be ruthlessly selective.** A session that produces zero kept entries is a *success* — it means the system isn't generating spam.
- **Never paraphrase existing CLAUDE.md or docs-internal/knowledge content.** If the candidate learning is already documented elsewhere, drop it and tell the user "already in CLAUDE.md" as the reason.
- **Cite the session id** in every entry so future-you can trace back if needed.
- **Prefer commands and file paths over prose.** "`make rpmpkg-ual` needs PATH+GOPATH exported" beats "remember to set environment variables when running the RPM build".

## When NOT to invoke this skill

- If the user is asking to capture a learning right now from the current conversation — that's a memory/CLAUDE.md edit, not this skill.
- If `_pending/` is empty.
- If the user wants to write a brand-new KB article from scratch — use direct file editing instead.

## Maintenance

If `learnings.md` grows past ~200 lines, suggest distilling its contents into the proper topical files under `docs-internal/knowledge/dcm/` (`architecture.md`, `gpu-manager.md`, etc.) and archiving the old entries. This skill does not auto-do that — it's an explicit user decision.
