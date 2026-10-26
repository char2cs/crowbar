# Provider terminal-prompt surface

## Problem

A provider blocked on its own modal (trust, login, permission) is shown in chat as
a sentence plus a Terminal button. On a phone the terminal is a poor place to
answer a one-keystroke question.

## Shipped

- Descriptor: a `terminal_prompts` entry may declare `answers: [{id, label, keys}]`
  (`keys` is the PTY keystroke string). Claude declares `accept` / "Trust workspace"
  (Enter, which takes the dialog's default "Yes, I trust this folder"). Codex
  declares none, so it keeps the Terminal button only.
- Daemon: `AgentTerminalWait` carries the matched prompt's answers; the wire
  `terminalWait` exposes `answers: [{id, label}]` (keys never leave the daemon).
  `POST .../chats/:id/terminal-wait/answer {answerId}` writes the keys to the
  chat's live PTY (409 once the prompt is gone, 400 for an unknown answer). The
  existing sweep clears the wait when the prompt leaves the screen.
- Web: the composer signpost and `AgentTerminalWaitBanner` render one primary
  button per declared answer beside the Terminal button.

## Not done

- A decline answer. The claude dialog's second row is "No, exit", but no fixture
  shows what it does to the chat, so none is declared.
- The matched screen text and multi-option prompts (login, theme picker). Those
  need an `options` list and the existing choice-card controls.
- Codex's trust prompt: its choices are not captured in any fixture.
