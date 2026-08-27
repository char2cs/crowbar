# Everything happens at the composer, nothing explains itself twice

**Date:** 2026-08-26

**Status:** design, prototyped in the chat-state-lab review artifact
(https://claude.ai/code/artifact/8b011a0a-cd20-4c4a-9836-0f1e195ac98f)

Six related fixes, all downstream of one complaint: several real states put content the user
needs to read or act on somewhere other than the composer — a floating top banner, a paragraph
justifying a limitation, a link sitting outside the bar it belongs to. `composer-state.ts` already
states the rule the rest of the app should follow: *"it is an input when you can talk, and it is
the question, the permission, or the reason you cannot, when you cannot. One box, one occupant,
never two stacked."* These fixes bring the outliers in line with a rule the codebase already
believes in.

## 1. Sending shows a spinner, not an idle-looking arrow

**Current:** the send button looks identical (up-arrow, primary color) whether idle or actively
mid-flight to the provider.

**Proposed:** a `sending` mode reuses `FlickerSpinner` — the same spinner `WorkingLine` already
uses — in place of the arrow, with a tooltip (`title`, the same mechanism every button's
`tooltip` prop already compiles to in this file) explaining what's happening. Left enabled, not
disabled — some browsers suppress `title` tooltips on disabled elements.

Prototype: `sendBtn()`'s `sending` branch, `.send.sending` CSS, the "Submitting" state.

## 2. Idle/reviving states move from a floating top banner into the composer

**Current, verified against `agent-chat-pane.tsx` ~1010-1034:** reviving, a clean exit, and a
failed restart attempt all show `absolute inset-x-4 top-2` — a bar pinned to the pane's top edge,
independent of the composer at the bottom. `terminal_wait` additionally showed this real content
*twice*: once as this top banner, once as the `ComposerSignpost` bottom bar that already existed
and already said the same thing.

**Proposed:** all of it moves into the dock, alongside `dormant`/`unsupported`/`terminal_wait`,
which already lived there. The redundant top banner for `terminal_wait` is simply removed — the
bottom signpost was already correct.

**Also proposed, going further than repositioning:** a clean exit (`idle-exited`) no longer waits
for a click. It auto-retries silently, exactly like a manual provider switch already does
(`reviving`) — nothing to see unless the attempt fails, at which point it becomes `idle-failed`,
the one case that still surfaces a Resume button. That one keeps the click deliberately: a CLI
that genuinely isn't installed can't retry itself into working, and retrying blind would either
loop or fail silently. This is closer to the spirit of the original complaint ("I only have to
type and Crowbar handles the rest") than a positioning fix alone would be, but it's a real
behavior change, not just layout, and is flagged as such — worth confirming before it ships.

Prototype: `idleDockBar()` (replacing `idleOverlay()`), `.pill.halted.quiet` for the two states
where nothing is actually wrong, the `terminal-wait` render case with `bannerNotice()` removed.

## 3. Multi-question grows one box, not a card plus a separate pill

**Current:** `ComposerChoice`'s question content and its action bar were already one `.asks`
wrapper in markup, but `.asks` itself carried no box styling — visually it read as a floating
question card sitting above an unrelated pill, not one thing.

**Proposed:** `.asks` now carries the border/background (`.pill.multi`'s own radius, since this is
never a one-line case), and the nested `.pill.asking` gives up its own box to become a plain
bottom row inside it. Same visual language the composer already uses for growing to fit a
multi-line message — this grows to fit questions instead.

Prototype: `.asks` / `.asks .pill.asking` CSS. No markup change needed — the wrapper already
existed.

## 4. Permission-card copy: redirect, don't justify

**Current, across five variants:** each one that can't be fully handled from Crowbar explained
*why* in a sentence or two — "Filling this form in has to be done in the terminal — Crowbar
cannot compose the answer it is asking for," "Crowbar cannot send the broader permission this
provider also offered... It declares no shape for them, and one narrowed to a plain allow would
grant something else," "This can no longer be answered from Crowbar — answer it in the terminal,"
"This provider cannot be answered that way from here: unsupported response shape," and a separate
ghost link floating above the bar for the fully-unanswerable case.

**Proposed:** every one of those sentences is gone. What's left is a Terminal button living
directly in the pill's own action row — the same place Deny/Allow/Decline/Cancel already are —
for every case where terminal is the actual next step. No paragraph justifies the redirect; the
button just is the redirect. The "Sent" variant's "Answer sent. Waiting for Claude to confirm it."
is removed the same way — the pill itself, buttons gone, already carries that meaning.

Prototype: `renderPermissionVariant()`'s `form-only`, `unanswerable`, `suggestions`, `sent`,
`error-409`, `error-400` branches.

## What's untouched

`Answerable` (single permission, already one pill, no complaint), the general `Halted` state
(real, sourced from an actual `notice` message, not touched here), and `dormant`'s own signpost
copy (verified real, `composer-state.ts:75` — not flagged this round, left as-is).
