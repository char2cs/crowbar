# §13 accepted delta — native menus, DECIDED BY THE USER

**Decided:** 2026-07-31, verbatim: *"Dropdown menus should be native, not
'react' simulated."*
**Status:** decided. What waits here is the **spec edit**, not the decision.

This file exists because §13's accepted-deltas list is a user decision and I do
not add to it on my own. The user has now made one, so this records it and the
formalisation it implies, rather than my having quietly edited the spec.

## The delta

Crowbar-native uses **real platform menus**, so a menu will not match the React
app pixel-for-pixel. It will look like a macOS menu, because it *is* one.

Concretely, against `web/src/components/ui/dropdown-menu.tsx`: the popup's
background, radius, border, padding, item height, font and the highlight colour
are all AppKit's, not `theme.css`'s. Keyboard navigation, Escape,
click-outside, submenu open/close timing and screen-edge flipping become the
OS's behaviour rather than base-ui's.

## What it costs, stated plainly

`dropdown-menu` leaves the §5.1 strict-parity gate. It **cannot** be
anchor-diffed: an `NSMenu` is not in the window's view tree and has no
`data-oracle-id` to read. So this surface is **judged, not diffed** — the same
treatment §5.2 gives the editor, diff and terminal, and for the same reason:
there is no comparable reference.

That is a real reduction in what the oracle covers, and it should be visible in
the final report rather than absorbed silently.

## What it buys

OS keyboard navigation, VoiceOver reachability, submenu timing and screen-edge
flipping, none of which we would otherwise owe an implementation — §10.4 dropped
the AX spike as "THIN", and a platform menu is accessible without it.

## Consequences already handled

- **P2.1's GPUI port is superseded, not deleted.** It reproduced base-ui's
  structure and Tailwind styling specifically so it could be pixel-diffed. Its
  `MAPPING.md` section stays — the `ring-1`-is-a-box-shadow trap and the measured
  Tailwind values are surface-independent. The component is retired only once the
  native one is proven.
- **P2.8's fixture is not wasted.** The review thread it created through the
  daemon's API is what makes any menu reachable at all, and a native menu still
  needs a trigger to judge.
- **P2.9, the parity capture, was stopped mid-flight** rather than finished, as
  soon as the target changed. Its partial work is discarded on purpose.

## The formalisation still owed

Spec §13's list should gain an entry naming this delta, and §5.1 should stop
listing `dropdown-menu` as strict-parity. **I have not edited the spec** — it is
authoritative, and a §13 entry is exactly the thing I am told not to add on my
own initiative. The decision is made; the edit is a separate, deliberate act.

## The one open implementation question

`NSMenu` for true context menus, **GPUI-drawn** for anything that must carry the
design tokens or live inside a pane. The user did not rule between them, so I
took that split as the reading and said so before acting. If it is wrong, the
`NSMenu` work is confined to `crowbar-platform` and a driver surface, and the
GPUI path is what P2.1 already built.
