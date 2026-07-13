# Phase-0 spike — agentic engine (2026-07-05)

Throwaway PTY harness that proved every load-bearing claim of
[`../2026-07-05-crowbar-agentic-engine-design.md`](../2026-07-05-crowbar-agentic-engine-design.md)
live against the real `claude` (2.1.201) and `codex` (0.139.0) binaries, **before any engine code**.
Retained as executable reference for the integration-test authors — this is *how* to drive the
CLIs, not production code.

## What each script proves

| Script | Proves |
|---|---|
| `drive.py` | Claude: spawn in a PTY, accept trust, `/clear` → SessionStart re-fires with a **new** session id (detection). |
| `drive_codex.py` | Codex: relocated `CODEX_HOME` + `--dangerously-bypass-hook-trust`; hooks fire interactively; `/new` → new session id. |
| `sum.py` | Renders a captured `hooks.log` as a legible `event / source / session_id` sequence. |
| `probe_persist2.py` | Claude persists its transcript incrementally **once `CLAUDE_CODE_CHILD_SESSION`/`CLAUDECODE` are cleared** from the child env. |
| `orchestrator.py` | Full **Claude→Codex→Claude** switch: opaque cross-provider handoff, native `--resume`, appended delta lands, and the delta does **not** pollute the vendor session file. |
| `capture.sh` | The hook command: appends each hook's stdin payload to `hooks.log`, labelled + timestamped. |
| `settings.json`, `codex_home/` | The injected hook configs (Claude `--settings` / Codex `CODEX_HOME/hooks.json`). |

## Caveats (reference only — will not run as-is)

- **Hardcoded absolute paths** point at the spike scratchpad; adjust `SPIKE` / `HOOKS_LOG` before reuse.
- **`codex_home/auth.json` and `models_cache.json` are intentionally omitted** (the auth file is a
  live credential). The real harness copied them from `~/.codex`.
- Runs from *inside* Claude Code, hence the env-clearing dance (`orchestrator.py`, `probe_persist2.py`).
  Crowbar's Go daemon spawns with a clean env and does not need it — but the descriptor clears the
  markers defensively anyway.

## Key findings folded into the spec

1. Transcript path is **read from the hook**, never computed (Claude slug truncated+hashed; Codex date-partitioned rollout).
2. Detection branches on the **id-change fact**, not the `source` label — the labels disagree across CLIs (`clear` vs `startup`).
3. Codex hooks fire **interactively only** (never under `codex exec`).
4. Switch = **gracefully quit** the outgoing CLI (Claude flushes on clean exit; Codex tolerates a hard kill).
5. `--append-system-prompt` on `--resume` is **per-invocation** → non-polluting switch-back.
