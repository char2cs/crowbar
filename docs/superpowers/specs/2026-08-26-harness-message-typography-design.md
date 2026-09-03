# Harness message: typography, not translation

**Date:** 2026-08-26

**Status:** design, prototyped in the chat-state-lab review artifact
(https://claude.ai/code/artifact/8b011a0a-cd20-4c4a-9836-0f1e195ac98f)

## Current

A `harness` message renders a raw provider payload verbatim — `<task-notification id="a1b2"
status="completed"/>` is the measured case — inside a plain `<span>`, same proportional font as
everything else in the card (`message-row.tsx:77-95`, `transcript.css` `.harness, .notice` rule).

## Why this content looks like it does

This payload rides the model's own user-turn channel: a CLI's background-harness machinery has
no separate wire-level role for "synthetic internal event," so the only way it can inject
something into an ongoing conversation is on the turn a human's messages also occupy. Crowbar
already corrects for that on the human's side of the screen — the disclaimer line above the
payload, "Sent to the agent by {provider} — not by you" (`message-row.tsx:79`), exists
specifically because the wire protocol makes this look user-authored when it wasn't.

## What changed, and what deliberately didn't

**Typography only:** the payload now renders in `<code>`, using the same `var(--font-mono)` /
`var(--muted-foreground)` pairing `.agent-prose code` already uses for markdown code spans
(`transcript.css:287`) — this row is the same *kind* of thing, literal machine text rather than
prose, regardless of which provider produced it. The card around it and the disclaimer line
above it are untouched.

**Deliberately not attempted:** parsing `text` for meaning — reading `status="completed"`,
picking an icon or label based on the tag name, anything that would make Crowbar's renderer
learn one provider's own wire vocabulary. That crosses a line this codebase already draws
elsewhere: generic layers don't carry provider-specific wire-format knowledge. `<task-notification>`
is Claude's own shape; a different provider's harness payload could look nothing like it, and a
renderer built around this one's field names would be wrong for that one on day one.

If genuine semantic prettification is wanted later — "Subagent finished" instead of raw XML —
the honest way to get there is a provider-declared display template (the provider's own
descriptor says how to render its own event shapes), not the frontend hardcoding Claude's tags.
That's a separate, larger piece of work and isn't attempted here.

Prototype: the `<code>` branch of `row('harness', ...)` and the `.harness code` rule in the lab
artifact.

## A long payload collapses instead of pushing the turn down

Real harness payloads run bigger than the measured `<task-notification id="a1b2"
status="completed"/>` case — a subagent completion report can carry several attributes plus a
multi-line body. Rather than let that push the rest of the conversation down (or invent a new
accordion widget), this reuses a pattern Crowbar already ships: `ChoiceSchema`
(`composer-choice.tsx:540-549`) collapses a permission card's JSON Schema behind a native
`<details><summary>What it is asking for</summary><pre className="max-h-40 overflow-auto ...
bg-muted/50 ... font-mono">`. Same element, same idea, translated from Tailwind to this file's
own tokens (`var(--radius-md)`, `color-mix(in oklch, var(--muted) 50%, transparent)`,
`var(--font-mono)`) — not a new visual language.

The threshold (`text.length > 140` or 2+ newlines) is a plain length/line-count check, the same
kind of heuristic `row()` already uses for `multi` just above it — it has no idea what the
payload means, only how big it is. A short payload stays exactly as described above, readable
without a click, since collapsing something that already fits on one or two lines would make the
common case worse, not better.

Prototype: the `big` branch of `row('harness', ...)`, the `.harness summary` / `.harness details
pre` rules, and the standalone "Harness message" state (shows both sizes stacked, with the long
example illustrative rather than a captured real trace).
