# Typed Prompt Delivery — Plan

**Status:** Implemented · **Date:** 2026-11-14

Deviations from the plan: the gate runs ahead of `requirePromptIdle` rather than
after it, because that check refuses on `State.Busy()`, which a running subagent
holds; the ledger half of the gate (no main-scope turn, no pending prompt or
move, the text with its move note still typeable) is decided inside the commit
that records the prompt, and the gate, the record and the paste all run in the
runner's turn order. A runner with an attached view is never typed into: that
view is an attach client of an api session, which the api path delivers to.
`skip_prefixes` is judged on the dispatch text, after the sigil guard, so a
message opening with an attachment is typed escaped. Fixing the black-box test
surfaced a terminal bug: a fresh PTY's first foreground sample cleared the modes
its app set in its first output (bracketed paste among them); a fresh birth now
latches its own process group, and only a restore keeps that startup reset.

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
      skip_prefixes: ["/", "!"] # messages that must still restart (see §8)
      composer:                 # how the empty input box looks
        row_prefix: "❯\u00a0"   # glyph + NO-BREAK space; a plain space never matches
```

"Composer empty" is generic: the screen row holding the cursor starts with
`row_prefix`, the cursor column is exactly the prefix's width, and every cell
after it is blank or drawn as `composer.ghost` (`faint`: SGR 2). The cursor
column alone is not enough: Home on a draft puts the cursor where an empty box
has it, with the (non-faint) draft after it. No Go code names a provider.

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
5. The message does not start with one of the block's `skip_prefixes`.

The composer test alone cannot tell idle from busy (§8): it reads empty
mid-turn too. Idleness is the ledger's call, and it must be "no main-scope turn
is running", not `State.Busy()`: background subagents and shells outliving the
turn are exactly what typing exists to keep, so they must not block it.

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

## 8. Spike results (claude 2.1.325, through the daemon's own emulator)

1. **Composer.** Idle, ghost suggestion, and mid-turn all read cursor col 2 on
   the row `❯\u00a0…`; a draft puts the cursor past col 2 (`hello` → col 7); a
   dialog or shell mode moves the cursor off that row. Same at 80x24 and
   120x40. Mode 2004 was on in every state. Ghost text is faint and starts at
   col 2. Re-recorded with cell attributes (`.faint` sidecars): the fresh
   placeholder and the repeated-prompt suggestion are faint; `hello` then Home
   (Ctrl-A) reads col 2 with a non-faint draft after it.
2. **Submit.** Paste plus `\r` in one write submitted 3 of 3; two back-to-back
   writes also 3 of 3. No wait signal is needed.
3. **Sigils.** Under paste a leading `!` still enters shell mode and a leading
   `/` still runs the command. So `leading_sigils` stays for the restart path
   and those messages are kept off typed delivery by `skip_prefixes`.
4. **Multi-line.** A paste with an embedded newline is one draft, and `\r`
   sends one message.
5. **Busy.** The composer reads empty mid-turn too, so only the ledger guards
   against typing into a running turn.

Recordings: `api/internal/engine/agents/testdata/claude-composer-*-2.1.325.screen`
with a `.cursor` sidecar each. The trust dialog could not be re-recorded on this
account; the 2.1.319 recording stays the only evidence for it.

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

## 10. Known limits

- An idle notification dispatched just before the paste's `UserPromptSubmit`
  settles the typed prompt `not_acknowledged`, and the turn then opens as a
  provider prompt (the text shows twice). Telling the two orders apart needs a
  timestamp the CLI does not send; the alternative leaves a prompt the CLI
  never took unsettled, which blocks the chat until Stop.
- A keystroke from an attached pane that opens a turn before the paste is
  submitted also settles the typed prompt `not_acknowledged`.
- Only `/` and `!` were recorded as sigils that change meaning under paste.

## 11. Out of scope

Model, effort and permission changes keep `restart_tui`. Codex and Hermes are
unchanged; the block is generic, so they can declare it later.
