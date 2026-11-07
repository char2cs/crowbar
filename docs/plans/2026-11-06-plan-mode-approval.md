# Plan mode: answering a plan approval from the chat

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

## Next

3. Mode after approval. The TUI offers "Yes, and use auto mode" and "Yes, manually
   approve edits". The hook can say this with `updatedPermissions: [{type: setMode,
   mode, destination: session}]`. The descriptor would offer them as declared literal
   suggestions on the ExitPlanMode ask (same shape as `suggestion_label.*`), since claude
   offers none of its own.
4. Entering plan mode. `PermissionLevels()` is a closed set ordered by how permissive
   each level is, and plan is not on that axis. It belongs beside the level as a
   per-chat mode that the descriptor maps to `--permission-mode plan`, with the
   composer showing it. Shift+Tab inside the TUI changes it without Crowbar knowing:
   the PermissionRequest `permission_mode` field is the CLI's own report, and the label
   should follow it (this is also the fix for the "Allow edits for the rest of this
   session" label).
5. A TUI dialog while busy. termwait returns nothing while a turn runs, so a dialog the
   hook does not cover (the plan dialog before this fix) shows no banner. That needs a
   decision on whether a matched kinded prompt can override busy.
