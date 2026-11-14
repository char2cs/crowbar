# Typed Prompt Delivery — Plan

**Status:** Proposed · **Date:** 2026-11-14

## 1. The problem

A prompt to a hooks-channel TUI (claude) is delivered by killing the CLI and
relaunching it with the message as argv (`presentation.prompt_submit.strategy:
restart_tui`). The restart kills what the agent was running: subagents,
background shells, Monitors. Mid-turn prompts already avoid it (the Stop-hook
steer). Prompts to an idle runner do not.

The restart is a delivery choice, not something the hook protocol forces. Hooks
stay the way Crowbar *listens*; this plan changes how an idle prompt *goes in*.

## 2. Decision

When the chat's live CLI is idle, its descriptor declares `prompt_submit.type`,
and the terminal shows an empty composer with bracketed paste on, Crowbar writes
the prompt into the CLI's PTY as a bracketed paste followed by the submit key.
In every other case the existing restart path runs unchanged. That choice is made
before anything is sent, so it is a decision and not a retry.

`-p` is not used: it has no TUI (no hotswap terminal view) and a billing
position Anthropic may change.

## 3. Descriptor

`strategy` stays `restart_tui`: it is the launch path and the fallback. A new
optional block sits beside it:

```yaml
presentation:
  prompt_submit:
    strategy: restart_tui
    type:
      paste: bracketed          # message wrapped in ESC[200~ … ESC[201~
      submit: "\r"              # key that sends it
      composer:                 # how the empty input box looks (values from the spike)
        row_prefix: "…"
```

"Composer empty" is generic: the screen row holding the cursor starts with
`row_prefix` and the cursor column is exactly the prefix's width. Ghost-text
suggestions paint after the cursor and a draft pushes the cursor past it, so the
cursor column, not the row's text, is the test. No Go code names a provider.

The rule (`rules/prompt_submit.go`) validates the block: `paste` is a known
value, `submit` and `row_prefix` are non-empty. `type` requires `strategy:
restart_tui` (the fallback must exist) and a declared `steer`-independent
`session.resume`, as today.

## 4. The gate

One function in the runner decides, between the steer branch and the API branch
of `submitPromptLocked`. A prompt is typed only when all hold:

1. The descriptor declares `type`, over the hooks channel (not an API connection).
2. `requirePromptIdle` passes and no other prompt is pending (the ledger is the
   single owner of idle; the check runs under `live.inTurnOrder`, so no turn can
   open meanwhile).
3. `selectionRequiresRestart` is false (a changed model, effort or permission
   level still restarts).
4. The runner has a live PTY terminal session, and its snapshot reports bracketed
   paste on and an empty composer.
5. The message is not one the descriptor keeps off typed delivery (a
   `skip_prefixes` command, as for steer).

Otherwise `resolvePromptDelivery` / `launchCarrying` run as before.

## 5. Delivery and receipt

- `recordPrompt(..., ledger.DeliveryType, live.ID, ...)` commits the prompt
  `pending` **before** the write, with the leading-sigil guard and attachment
  materialisation applied as the restart path does, and the move note registered
  through `RecordInjection` as the API branch does.
- The write is one paste plus the submit key to `live.TerminalSession`. A write
  error settles the prompt `delivery_failed` (`failPrompt`).
- The receipt is the `UserPromptSubmit` hook. Ingress already matches the oldest
  pending prompt whose text and carrier runner equal the hook's, and flips it to
  `delivered`. A prompt the CLI never takes settles `not_acknowledged` on idle
  (`SettleUntaken`) because its delivery is not `steer`. Nothing new is polled.
- Retract: a typed prompt is with the CLI the moment it is written, so
  `cliHolds` reports it held and it cannot be recalled.

## 6. Terminal read

The terminal engine already exposes `Screen`. Missing: the cursor position and
bracketed-paste mode. Add one read that returns, under the session lock, the
visible text, cursor row and column, and whether mode 2004 is set — as an
optional interface on the model (precedent: `model.ThemeAware`), a `Session`
method, and an `Engine` method. The runner reaches it through an optional
interface assertion on `rs.term`, as `termwait` does for `Screen`, so the seam
fakes are not widened.

## 7. Client

`delivery` on a user-message row gains the new value. A typed prompt folds like
any pending prompt (queued until its turn starts); only `restart` carries the
launch divider.

## 8. What the spike must settle before values land in `claude.yaml`

Measured against a real claude with a recorded screen, never assumed:

1. What the empty composer looks like, with and without a ghost suggestion, with
   a draft, mid-turn, and under a dialog; where the cursor sits in each.
2. Whether a paste followed by the submit key in **one** write submits, or the
   CLI needs the paste to land first. If it does, the signal to wait on is the
   screen changing, never a timer.
3. Whether a pasted leading `!` or `/` changes mode, and whether `leading_sigils`
   still has to escape it.
4. Whether a multi-line prompt arrives as one message.

Each result becomes a recorded screen under `engine/agents/testdata/` (per
claude version, like `claude-trust-2.1.319.screen`) and a descriptor value, so a
TUI change breaks a test instead of users.

## 9. Tests

- `rules`: accepts a well-formed `type` block, rejects each broken shape.
- Gate (runner internal tests, fake commander): typed when all hold; restart
  when busy, pending, selection changed, no PTY, paste off, composer holding a
  draft, command text.
- Black-box `TestRegression_*` against the fake provider (`fakecli` made a
  composer: prints `ESC[?2004h`, reads a paste, awaits the submit key): an idle
  prompt does not relaunch the CLI (same PID, same launch), the PTY receives the
  paste, the `UserPromptSubmit` hook flips it `delivered`; a draft in the box
  falls back to a restart.
- Live: a subagent started in turn one survives the prompt of turn two.

## 10. Out of scope

Model, effort and permission changes keep `restart_tui`. Codex and Hermes are
unchanged; the block is generic, so they can declare it later.
