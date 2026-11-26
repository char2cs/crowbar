# Typed Prompt Delivery — Plan

**Status:** Implemented · **Date:** 2026-11-14

Deviations from the plan: the gate runs ahead of `requirePromptIdle` rather than
after it, because that check refuses on `State.Busy()`, which a running subagent
holds; the ledger half of the gate (no main-scope turn, no pending prompt or
move, the text with its move note still typeable) is decided inside the commit
that records the prompt, and the gate, the record and the typing all run in the
runner's turn order. A runner with an attached view is never typed into: that
view is an attach client of an api session, which the api path delivers to.
`skip_prefixes` is judged on the dispatch text, after the sigil guard, so a
message opening with an attachment is typed escaped. Fixing the black-box test
surfaced a terminal bug: a fresh PTY's first foreground sample cleared the modes
its app set in its first output (bracketed paste mode among them); a fresh birth now
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
and the terminal shows an empty composer, Crowbar types the prompt into the
CLI's PTY as keys, in small chunks, then sends the submit key as a write of its own.
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
      input: keys               # typed as keystrokes, never a bracketed paste
      chunk_bytes: 256          # at most this many bytes per write, cut on a rune
      chunk_interval_ms: 10     # pause between writes, so the CLI reads keys not a paste
      submit: "\r"              # key that sends it, written alone after the chunks
      skip_prefixes: ["/", "!"] # messages that must still restart (see §8)
      skip_suffixes: ["\\"]     # a trailing backslash makes the submit a line break
      composer:                 # how the empty input box looks
        row_prefix: "❯\u00a0"   # glyph + NO-BREAK space; a plain space never matches
```

"Composer empty" is generic: the screen row holding the cursor starts with
`row_prefix`, the cursor column is exactly the prefix's width, and every cell
after it is blank or drawn as `composer.ghost` (`faint`: SGR 2). The cursor
column alone is not enough: Home on a draft puts the cursor where an empty box
has it, with the (non-faint) draft after it. No Go code names a provider.

The rule (`rules/prompt_submit.go`) validates the block: `input` is a known
value, `chunk_bytes` is large enough for a rune, `submit` and `row_prefix` are non-empty. `type` requires `strategy:
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
4. The runner has a live PTY terminal session, and its snapshot reports an empty
   composer (bracketed paste mode on is how the snapshot shows the TUI is up).
5. The message does not start with one of the block's `skip_prefixes`, does not
   end with one of its `skip_suffixes`, and holds no control character other than
   a newline (C0, DEL or C1): it reaches the CLI exactly as written or not at all.

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
- The writes are the chunks, then the submit key alone, each `chunk_interval_ms`
  apart, to `live.TerminalSession`. A write
  error settles the prompt `delivery_failed` (`failPrompt`).
- The receipt is the `UserPromptSubmit` hook. Ingress already matches the oldest
  pending prompt whose text and carrier runner equal the hook's, and flips it to
  `delivered`. A prompt the CLI never takes settles `not_acknowledged` on idle
  (`SettleUntaken`) because its delivery is not `steer`. Nothing new is polled.
- A prompt typed while the CLI is still inside the last turn's Stop names that
  turn's `prompt_id` on its `UserPromptSubmit` (measured 2.1.327, within ~30 ms
  of the Stop). Ingress reads an id it already knows as a retry only when no
  pending prompt awaits those words; otherwise the turn opens with no provider id.
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
   writes also 3 of 3. A bracketed paste is rewritten by the TUI and a single read
   of ~800+ bytes is taken as one, so delivery types keys instead (2.1.327: chunks
   of 256 bytes 10 ms apart, then `\r` alone, submitted every case below, 5,000
   characters and emoji/CJK across a chunk edge included). Enter after a trailing
   backslash is a line continuation and leaves the draft, so `skip_suffixes` sends
   such a message down the restart path.
3. **Sigils.** As typed keys a leading `!` still enters shell mode and a leading
   `/` still runs the command. So `leading_sigils` stays for the restart path
   and those messages are kept off typed delivery by `skip_prefixes`.
4. **Multi-line.** Text with an embedded newline is one draft, and `\r`
   sends one message.
5. **Busy.** The composer reads empty mid-turn too, so only the ledger guards
   against typing into a running turn.

Recordings: `api/internal/engine/agents/testdata/claude-composer-*-2.1.325.screen`
with a `.cursor` sidecar each. The trust dialog could not be re-recorded on this
account; the 2.1.319 recording stays the only evidence for it.

## 9. Tests

- `rules`: accepts a well-formed `type` block, rejects each broken shape.
- Gate (runner internal tests, fake commander): typed when all hold; restart
  when busy, pending, selection changed, no PTY, mode 2004 off, composer holding a
  draft, command text.
- Black-box `TestRegression_*` against the fake provider (`fakecli` made a
  composer: prints `ESC[?2004h`, reads keys, awaits the submit key): an idle
  prompt does not relaunch the CLI (same PID, same launch), the PTY receives the
  keys, the `UserPromptSubmit` hook flips it `delivered`; a draft in the box
  falls back to a restart.
- Live: a subagent started in turn one survives the prompt of turn two.

## 10. Known limits

- An idle notification dispatched just before the typed prompt's `UserPromptSubmit`
  settles the typed prompt `not_acknowledged`, and the turn then opens as a
  provider prompt (the text shows twice). Telling the two orders apart needs a
  timestamp the CLI does not send; the alternative leaves a prompt the CLI
  never took unsettled, which blocks the chat until Stop.
- A keystroke from an attached pane that opens a turn before the typed prompt is
  submitted also settles the typed prompt `not_acknowledged`.
- Only `/` and `!` were recorded as sigils that change meaning as typed keys.

## 11. Out of scope

Model, effort and permission changes keep `restart_tui`. Codex and Hermes are
unchanged; the block is generic, so they can declare it later.
