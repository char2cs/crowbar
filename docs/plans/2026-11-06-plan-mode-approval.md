# Plan mode: answering a plan approval from the chat

Status: steps 1–3 and 5 done; step 4 (entering plan mode) is open.

## Measured (claude 2.1.319, `--permission-mode plan`, a `--settings` PermissionRequest hook)

- ExitPlanMode reaches the PermissionRequest hook with `tool_input.plan` and
  `tool_input.planFilePath`, and no `permission_suggestions`
  (fixture `PermissionRequest.ExitPlanMode.json`).
- A bare `{"behavior":"allow"}` leaves the TUI's own "Would you like to proceed?"
  dialog open: the chat hangs, busy, with no banner.
- `allow` + `updatedInput` (the input echoed) approves the plan: "User approved
  Claude's plan", then the edits run.
- `deny` + `message` hands the message to claude as the tool's error ("Error: <message>",
  "Denied by PermissionRequest hook"), and it keeps planning. That is the TUI's option 3.

## Done

1. The descriptor's `allow` reply echoes `{tool_input_json}`, so Allow on the card
   approves the plan (for every other tool the echo is the same input, so nothing changes).
2. The card shows the plan text (`requestView` reads `tool_input.plan`).
3. Mode after approval: `events.permission.tool_suggestions.ExitPlanMode` declares the
   dialog's "Yes, and use auto mode" / "Yes, manually approve edits" as `setMode`
   grants, and the `suggestion` reply echoes the input as well. Measured on 2.1.319:
   auto ran the Write unasked; manual asked for it.
5. A dialog over a running turn shows the terminal-wait banner unless a pending choice
   already covers it (termwait `evaluate`).
- Entering plan mode from the composer works today through the `/plan` built-in
  (measured: `claude -- "/plan <task>"` plans, then asks through ExitPlanMode).

## Next

4. Entering plan mode. `PermissionLevels()` is a closed set ordered by how permissive
   each level is, and plan is not on that axis. It belongs beside the level as a
   per-chat mode that the descriptor maps to `--permission-mode plan`, with the
   composer showing it. Shift+Tab inside the TUI changes it without Crowbar knowing:
   the PermissionRequest `permission_mode` field is the CLI's own report, and the label
   should follow it (this is also the fix for the "Allow edits for the rest of this
   session" label).

### Step 4, measured and decided (2026-11-07)

- Every claude hook but SessionStart carries `permission_mode` (fixtures: default,
  acceptEdits, auto, plan, bypassPermissions). It is the one source for the label.
- claude restarts on every message (`prompt_submit.strategy: restart_tui`), so a
  `setMode … destination: session` lasts until the next message: the next launch
  passes the level's `--permission-mode` again. The label must therefore show the
  reported mode only for the turn that reported it, and the level's mode otherwise.
- A plan chip needs the person's request ("plan the next message") to outlive the
  launch it applies to, which is a second piece of per-chat state beside the level
  (both render the same `--permission-mode` flag). Until that is decided, `/plan` is
  the entry, and nothing stores a mode.
- Proposed shape: `ledger` folds the reported mode from the turn's rows (TurnStarted /
  TurnEnded gain `mode`, as TurnEnded already carries `effort`); the composer shows the
  reported mode of a running turn; a `modes:` descriptor block maps a requested mode to
  the apply step that replaces the level's for one launch.
