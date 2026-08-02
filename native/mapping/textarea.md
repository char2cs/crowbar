# `textarea` (P3.27) — built, and unreachable by any parity run this item could drive

`web/src/components/ui/textarea.tsx` (`export function Textarea`) →
`crates/crowbar-ui/src/components/textarea.rs`,
`crates/crowbar-app/src/{surfaces,row_layout}/textarea.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

A `<span data-slot="textarea-control">` carrying every painted property,
wrapping a `<textarea data-slot="textarea">` — the same two-element shape
`input.md`'s own §12 predicted for it: *"`textarea`, `select`, `checkbox`
and `radio-group` are all next"* for the void-element reason §2 below
restates.

**There is no `/tmp/p3-ref-textarea.json`.** See §5 for the measurement
technique used instead and §6 for why zero reachability is a finding rather
than a gap this item declined to close.

**Live count: 1 importer, 0 reachable.** `textarea.tsx`'s only importer is
`web/src/features/git/components/commit-popover.tsx`
(`<Textarea autoFocus value={message} … placeholder="Commit message…"
className="ui-font ui-text-sm min-h-20 resize-none" />`), reached through
the sidebar's Git panel "Changes" list and its "Commit changes" popover
trigger. `native/mapping/popover.md` §0 already reported this exact call
site unreached in an earlier item ("needs a dirty worktree"). This item's
`oracle-fixture` project genuinely has one — see §6.

## 0. Wrap or build: the seam test, applied

`native/vendor/gpui-component/src/input/` has a multi-line `Input` mode
(`InputState::multi_line`, `input/mode.rs`). It is the same primitive
`input.rs` already applied the seam test to and built rather than wrapped,
and the multi-line half changes nothing about the verdict: the whole
editable text element — cursor, selection, line wrapping, syntax
highlighting hooks, the works — is `input/element.rs`'s 100KB+
`InputElement`, a low-level `Element` impl with no `ParentElement`/`Styled`
seam a caller could reach *through* to supply the one box this surface
needs anchored. `Input::prefix`/`::suffix` exist and land *inside* that
private element, the same shape `number-input.md` §0 found for its own
`Input::new(&state)` child — and `textarea.tsx` uses neither: it is a bare
`<textarea>`, no affix slots at all. **Verdict: built**, from raw `div()`s
— `input.rs`'s own call, confirmed independently on the multi-line half of
the same vendor primitive rather than assumed to carry over from the
single-line finding.

## 1. The primitive

```text
<span data-slot="textarea-control">   ← every painted property
  <textarea data-slot="textarea">     ← the box the value/caret sit in
</span>
```

## 2. The field has no text node — `input.md` §1, confirmed on `<textarea>` independently

`input.md` predicted this and this item confirms it directly rather than by
inheritance: a React-controlled `<textarea value=…>` sets the DOM `.value`
**property**, never a child text node. Measured on a throwaway element:
`textarea.childNodes.length` is **0** with `value = "Fix the thing"` set —
the identical zero `input.md` measured for `<input>`. So exactly `input.rs`'s
reasoning applies: no `AnchorSink` text method is used, and a reference
(were one reachable) would carry no `text`/`fg`/`text_width`/`clipped`/`font`
for either anchor.

## 3. Values — the control

Every "Compiles to" below is a throwaway-element measurement — see §5 — not
a live capture.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-full` | fills the parent | `.w_full()` | `bounds.w` |
| `rounded-lg` | **10px** | `theme.radius_lg` | `radius` = 10 |
| `border border-input` | `1px`, `oklch(1 0 0 / 0.08)` | `.border(BORDER_WIDTH).border_color(theme.input)` | `border.w` = 1, `border.color` compared |
| `bg-background`, `dark:bg-input/32` | light: the bare token; dark: `oklab(1 0 0 / 0.0256)` | `Textarea::background` — the **identical** two-token pair `input.rs`'s `Input::background` documents | `bg` |
| `text-base sm:text-sm` (unless overridden — see §7) | `16px`/`14px` | `theme.ui_text_lg`/`ui_text_base` | invisible — inherited by the field, not painted here |
| `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | invisible |
| `shadow-xs/5` | gpui's preset, byte-identical (`input.rs`'s finding, confirmed again) | `.shadow_xs()` | §6, no field |
| `has-focus-visible:ring-[3px] ring-ring/24` | one box-shadow | a `BoxShadow` | §6, no field |
| `has-focus-visible:border-ring` | border colour → `theme.ring` | `Textarea::border_color` | compared, unreachable (`document.hasFocus()` false) |
| `has-aria-invalid:border-destructive/36`, `has-focus-visible:has-aria-invalid:border-destructive/64` | **byte-identical** rules to `input.tsx`'s | `INVALID_BORDER_ALPHA`/`INVALID_FOCUS_BORDER_ALPHA`, same numbers as `input.rs`'s own | compared, no reference |
| `has-focus-visible:has-aria-invalid:ring-destructive/16`, `dark:…/24` | **byte-identical** to `input.tsx`'s | `INVALID_RING_ALPHA`/`INVALID_RING_ALPHA_DARK` | §6, no field |
| `has-disabled:opacity-64` | opacity | `DISABLED_OPACITY` | **invisible** — v1.7 fires only at zero |
| `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none` | drops the shadow | `Textarea::has_shadow` | §6, no field either way |
| `not-dark:bg-clip-padding` | `background-clip` | nothing | §6, no gpui equivalent |

**`input.rs`'s exact `dark:bg-input/32` background pair**, confirmed
independently rather than assumed from the shared substring: measured
`oklab(1 0 0 / 0.0256)` in dark, and `0.08 × 0.32 = 0.0256` — `theme.input`
mixed at the same 32% `input.rs`'s own `DARK_BACKGROUND_ALPHA` names.

## 4. Values — the field

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-full` | fills the control's **content** box | `.w_full()` | `bounds.w`; inset by the control's border, `input.rs`'s own control/field relationship |
| `rounded-[inherit]` | the control's own **10px**, read twice | `theme.radius_lg` | `radius` = 10 |
| `px-[calc(--spacing(3)-1px)]` (`default`), `-(2.5)` (`sm`), unchanged (`lg`) | **11 / 9 / 11px** | `Size::padding_x` | `bounds` |
| `py-[calc(--spacing(1.5)-1px)]` (`default`), `-(1)` (`sm`), `-(2)` (`lg`) | **5 / 3 / 7px** | `Size::padding_y` | folds into the min-height arithmetic |
| `min-h-17.5`/`16.5`/`18.5` | **70 / 66 / 74px** — a **floor**, not the content's own height | `Size::min_height` | `bounds.h`, see §6 |
| `field-sizing-content` | grows with typed content | **not modelled** — see §6 | no field either way |
| inherited font (`text-base`/`ui-text-sm`, depending on the call site — §7) | `14px`/`20px` measured with the live call site's own `className` | `CallSite::text_size` | invisible — void element, §2 |
| `outline-none` | `outline-style: none` | nothing | no field |

## 5. How the values were measured, given zero reachability

Not inferred from the class name — every number above (except the "Wrap or
build" section) came off `getComputedStyle` on a `<span>`/`<textarea>` pair
built with `textarea.tsx`'s **own compiled class strings**, injected into
the live app's DOM (which already carries the compiled Tailwind sheet these
classes resolve against — the discipline `radio_group.rs`'s module docs and
`native/mapping/radio-group.md` establish, followed here rather than
re-derived) and removed immediately after. **This is not a capture of the
primitive mounted through React** — no `/tmp/p3-ref-textarea.json` was
written, and none is claimed. It differs from an oracle capture in the way
that matters: a mounted capture reads what React actually rendered for a
real call site; this reads what the same compiled CSS rules resolve to on a
hand-built element carrying the identical class strings — a measurement of
the stylesheet, not of a fabricated "capture."

One correction the injection needed and `radio_group.rs`'s did not:
`size==='sm'`/`'lg'` each add a **conflicting** `min-h-*`/`px-*`/`py-*`
utility over the `default` arm's, and `cn()` (`tailwind-merge`) drops the
earlier one — a plain string concatenation does not. The first pass measured
`sm` and `default` at the same 70px because both `min-h-17.5` and
`min-h-16.5` were present in one class list and the compiled sheet's
*declaration order*, not the override's, won. Re-measured with the
conflicting base utilities dropped by hand (matching what `tailwind-merge`
keeps): `sm` came back **66px**, distinct from `default`'s 70 and `lg`'s 74
— all three whole `--spacing` multiples, and pinned in a real `row_layout`
window (`every_size_arms_own_floor_is_measured`) as well as unit-tested.

## 6. `field-sizing: content` has no gpui equivalent, and this port does not need one

`field-sizing-content` makes a `<textarea>` grow to fit its **typed**
content past its `min-height` floor. GPUI lays out one static instant; there
is no keystroke to grow from. What this surface renders is the resting,
**empty** cell — `commit-popover.tsx`'s own initial state
(`useState('')`) — where the intrinsic content height never exceeds the
floor (measured: an empty throwaway textarea's own height equals its
`min-height` exactly, at all three sizes). So `Textarea::field_height` is
the floor, unconditionally (see §7 for the one authored override that
stretches it), and growth beyond it is simply not a picture this port
draws — the same call `input.rs` makes about the caret: not approximated,
just outside what a snapshot is.

## 7. The one real call site's own override, modelled as `CallSite`

`commit-popover.tsx`'s `<Textarea className="ui-font ui-text-sm min-h-20
resize-none" …/>` merges onto the **control**, never the field —
`className` is destructured out of `TextareaProps` before `...props` reaches
the field's own `cn()` call, so a call site can restyle the control but
never touch the field's class list directly. Three real effects, all
measured on the injected pair with this exact class string appended:

* `ui-text-sm` (12px/18px) **replaces** the control's own `text-base` (same
  font-size group) and, since `textarea { font: inherit }` is Tailwind's
  preflight rule, cascades down into the field — measured `fontSize: 12px`
  on the field with this className present, `14px` without it.
* `min-h-20` (80px) is a **second, independent** min-height on the control,
  on top of the field's own 70px floor. Since the control is `inline-flex`
  with default (`stretch`) cross-axis alignment and one child, the floor
  that wins is whichever is taller: measured control `80px`, field
  **stretched to 78px** (`80 − 2×border`) — *taller* than the field's own
  unstretched 70px floor. `CallSite::CommitMessage` carries this as
  `control_min_height`, and `Textarea::field_height` takes the max of the
  field's own floor and the stretched value — confirmed in a real
  `row_layout` window (`bare_drops_the_call_sites_min_height`), where taffy
  independently produces the same 78px.
* `resize-none` lands on the **control**, never the field — the one element
  a browser's native resize handle actually reads. Measured: the field's
  own `resize` computes `vertical` (`WebKit`'s textarea default), unchanged
  by the call site's class. Not a defect this port introduces; the contract
  has no field for it either way (§6-equivalent, no resize-handle
  geometry).

`ui-font` was measured too and found inert here: the app's own default sans
stack is already `CalSansUI, …`, so the explicit class changes nothing
measurable.

## `CONTENT_SIZED` / `LINE_SIZED`

Both empty, for `input.rs`'s exact §1/§2 reasons: the field is a void
`<textarea>` (§2) with no `font` group on either side, so v1.6's
`bounds.h`-against-`font.line_height` comparison has nothing on the other
side; the control's height is either an authored `min-h-*` or a stretched
child, never a text run's max-content extent on either axis.

## Reachability — measured in full, not asserted

The claim "0 reachable" is stated plainly elsewhere in this file; here is
the evidence behind it, because a bare "unreachable" is exactly the kind of
claim this port's brief asks to be checked rather than taken on trust.

1. **The fixture workspace genuinely has dirty files.** Queried directly
   against the daemon's own `GET
   /v0/projects/:pid/repos/:rid/workspaces/:wid/git/status` (via the app's
   `crowbar://localhost` protocol handler, from inside the live webview):
   `{"branch":"(detached)","ahead":0,"behind":0,"files":[
   {"path":"src/a/a.ts","status":"modified","staged":false},
   {"path":"src/a/deleted-file.ts","status":"deleted","staged":false},
   {"path":"src/a/staged-file.ts","status":"added","staged":true},
   {"path":"src/features/terminal/lib/an-extremely-long-file-name-…","status":"modified","staged":false},
   {"path":"src/features/terminal/lib/resolve-terminal-connection.ts","status":"modified","staged":false},
   {"path":"src/a/untracked-new-file.ts","status":"untracked","staged":false}]}`
   — six real changes, matching the fixture files `git-status-row.md`'s own
   fixtures already name.
2. **The sidebar's "Changes" tab nonetheless renders empty** — a virtualised
   list with two zero-height placeholder rows and no file entries — across
   a full `location.reload()`.
3. **A real, successful git action did not fix it either.** `POST
   .../git/stage {"paths":["src/a/untracked-new-file.ts"]}` returned
   `200 {"success":true,…}` — a genuine app-level mutation through the
   correct endpoint, not a synthetic disk edit — and the "Changes" tab
   still rendered empty afterward, including after a further full reload.
   The stage was reverted (`POST .../git/unstage` on the same path) before
   moving on.
4. This is a **frontend staleness bug independent of this item's scope** —
   the backend computes and returns correct status on every request; the
   sidebar's own git-store subscription simply never picks it up in this
   session. Not something this item is positioned to fix (out of the
   `data-oracle-*`-and-Rust-port remit this brief sets), and not routed
   around by fabricating a capture.

A file this item edited and reverted in the course of checking the above:
`a.ts` in the **protected `main` branch's own locked worktree**
(`.../oracle-fixture-demo/main/worktree/src/a/a.ts`, a *different* checkout
from the "home" workspace's `/tmp/crowbar-oracle-fixture/demo` — the repo
has two workspaces on the same commit) was briefly changed from
`export const a = 1` to add a comment, then restored to its original
one-line content via the same tool. The "home" workspace's own dirty files
(listed above) were left exactly as found; only the `untracked-new-file.ts`
stage/unstage round-trip touched them, and both ends of that round-trip are
confirmed identical by re-querying `git/status`.
