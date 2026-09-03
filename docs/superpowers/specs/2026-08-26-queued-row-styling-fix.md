# Queued-row styling: reconnecting a real bug, not a redesign

**Date:** 2026-08-26

**Status:** fixed in `transcript.css`, mirrored in the chat-state-lab review artifact
(https://claude.ai/code/artifact/8b011a0a-cd20-4c4a-9836-0f1e195ac98f)

## The bug

`QueuedRow` (`queued-row.tsx`) renders `<article className={cn('queued', multi && 'multi', failed
&& 'bad')}>` containing `.txt` / `.err` / `.acts` as flat children — directly into `.stream`, not
wrapped in `.row.me`. None of `.queued`, `.txt`, `.err`, `.acts` were defined anywhere in
`transcript.css`, `composer.css`, or `activity.css` (grepped all three). A queued prompt shipped
as plain unstyled text with tiny icon buttons crammed against it.

The intended design was already written down — just pointed at class names the component doesn't
use:

```css
/* transcript.css, dead code before this fix */
.agent-chat .bubble.pending { border: 1px dashed var(--input); background: ...; }
.agent-chat .bubble.bad { border: 1px solid ...; background: ...; }
.agent-chat .pending-err { margin: 6px 0 0; font-size: var(--ui-text-xs); color: var(--warning-foreground); }
.agent-chat .pending-acts { display: flex; justify-content: flex-end; gap: 2px; margin-top: 6px; }
```

with a comment explicitly saying "A queued prompt is the SAME bubble... Dashed because it is not
in the record yet; solid + warning once it needs attention." Somewhere along the way the
component's classes changed and the CSS never followed.

## The fix

Retargeted the same intent at the real class names, with one structural correction: since
`QueuedRow` has no `.bubble` wrapper element to carry the box styling, `.txt` — not the outer
`.queued` article — is the pill now. It mutates via the ancestor's `.multi` / `.bad` classes
rather than its own, so "the same bubble... only the border and background swap out from under
it" is now literally true, not just the comment's intent:

```css
.agent-chat .queued { display: flex; flex-direction: column; align-items: flex-end; align-self: flex-end; max-width: 88%; }
.agent-chat .queued .txt { border-radius: 9999px; border: 1px dashed var(--input); background: ...; padding: 9px 18px; ... }
.agent-chat .queued.multi .txt { border-radius: 18px; padding: 10px 16px; }
.agent-chat .queued.bad .txt { border: 1px solid ...; background-color: ...; background-image: repeating-linear-gradient(...); }
.agent-chat .queued .err { margin: 6px 0 0; font-size: var(--ui-text-xs); color: var(--warning-foreground); }
.agent-chat .queued .acts { display: flex; gap: 2px; margin-top: 6px; }
```

**Actions render outside the pill, not inside it.** The dead code's `.pending-acts` was written
assuming everything lived inside one `.bubble` box. Moving `.acts` out from under `.txt`'s
border/background — a plain row below the pill instead of icons floating inside its padding — was
a design correction made against the actual rendered result, not something the original comment
called for. `.err` stays where the dead code put it: content about the message, so it stays near
the message.

No component changes were needed — `queued-row.tsx`'s existing flat `.queued > .txt / .err /
.acts` structure already supports this; only which element gets boxed changed.

## The diagonal hatch on `.bad`

A further embellishment, proposed rather than restored: the failed state's warning tint gets a
subtle diagonal hairline pattern layered on top, using the same `repeating-linear-gradient(315deg,
...)` / `10px 10px` tile recipe as tailwindcss.com's own pattern swatches. Translated to Crowbar's
own idiom — `color-mix(in oklch, var(--foreground) 6%, transparent)` — instead of Tailwind's raw
`--color-black` / `--color-white`, so it inverts automatically between themes the way every other
token in this file already does, with no separate dark-mode rule needed.

## Follow-up: error text hidden, hatch tightened

Two further tweaks on top of the fix above:

- `.err` is now `display: none`. The pill's own warning tint + hatch already flags "something's
  wrong" on sight, and the retry button's tooltip still carries the specific reason on hover —
  the inline error line was judged redundant. `QueuedRow` still renders the span whenever
  `item.error` is set; only its visibility changed, nothing in the component.
- The hatch tile shrank from `10px 10px` to `6px 6px` — same recipe, lines closer together.

## How this was verified

`web/src`'s CSS isn't buildable in isolation from a static HTML page — Tailwind v4's generated
`--color-*` scale (`--warning` resolves through `var(--color-amber-500)`, `--foreground` through
`var(--color-neutral-800)`, etc.) only exists once the full Vite/Tailwind pipeline runs. Booting
the real dev server risked touching another session's cached workspace state over a shared
`file://` origin (a known hazard — dev webviews share IndexedDB), so instead: a throwaway page
linked the real `theme.css` + `transcript.css` by absolute path, with the specific missing
`--color-*` tokens patched in at their real Tailwind default values, and the *exact* DOM shape
`queued-row.tsx` renders. That caught a real false alarm (the `.bad` state initially rendered with
no border or background at all — track it down before assuming the fix itself was broken; it was
the harness missing `--color-amber-500`, not the CSS) and confirmed the corrected fix computes
right once the missing tokens are supplied. Not a substitute for opening the real running app, but
enough to verify the CSS text is mechanically correct before mirroring it into the lab.
