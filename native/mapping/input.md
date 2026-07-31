# `input` (P3.4)

`web/src/components/ui/input.tsx` → `crates/crowbar-ui/src/components/input.rs`,
`crates/crowbar-app/src/surfaces/input.rs`.

> **This file is a §6.2 section that does not live in `native/MAPPING.md`.**
> Parallel workers writing one file conflict; the "How to read a row" preamble,
> the column meanings and the append-only rule are `MAPPING.md`'s and apply here
> unchanged. **Read the Traps section of every component before porting a new
> one** — including this one, and including §1, which is the only reason this
> component is interesting.

**Compile the CSS, do not read the class name.** Every "Compiles to" below came
from running the app's own `src/index.css` through its own `tailwindcss` 4.3.0
with the utility as a candidate. Every "Measured" came off the **live** app —
pid 64880, bridge 9223, `innerWidth` 1714 at DPR 2, `html.dark`, the tree filter
in the file-explorer panel. `--spacing` is Tailwind's stock `0.25rem`; `--radius`
is `0.625rem` and `--radius-lg` is `var(--radius)`, so `rounded-lg` is **10px**
rather than Tailwind's stock 8.

The primitive is two elements and one pseudo:

```text
<span data-slot="input-control">     ← every painted property
  ::before                           ← an inset-shadow overlay
  <LeftIcon/>?                       ← a call site's component, absolutely positioned
  <input data-slot="input">          ← the box the caret sits in
</span>
```

`nativeInput` chooses between a bare `<input>` and `@base-ui/react`'s `Input`.
**It makes no difference to any of this**: both render an `<input>` with the same
`inputClassName`, the same `data-slot`, and `{...props}` spread last.

---

## 1. The headline: an `<input>` has **no text node**, so the whole text group is gone

This is a property of the *extractor*, not a judgement call, and it decides the
shape of the port.

`web/src/lib/oracle/extract.ts` builds `text`, `fg`, `text_width`, `clipped` and
`font` from **one** source — `oracleOwnText(el)`, which walks `el.childNodes`
looking for `nodeType === 3`. An `<input>` is a **void element** and has no child
nodes at all, so `extractSnapshot`'s `if (text.length > 0)` branch is **never**
taken for it.

Measured on the live app rather than reasoned about:

| probe | live value |
|---|---|
| `input.childNodes.length` | **0** |
| `oracleOwnText(input)` | `''` |
| `document.createRange().selectNodeContents(input).getClientRects().length` | **0** |
| `range.getBoundingClientRect().width` | **0** |
| `input.scrollWidth` / `input.clientWidth`, with `Search` showing | **224 / 224** |

So even the fallback clip signal (`scrollWidth - clientWidth`) is dead: a value
longer than the field would report **no** overflow, because a scrolled input's
`scrollWidth` tracks its own scroll box rather than its content.

**What the reference's two anchors carry, in full:** `id`, `bounds`, `bg`,
`visible`, `radius`, `border`. Nothing else. `/tmp/p3-ref-input.json` is the
archived proof.

### What follows for the port

The string the component paints goes through **no `AnchorSink` text method at
all**. It is a plain unanchored child, the same call `tabs` made for a tab's
label span — but here the reasoning is the opposite way round and worth stating,
because the wrong answer looks *more* defensible:

* `tabs`: the label is a `<span>` the **call site** puts inside the tab, so the
  tab owns no text and recording one would invent it.
* `input`: the field genuinely **does** paint its own string, and it is *still*
  unanchorable — because the string is not in the DOM.

Routing it through `AnchorSink::text_half` produces five `FieldPresence` deltas
per cell in the `git-row-badge` shape with the sides swapped. **Mutation-tested:
1 failure**, §11.

---

## 2. What the anchor set can and cannot see about a text field

Recorded plainly, because §8.2 requires it and because this is the question the
item exists to answer.

| | seen? | why |
|---|---|---|
| the control's `bounds` | **yes** | compared at ±0.5 |
| the control's `bg` | **yes** | `#ffffff07` in dark |
| the control's `radius` | **yes** | 10 |
| the control's `border.w` **and** `border.color` | **yes, both** | `1` / `#ffffff14`; v1.3 compares the colour because `w > 0` |
| the control's `visible` | **yes**, and it is *not* free — see §9 |
| the field's `bounds` | **yes** | `21,1,224×26` |
| the field's `radius` | **yes** | 10, through `rounded-[inherit]` |
| the field's `border.w` | **yes** | `0` — preflight's `border: 0 solid` |
| the field's `border.color` | **no** | v1.3: ignored while `w == 0`. The reference emits `#ffffff0f` and the differ never looks |
| the **value** | **no** | §1 |
| the **placeholder** | **no** | §1, and `getComputedStyle(input, '::placeholder')` returns the *element's* colour, so it is not readable by hand either |
| the placeholder's colour, size, family, line height, advance width | **no** | §1 — the whole `font` group, `fg` and `text_width` |
| whether the text is **clipped** | **no** | §1, and the `scrollWidth` fallback is dead |
| the **caret** | **no** | no element, and §3's pseudo shortcut does not reach it |
| the **selection highlight** | **no** | no element; `::selection` is user-agent paint |
| the field's **scroll offset** when the value overflows | **no** | not a field, and runtime state §6 excludes |

**The caret and the selection are recorded, not modelled, and no field is
invented for them.** They are a third kind of invisible, distinct from the two
this port has met before:

* `resizable`'s `::after` hit strip — a **pseudo-element**, so no node, and §3's
  `inset: 0` shortcut does not apply to it;
* `button`'s `::before` overlay — a pseudo the shortcut *would* apply to, and
  taking it would replace the host's record;
* a caret — **not a box at all.** It is a one-pixel rule the user agent blinks
  inside the field's content box, with no geometry any extractor can address.

---

## 3. Values: spacing, type, radius, colour

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `h-7.5 sm:h-6.5` (`sm`) | **30 / 26px** | `Size::extent` | compared |
| `h-8.5 sm:h-7.5` (default) | **34 / 30px** | `Size::extent` | compared |
| `h-9.5 sm:h-8.5` (`lg`) | **38 / 34px** | `Size::extent` | compared, no reference |
| `leading-7.5 sm:leading-6.5` etc. | `calc(--spacing * n)` — an **absolute length**, not a ratio | `Size::line_height` — its own table, see §5 | invisible |
| `px-[calc(--spacing(3)-1px)]` | **11px** | `Size::padding_x` | compared through the field's box |
| `px-[calc(--spacing(2.5)-1px)]` (`sm`) | **9px** | `Size::padding_x` | compared |
| `ps-7` / `ps-8` (with `leftIcon`) | **28 / 32px** — a whole step, no `-1px` | `Size::icon_gutter` | compared |
| `ps-5` (a **call site's**, on the control) | **20px** | `LeadingPad::Ps5` | compared — see §7 |
| `size-3.5` / `size-4`, `start-2` / `start-2.5` | **14 / 16px**, **8 / 10px** | `Size::icon_size`, `Size::icon_inset` | invisible (unanchored) |
| `min-w-0` | `min-width: calc(--spacing * 0)` = 0 | `.min_w(px(0.))` | compared |
| `w-full` (both elements) | `width: 100%` | `.w(relative(1.))` | compared |
| `rounded-lg` | `var(--radius)` = **10px**, *not* stock 8 | `theme.radius_lg.value()` | compared |
| `rounded-[inherit]` (the field) | `border-radius: inherit` → the control's **10px** | the same token read twice | compared |
| `before:rounded-[calc(var(--radius-lg)-1px)]` | **9px**; measured live | `Input::overlay_radius` | **invisible** (§6) |
| `border` (the control) | `border-style: var(--tw-border-style); border-width: 1px` — unconditional | `BORDER_WIDTH` | compared, **exactly** |
| `border-input` | `var(--input)` = `oklch(1 0 0 / 8%)` → `#ffffff14` | `theme.input` | compared |
| `bg-background` | `var(--background)` | `theme.background` | compared, **light only** |
| `dark:bg-input/32` | `color-mix(in oklab, var(--input) 32%, transparent)` → `#ffffff07` | `theme.input.mix(32, TRANSPARENT)` | compared — the live cell's |
| `text-base` / `sm:text-sm` | `1rem` on `calc(1.5/1)`; `0.875rem` on `calc(1.25/0.875)` | `theme.ui_text_lg` / `theme.ui_text_base` | invisible (no anchor paints text) |
| `text-foreground` | `var(--foreground)` | `theme.foreground` | invisible |
| `placeholder:text-muted-foreground/72` | `color-mix(… var(--muted-foreground) 72%, transparent)` on `::placeholder` | resolved into the run — see §4 | invisible |
| `shadow-xs/5` | `0 1px 2px 0` at `rgb(0 0 0 / .05)`; measured live as `oklab(0 0 0 / 0.05) 0 1px 2px 0` | `.shadow_xs()` — gpui's preset is **byte-identical** | invisible (§6) |
| `ring-ring/24` | `--tw-ring-color: color-mix(… var(--ring) 24%, transparent)` | `theme.ring.mix(24, T)` | invisible |
| `has-focus-visible:ring-[3px]` | `--tw-ring-shadow: 0 0 0 calc(3px + 0px)` — **a box-shadow** | a `BoxShadow`, spread 3px | invisible |
| `has-focus-visible:border-ring` | `border-color: var(--ring)` | `theme.ring` | **compared** — see §9 |
| `has-aria-invalid:border-destructive/36` | `color-mix(… 36%, transparent)` | `theme.destructive.mix(36, T)` | **compared**, no reference |
| `has-focus-visible:has-aria-invalid:border-destructive/64` | `… 64%` | `.mix(64, T)` | compared, no reference |
| `has-focus-visible:has-aria-invalid:ring-destructive/16` | `… 16%` | `.mix(16, T)` | invisible |
| `dark:has-aria-invalid:ring-destructive/24` | `… 24%` | `.mix(24, T)` — and it **wins** in dark, see §8 | invisible |
| `has-disabled:opacity-64` | `opacity: 64%` | `.opacity(0.64)` | **invisible** — v1.7's `visible` fires only at zero |
| `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none` | drops `shadow-xs` | `Input::has_shadow` | invisible |

**The `--ui-text-*` trade, third occurrence.** Tailwind's `text-base` is 1rem and
`--ui-text-lg` is the same 1rem; `text-sm` is 0.875rem and `--ui-text-base` is the
same 0.875rem. `MAPPING.md` states the trade once for the whole port. It does not
run out here — `button`'s fourth step (`text-lg` against `--ui-text-xl`) is not
one this component uses.

**Preflight is load-bearing and is not in any class list.** `input.tsx` writes
`text-base sm:text-sm` on the **control**, and what carries it down to the field
is Tailwind's own `button, input, optgroup, select, textarea { font: inherit }`.
Without that rule an `<input>` keeps the UA's 13.33px form font. Read out of the
compiled sheet.

---

## 4. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` on the control | `display: inline-flex` — but the **computed** value is **`flex`**, because CSS blockifies a flex item's display and every live control is one. Measured, not assumed | `.flex()`. gpui has no inline flow at all, so this is free — and unlike `button`, no flex host is needed above the root: `w-full` pins the width in a block container just as well | compared through geometry |
| `w-full` on the control, `w-full min-w-0` on the field | 100% of the parent, then 100% of the control's **content** box | `.w(relative(1.)).min_w(px(0.))` | compared |
| the control authors **no height** | its height is the field's border box plus its own two border pixels | nothing to write; measured **28 = 26 + 2** on the live app and reproduced by taffy | compared |
| `ps-*` against `px-*` | both survive tailwind-merge, and the compiled sheet emits **every** `padding-inline-start` rule *after* **every** `padding-inline` one, so `ps-*` wins on the start side | `.px(…)` then `.pl(…)` — later wins, same as the sheet | compared |
| `relative` + `before:absolute before:inset-0` | an overlay on the control's **padding** box: measured `244×26` at `0,0` against a `246×28` control, and **unaffected by `ps-5`** — an abspos inset resolves against the padding box, which padding sits inside | an `.absolute()` child with all four insets zero | invisible (§6) |
| `absolute inset-y-0 my-auto` on the `leftIcon` | vertically centred out of flow | `.absolute().top(0).bottom(0).my_auto()` | invisible (unanchored) |
| `border-radius: inherit` | the parent's *computed* radius | the same sealed token read twice, never `px(10.0)` | compared |
| `[transition:background-color_5000000s_ease-in-out_0s]` | the WebKit autofill-suppression hack | **absent.** Not a visual property at any instant §6 can see | absent |

---

## 5. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `::placeholder` | gpui has no pseudo-elements | **resolved into the run being painted.** The port paints one string — the value or the placeholder — and picks the colour from which one it is. Costs nothing: no anchor here reports an `fg` |
| the **caret** and the **selection** | the user agent draws both; gpui's `div` is not an editor | **absent**, and §2 records them rather than approximating them. Painting a caret would put a shape on screen for the oracle to converge on, which is the call every icon in this port has already refused |
| `unstyled` | strips the control's whole class list, leaving a bare `<span>` at **`display: inline`** — and gpui has no inline flow at all | **absent.** Its one consumer, `SidebarHeaderSearch` in `components/ui/sidebar.tsx`, is exported and **never rendered**, so the arm has no reference either. See the traps for what that call site would look like if it were |
| `not-dark:bg-clip-padding` | `background-clip: padding-box` | **absent.** gpui has no background-clip, and it changes no pixel while the border is opaque |
| `has-autofill:bg-foreground/4`, `dark:has-autofill:bg-foreground/8` | WebKit paints autofill from the platform | **absent.** No cell can reach it |
| `not-has-disabled:…:before:shadow-[0_1px_--theme(--color-black/4%)]` and the dark `[0_-1px_--theme(--color-white/6%)]` | inset shadows in Tailwind's own black and white | **not painted.** §6 has no shadow field, and `check-invariants.sh` rule 4 will not let a component mint either literal — `Theme::LIGHT.card` is the one door open for white and there is none for black. Measured live: the overlay's computed `box-shadow` in dark is `oklab(1 0 0 / 0.06) 0 -1px 0 0` |
| `outline-none` | `outline-style: none` | **absent.** No outline field, and gpui paints none to suppress |
| `transition-shadow` | a curve | **absent.** §6: a snapshot is one instant. And nothing geometric transitions, so **v1.9's timing hole does not reach this surface** — `button` is the other control for it |
| the `type="search"` and `type="file"` arms | `-webkit-search-cancel-button` and `::file-selector-button` are UA shadow parts | **absent.** Neither has a DOM node, neither can carry an anchor, and no live `<Input` passes either type |
| `size={number}` | the `<input size>` **attribute** — a character count | **absent**, and it is a *trap* rather than an omission. See §8 |

---

## 6. Painted but invisible to the oracle

`ANCHORS.md` §6 has no field for any of these. **Say so in any report.**

| React / Tailwind | gpui | Note |
|---|---|---|
| `shadow-xs/5` | `.shadow_xs()` | gpui's preset is **byte-identical** to what the app compiles — `0 1px 2px 0` at `rgb(0 0 0 / .05)` — which is the `dropdown-menu` `shadow-md` situation rather than the `tabs` one, where `shadow-black/10` needed an alpha no preset carries. Reaching for the preset also settles rule 4: the literal lives inside gpui |
| `has-focus-visible:ring-[3px] ring-ring/24` | one `BoxShadow` through `Styled::style` | **no offset layer**, unlike `button`'s: `input.tsx` writes no `ring-offset-*`, so `--tw-ring-offset-width` stays at its `0px` initial and the offset layer keeps its `0 0 #0000` initial — `resizable`'s reading of the same construct |
| the `::before` overlay | an `.absolute()` child with the 9px radius | **unanchored, deliberately.** §3's shortcut is *legal* here — the pseudo really is `position:absolute; inset:0` — and taking it would still be wrong, because a pseudo-backed anchor **replaces** the host's record and would throw away the control's background, its 1px border and its 10px radius. `button` reached the same place by the same door; measured numbers are in `Input::overlay`'s doc comment |
| `has-disabled:opacity-64` | `.opacity(0.64)` | v1.7's `visible` term fires **only at zero**, so a disabled field is `visible: true` on both sides and the 36% is a difference neither extractor can report |
| the `leftIcon` box | an empty absolute box | a component a call site passes; unanchorable on the reference side |
| the painted string, its colour, its font and its advance | a plain unanchored child | §1 — **the whole point of this component** |

---

## 7. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | `input-control` on the `<span>` and `input` on the field, mirroring the two `data-slot`s |
| where they are written | the field's is a **JSX attribute placed before `{...props}`**, so a call site can override it — P2.1's convention. P3.1's *object-property* shape does **not** recur here: `input.tsx` renders `<InputPrimitive …/>` as JSX and never builds a props object, unlike `button.tsx`'s `useRender`. Both the `nativeInput` and the base-ui branch take the same spelling |
| the control's id | **cannot be overridden**, and that is a difference from every earlier component: `input.tsx` destructures `className` out and spreads `{...props}` onto the *field* only, so nothing reaches the `<span>`. Recorded rather than fixed — fixing it would be more than an attribute |
| the root | **`input-control`**. It has to be the outer element: §4 puts every bound relative to it, and the field's `21,1` is the whole geometry of the surface |
| the `leftIcon` | **not anchored.** It is a component a call site chooses. `<LeftIcon>` does forward unknown props to its `<svg>`, so an attribute would *work* — and every icon in this port is an unanchored empty box, and one exception would be a shape for the oracle to converge on |
| the `::before` overlay | **not anchored** — see §6 |
| a surface scope declaration (v1.8) | **none, and none needed.** The root's subtree contains the field and, at three call sites, an icon that carries no anchor. Two anchors, each exactly once, in every cell. `oracleSurfaceScope` declares only `resizable` and `sidebar-carousel` and this item may not edit it anyway |
| `CONTENT_SIZED` | **empty**, and not a judgement call: both anchors are `w-full`. An `<input>`'s intrinsic width is its `size` attribute's character count, which `w-full` overrides |
| `LINE_SIZED` | **empty**, and it is the **closest call in the port so far** — see §8 |

---

## 8. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **Recording the value or the placeholder as an anchor's text.** | §1. The field really does paint its own string and the DOM extractor still emits none, because an `<input>` has no text node. Five `FieldPresence` deltas per cell — the `git-row-badge` shape with the sides swapped. This is the trap the whole item exists for, and it is *more* tempting than `tabs`'s because here the element genuinely owns the string |
| **Declaring the field `line_sized` because its height equals its line height.** | It **does** equal it — `h-6.5` is 26 and `leading-6.5` is 26, for all three sizes at both breakpoints — and it is still wrong, for a **third** reason on top of v1.6's two. `git-row-badge`'s box is authored and its line box is a different number; `tabs`'s tab is authored and contains someone else's text; **here the reference emits no `font` at all**, so v1.6's `bounds.h` against `reference.font.line_height` has nothing on the other side. Two authored declarations that agree is not a height *derived from* a line box. Mutation-tested: **2 failures**, §11 |
| **Skipping the control's border because the field "has no visible border".** | The bare `border` in the control's class list is unconditional; the variants change only the colour. `border.w` is compared **exactly**. `button`'s trap, in a second place, and this component carries **both** halves of it — the field's border really is `0`, so a port that put one on it would be equally wrong |
| **`ring-[3px]` is not a border.** | Tailwind 4 compiles it to `--tw-ring-shadow: 0 0 0 calc(3px + …)`, a box-shadow. A focused control's `border.w` is still `1`, never `4`. `MAPPING.md` records this for `ring-1` twice and `tabs` for `ring-2`; this is the third width |
| **`px-[calc(--spacing(2.5)-1px)]` is not `px-2.5`.** | The pixel comes off the **field's** padding to pay for a border on the **control** — a different element. 9 and 11, not 10 and 12. Mutation-tested: **1 failure** |
| **Reading a call site's `className` as the field's styling.** | It lands on the **control**. `SidebarHeaderSearch` writes `className="h-6 min-w-0 w-full rounded-md px-2 ps-7 text-sm bg-transparent border-transparent outline-none"` — every one of which reads like input styling and every one of which lands on the `<span>`. With `unstyled` that span is `display: inline`, so `h-6` and `w-full` are inert, and the `h-6.5` field inside it overflows a 24px wrapper by two pixels. A real defect, in dead code |
| **Taking `size={number}` for a fourth size.** | It is the HTML `<input size>` **attribute** — a character count — which `w-full` overrides to nothing, and the class list falls into the `default` arm because `size === 'sm'` is false. A numeric size *is* `default` plus an inert attribute. `--size 8` is a rejection that says so |
| **Reading `size-8 sm:size-7`-style pairs as one step.** | Every `sm:` variant on this component is exactly **one `--spacing` step** smaller, on the height and on the leading alike. That is a fact worth writing once (`SM_STEP_DROP`) rather than six literals that can drift |
| **`has-focus-visible:` beating `has-aria-invalid:`.** | Both are `(0,2,0)` — `:has()` takes its argument's specificity — so **source order decides**, and the compiled sheet emits `border-color: var(--ring)` at line 315 and `has-aria-invalid`'s at 331. **Invalid wins.** The doubly-qualified `/64` rule at `(0,3,0)` beats both |
| **`dark:has-aria-invalid:ring-destructive/24` losing to the focused rule.** | It does not. `.dark .cls:has([aria-invalid])` is also `(0,3,0)`, and Tailwind emits `dark:` variants last — so in the dark table the ring is 24%, not 16%. Invisible either way (it is a shadow), and recorded so a reader does not have to re-derive it |
| **Anchoring the `::before` overlay.** | `button`'s trap verbatim: legal under §3 and still wrong, because a pseudo-backed anchor *replaces* the host's record |
| **Expecting `scrollWidth > clientWidth` to detect an overflowing value.** | It does not. Measured live with `Search` in the field: `224 / 224`. A text input scrolls its own content, so the two track each other |
| **Expecting a programmatic `.focus()` to reach `:focus-visible`.** | It sets `document.activeElement` and nothing else. `document.hasFocus()` is **false** on this machine and stays false through `window.focus()` **and** Tauri's `getCurrentWindow().setFocus()`, so `:focus` never matches — and `:focus-visible` cannot match without it. Measured, both ways |
| **Capturing without switching the sidebar to Files.** | The live `<Input>` lives in the file-explorer panel of `sidebar-carousel`, and a capture taken on the Workspaces tab reports **`visible: false` on both anchors** with the geometry otherwise correct. §10 |

---

## 9. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it.

- **Half the contract has no reference and never will on this surface.** `fg`,
  `text`, `text_width`, `clipped` and the whole `font` group are unreachable on
  *any* cell, in *any* state, at *any* width — §1. This is different in kind from
  `button`'s "no live Button paints text": that is a fact about the fixture
  workspace and a labelled Button would fix it. Here the element is structurally
  incapable of reporting text.
- **`--content` is vacuous on every cell**, and follows directly. The caption
  says so unconditionally rather than only where a string was driven.
- **The field's `border.color` is never compared.** v1.3 made the colour
  compared only when `w > 0`, and the field's `w` is `0` in every cell. The
  reference emits `#ffffff0f` and the differ never looks at it.
- **`focus` moves a compared field and is unreachable.** This is the second time
  in the port an interaction flag reaches a compared field — `button`'s `hover:bg-*`
  was the first — and the second time it cannot be driven.
  `has-focus-visible:border-ring` is a border *colour*, not a shadow. The block
  is not `CGPreflightPostEventAccess()` this time but `document.hasFocus()`,
  which is a different wall and needs a human at the keyboard rather than a
  synthetic event.
- **`aria-invalid` has no live call site.** No `<Input` in `web/src/` passes it,
  although `select`, `checkbox`, `radio-group` and `textarea` all carry the same
  four rules. So the one *other* state that moves a compared field is drawable
  here and not on the other side — the shape of finding `resizable`'s grip and
  Phase 1's `git-row-dir` have.
- **`error` is a real state and is still declared unmodelled.** `surface.rs`'s
  `no_surface_declares_its_entire_state_axis_unmodelled` asserts
  `unmodelled(Error)` for *every* registered surface, and that assertion was not
  this item's to weaken. So `aria-invalid` is driven by `--invalid`, exactly as
  `button`'s `loading` branch is driven by `--loading`. **The next worker to port
  `select`, `checkbox`, `radio-group` or `textarea` will meet the same four rules
  and the same invariant**, and the honest fix is a contract change rather than
  four more surfaces working around it.
- **`hover` and `selected` are genuinely unmodelled**, and counted rather than
  assumed: `input.tsx` contains the substring `hover` **zero** times, and writes
  no `selected`, `:checked` or `data-selected` rule. A *text* selection exists but
  is the user agent's paint on no element.
- **`empty` is the resting cell.** The live reference is `value === ""`, so
  `--flags empty` is a no-op unless `--value` was given — the caption says so per
  cell, the way `tabs` says it of a `selected` cell that landed on the resting
  tab.
- **`--size lg` has no reference**, and `unstyled` has no *live* one: its only
  consumer is exported and never rendered. `--class-ps none` **does** have one —
  see §10 — and `--icon` has three call sites but none on screen in the fixture
  workspace.
- **A second `<Input>` shares the document**, so `oracleSurfaceScope` would not
  help even if this surface declared one: v1.8 is about anchors *beneath one
  root*, and these are two roots. The extractor's `index` is what chooses, and a
  capture that forgot it would silently take the tree filter — which is right by
  luck rather than by construction.
- **The control's id cannot be overridden by a call site**, unlike every other
  primitive's in this port. Nothing is spread onto the `<span>`.

---

## 10. The reference capture

`/tmp/p3-ref-input.json`, taken off the running app (pid 64880, bridge 9223) at
`innerWidth` **1714** — `theme` omitted, so it derived `dark` — rooted on
`input-control`, at rest.

```text
input-control    0,0,246,28   bg #ffffff07  r 10  border 1 #ffffff14  visible true
input           21,1,224,26   bg #00000000  r 10  border 0            visible true
```

Two anchors, each exactly once — v1.8 satisfied without a declaration. Neither
carries a single field from the text group, which is §1 in one line.

**The sidebar has to be on the Files tab.** The live `<Input>` is the tree filter
inside `sidebar-carousel`'s file-explorer panel, and the carousel had it snapped
out: the first capture came back geometrically perfect and `visible: false` on
both anchors, because the panel sits at `x 596..842` against a scrollport of
`0..294`. Switching the tab moved the control to `x 8` and both anchors to
`visible: true` with **identical bounds**. That is `sidebar-carousel`'s §7
"capture precondition" finding meeting a second surface, and it is worth knowing
that the failure is *silent in the geometry* — only `visible` moved.

The tab was switched from JavaScript (`PointerEvent` + `.click()` on the tab
button), which works because it is a DOM dispatch rather than a `CGEvent`; the
app was left on Workspaces afterwards, as it was found.

The bundle the app was serving predates the `data-oracle-id` edit, which lives in
a worktree the dev server does not serve. The two attributes were therefore
applied to the live DOM with `setAttribute` immediately before the extract and
removed immediately after — P3.2's arrangement. The snapshot was written to disk
**byte-exact through a local HTTP sink** (`Bun.serve` on an ephemeral port, body
straight to `writeFileSync`), so nothing round-tripped through the bridge's JSON.

`--width 246 --viewport-width 1714` is the cell that reproduces it, and
`row_layout::input` asserts every number above from taffy's own layout.

### The second live `<Input>`, and why it is archived too

There are **two** `[data-slot=input-control]` in the live document, and the
second is the reference for `--class-ps none`:

```text
input-control    0,0,454.42,28   bg #ffffff07  r 10  border 1 #ffffff14  visible true
input             1,1,452.42,26  bg #00000000  r 10  border 0            visible true
```

`/tmp/p3-ref-input-unpadded.json`, index **1**, same run and same sink. It is the
git review reply box in `features/git/components/review-thread-item.tsx`
(`size="sm" className="flex-1 cursor-pointer"`, placeholder `Reply…`, empty), and
it matters for two reasons:

* **`--class-ps none` is referenced**, and *visibly* — unlike `button`'s
  equivalent finding, where the only two Buttons keeping the primitive's own
  radius are clipped out by the carousel at `visible: false`. Here the field's
  `x` is exactly `1` — one border pixel — which is the number
  `row_layout::input::the_call_sites_leading_pad_moves_the_field` asserts.
* **Its width is fractional.** `454.421875` comes out of a `flex-1` division, and
  taffy rounds a panel's width to a whole logical pixel where WebKit does not —
  `resizable`'s §5 trap. §5's ±0.5 covers it, but `--width 454` against 454.42 is
  a cell to pick carefully rather than drive blind.

Both were captured with the extractor's `index` option (0 and 1) against the same
pair of attributes, applied to **every** matching element and removed
immediately after.

---

## 11. Mutation results

Each applied to the component, run with `--no-fail-fast`, reverted; the control
after every revert is **0 failures over 866 tests**.

| Mutation | Failures |
|---|---|
| the fixture drops the call site's `ps-5` (`LeadingPad::None`) | **4** |
| the control skips its `border` | **4** |
| `Size::padding_x` drops the `-1px` | **1** |
| the field's string goes through `AnchorSink::boxed_text` | **1** |
| the field declares itself `line_sized` | **2** |
| `Input::background` ignores the dark table | **2** |

The `boxed_text` row is the one worth reading: it is the mistake §1 exists to
prevent, and exactly **one** assertion catches it —
`row_layout::input::no_anchor_on_this_surface_reports_text`, which is there for
no other reason.

---

## 12. Cross-component notes added by this component

Things learned here that are **not** about `input`.

| Note | |
|---|---|
| **A void element has no text group, on either side** | `oracleOwnText` walks `childNodes`, so `<input>`, `<textarea>`'s value, `<img>`'s alt and `<select>`'s options are all invisible to the contract. Any primitive over a form control is a **box-only** surface, and the port must not record text for one. This is the single most reusable fact here: `textarea`, `select`, `checkbox` and `radio-group` are all next |
| **`:has()` takes its argument's specificity**, so `has-x:` variants tie with each other | which means Tailwind's *emission order* decides between them, and `dark:` is emitted last. Compile the sheet and read the line numbers; do not count qualifiers |
| **Tailwind's preflight `font: inherit` on form controls is load-bearing** | a font declared on a *wrapper* reaches an `<input>` only because of it, and it appears in no class list. Any port of a form primitive has to read the preflight, not just the `cn(…)` |
| **`ps-*` and `px-*` both survive tailwind-merge** | and the compiled sheet emits every `padding-inline-start` after every `padding-inline`, so the logical-start utility wins. A port that dropped one would be right by accident on one side of the box |
| **gpui's `shadow_xs()` is Tailwind's `shadow-xs` exactly** | including `rgb(0 0 0 / .05)`. `dropdown-menu` found the same of `shadow_md`. `tabs` found the opposite for `shadow-black/10` — the presets match Tailwind's *defaults*, so a `shadow-<color>/<alpha>` override is what breaks them |
| **A `visible: false` capture can be geometrically perfect** | `sidebar-carousel`'s snapped-out panels clip anything inside them, and only the `visible` column shows it. Check `visible` on the reference before believing a converged geometry — and before disbelieving one |
| **`document.hasFocus()` is the wall, not `CGPreflightPostEventAccess()`** | for any `:focus-visible` cell. A programmatic `.focus()` sets `document.activeElement` and nothing more, and neither `window.focus()` nor Tauri's `setFocus()` makes the webview's document focused from a bridge session. Every `focus-visible:` rule in the app is therefore unreachable to an agent, whatever the pointer situation |
| **A workspace invariant can force a dishonest declaration** | `surface.rs` requires `unmodelled(Error)` on every surface, and `aria-invalid` is a real `error` state. The workaround — a surface option, as `button` did for `loading` — is fine once and is now twice; it is worth deciding rather than repeating |
