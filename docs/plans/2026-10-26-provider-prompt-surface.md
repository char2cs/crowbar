# Provider terminal-prompt surface

## Problem

A provider blocked on its own modal (trust, login, permission) is shown in chat as
a sentence plus a Terminal button. On a phone the terminal is a poor place to
answer a one-keystroke question.

## What the daemon exposes today

`terminal_prompts` in a descriptor is `{kind?, needle}`. The matched prompt is
published as `AgentTerminalWait{Waiting, Kind}` and reaches the client as
`terminalWait: { kind }`. `kind` is only `workspace_trust` or empty. There are no
options, no question text, and no answer keys. Only the needle (which is not sent)
says what was matched.

## What would be needed

1. Descriptor: per terminal prompt, an `options` list, each with `id`, `label`,
   `kind` (accept/deny) and the `keys` the PTY expects. Keys are provider data and
   belong in the descriptor, not in Go.
2. Daemon: carry `options` and the matched screen text on `AgentTerminalWait`, and
   add an answer endpoint that writes the chosen option's keys to the chat's PTY,
   gated like choice answers (409 when the prompt has gone).
3. Client: render the options through the existing choice-card controls and fall
   back to the Terminal button when `options` is empty.

Until then the banner stays a message plus a one-tap Terminal button.
