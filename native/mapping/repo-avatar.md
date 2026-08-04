# `repo-avatar` (P3.50)

`web/src/components/layout/repo-avatar.tsx` →
`crates/crowbar-ui/src/components/repo_avatar.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> for the reason `avatar.md` gives.

One `components/layout` foundation leaf, one of P3.50's two: a repo's
identity mark, next to a repo or workspace name in the sidebar, the New Tab
view, and elsewhere. Unlike `avatar.tsx`, this file is not a thin wrapper
over a `@base-ui/react` primitive — it is hand-authored JSX with its own
three-way branch.

Every "Compiles to" below came from running the app's own `web/src/index.css`
through its own `tailwindcss` 4.3.0 with the utility as a candidate.

## 0. The headline: no persistent wrapper, and a datum this port cannot resolve

`RepoAvatar` renders exactly **one** `<span>` or `<img>` per call —
`avatar.url?.startsWith('emoji:')` picks the emoji span, `avatar.url` picks
`RepoAvatarImg` (an `<img>` or, once `onError` fires, the letter span),
neither picks the letter span directly. There is no outer element any of the
three sits inside: unlike `avatar.tsx`'s `AvatarPrimitive.Root`, which stays
mounted while `AvatarImage`/`AvatarFallback` swap underneath it, this
component's root **is** whichever leaf is live. So the port's single anchor,
[`repo_avatar::ID`], is shared by all three pictures rather than naming a
wrapper — there is no wrapper to name.

The second finding is `avatar.color`: the letter fallback's background is a
Tailwind `bg-*` class chosen per repository and **stored as data** in the
workspace record (`repo.avatarColor`), threaded through unmodified by every
call site. `avatar.md` already ruled this shape of value unmodelled for
`repo-icon-popover.tsx`'s equivalent fallback, because it sits behind an
unreachable popover. Here the letter fallback is the **dominant, live**
path — most repos have no icon URL — so the port cannot omit the branch the
way `avatar.md` did. It takes the resolved colour as a caller-supplied
[`Color`] instead of computing one, and the fixture uses `theme.primary` as
a placeholder with no claim to being any repository's actual colour. The
values observed across the test suite include real Tailwind classes
(`bg-indigo-700`, `bg-red-500`) *and* strings that are not Tailwind classes
at all (`avatar-rose`, `avatar-pink`) — a real repo assigned one of the
latter renders no background at all in the live app, a fact the port cannot
reproduce or improve on.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `h-4 w-4` (`sm`) | **16px** | `Size::Sm.extent()` | no reference — see §7 |
| `h-5 w-5` (`lg`) | **20px** | `Size::Lg.extent()` | no reference |
| `h-6 w-6` (`xl`) | **24px** | `Size::Xl.extent()` | no reference |
| `text-[10px]` (letter, `sm`) | literal, no token | `LETTER_TEXT_SM = px(10.0)` | no reference |
| `text-[11px]` (letter, `lg`) | literal, no token | `LETTER_TEXT_LG = px(11.0)` | no reference |
| `text-[13px]` (letter, `xl`) | literal, no token | `LETTER_TEXT_XL = px(13.0)` | no reference |
| `text-xs` (emoji, `sm`) | `0.75rem` on `calc(1/0.75)` → 12px on 16px | `theme.ui_text_sm` — the trade `dropdown-menu.md` establishes | no reference |
| `text-sm` (emoji, `lg`) | `0.875rem` on `calc(1/0.875)` → 14px on 16px | `theme.ui_text_base` | no reference |
| `text-lg` (emoji, `xl`) | `1.125rem`, **no `--ui-text-*` token carries this number** | raw literal, `EMOJI_TEXT_LG_REM = 1.125` | no reference |
| `font-bold` (letter) | `700` | `FontWeight::BOLD` — distinct from `avatar.tsx`'s message-avatar `semibold` (600) | no reference |
| `rounded-sm` (letter, image) | `calc(var(--radius) - 4px)` | `theme.radius_sm.value()` | no reference |
| `text-primary-foreground` (letter) | `var(--primary-foreground)` | `theme.primary_foreground` | no reference |
| `px-0.5` (letter) | `padding-inline: 2px` | `.px(px(SPACING * 0.5))` | no reference |
| `avatar.color` (letter background) | a Tailwind `bg-*` class **stored as data** | caller-supplied [`Color`]; fixture uses `theme.primary` as a placeholder | **unresolvable** — see §0, §3 |

**There is no `sm:` variant anywhere in `repo-avatar.tsx`**, the same finding
`avatar.md` records for `avatar.tsx` — so `--viewport-width` is vacuous here
too.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` (emoji, letter) | `display: inline-flex` | `.flex()` — blockified the way `avatar.md` measures | no reference |
| `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` | no reference |
| `items-center justify-center` (emoji, letter) | both axes centred | the same two builders | no reference |
| `leading-none` (emoji) | `line-height: 1` | `.line_height(relative(1.0))` | no reference |
| `object-cover` (image) | `object-fit: cover` | **nothing.** Decides a bitmap's crop; the port draws no bitmap | absent |
| the fetched `<img>` itself | a network photograph | an **empty box** — the same call `avatar.rs`'s `.image()` makes, one step further: this box additionally carries its own `rounded-sm`, where `avatar.tsx`'s image has none (radius 0) — a real, measured shape difference between the two ports | no reference |

## 3. No gpui equivalent / not ported

| React | Why | What the port does |
|---|---|---|
| `avatar.color` | a Tailwind `bg-*` class chosen per repository and stored as data — see §0 | **not resolved.** Taken as a caller-supplied [`Color`]; the fixture is a placeholder, not a captured value |
| the emoji glyph's own codepoint (`avatar.url.slice(6)`) | data, not a class or prop | modelled as a caller-supplied string, same treatment `avatar.md` gives `repo.avatarURL.slice(6)` |
| `useState`'s `errored` flag (`RepoAvatarImgAttempt`) | local React state, not a prop | modelled as an input the caller drives — [`ImageState`] — the way `avatar.rs` models `ImageStatus` |
| `key={props.src}`'s remount-on-new-`src` reset | a React reconciliation mechanic | **not applicable.** The port has no mount lifecycle to reset; [`ImageState`] is supplied fresh per cell |

## 4. Painted but invisible to the oracle

**Nothing beyond the fetched image**, which §2 already records as an empty
box. `repo-avatar.tsx` carries no shadow, no ring, no transition, no opacity,
and (per the module docs) no interaction rule of any kind.

## 5. Anchoring

| Construct | Decision |
|---|---|
| the root | [`repo_avatar::ID`], `"repo-avatar"` — shared by all three pictures, since none of them sits inside a persistent wrapper. See §0 |
| scope | not applicable in the same sense `avatar.md` means it: there is exactly one anchor on this surface, ever, whichever branch renders |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty.** Every box is `h-*`/`w-*` (`sizeClasses`); nothing takes its size from what it contains. `repo-avatar.tsx` also carries no `overflow-hidden`, so a label longer than its box **overflows** rather than clipping — see the traps below |
| `data-oracle-id` | **absent.** Checked, not assumed: no element in this file, in any branch, and no call site that renders it, carries one. See §7 |

## 6. Traps

| Trap | What actually happens |
|---|---|
| **Expecting the letter box to clip an overflowing label.** | `repo-avatar.tsx` has no `overflow-hidden` anywhere. A label longer than its box (there is no length limit on `avatar.label`) draws past the box's edge rather than being cut off — the one thing `--content overflow` demonstrates on this surface, even with no reference to compare it to |
| **Treating `Kind::Letter` and `Kind::Image(Errored)` as merely similar.** | They are the *same value*: `letterFallback` is one JSX expression in the source, passed as `RepoAvatarImg`'s `fallback` prop and returned directly at the bottom of `RepoAvatar`. `an_errored_image_is_pixel_identical_to_the_letter_fallback` asserts full-record equality, not just matching kinds |
| **Assuming the loaded image carries no radius, by analogy with `avatar.tsx`.** | `avatar.tsx`'s image is radius 0; this one is `rounded-sm` (`className={cn('shrink-0 rounded-sm object-cover', box)}`). Copying the neighbour's shape here is wrong |
| **Assuming `avatar.color` resolves to something.** | It is a string stored as data. Real values observed in the test suite include non-Tailwind strings (`avatar-rose`) with no matching CSS rule anywhere in the stylesheets — the live app renders **no background at all** for those repositories, a picture the port has no way to distinguish from "background not yet resolved" |
| **Reaching for `md` between `sm` and `lg`.** | There is none. `sizeClasses`'s keys are `sm`/`lg`/`xl`, and the option's closed vocabulary matches that exactly |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it.

- **No live oracle capture exists for this surface at all.** `repo-avatar.tsx`
  carries no `data-oracle-id` anywhere, on any element, in any branch, and
  neither does any of its four call sites. This is not the situation
  `avatar.tsx` was in (ids come from the `@base-ui/react` primitive itself)
  or `popover`/`select`'s (a dedicated prerequisite, P3.15, landed ids on the
  React source before P3.18 declared their anchor sets). Every value in §1 is
  marked "no reference" for this reason, not because it was measured and
  found unimportant. A follow-up prerequisite — deciding where
  `data-oracle-id="repo-avatar"` goes across the emoji span, the `<img>`, and
  the letter span (used from two branches), the same shape of decision P3.15
  made its own commit — is required before a parity run can reach this
  surface at all.
- **`avatar.color` has no reference of any kind, live or hypothetical.** It
  is data stored per repository, not a class or prop a fixture can represent
  faithfully; see §0 and §3.
- **`--width` and `--viewport-width` are vacuous.** Every box is `h-*`/`w-*`;
  there is no `sm:` variant anywhere in the source.

## 8. Cross-component notes added by this component

Things learned here that are **not** about `repo-avatar`.

| Note | |
|---|---|
| A component's root can **be** the swapped element, not wrap it | `avatar.tsx`'s `AvatarPrimitive.Root` stays mounted while its children swap; `repo-avatar.tsx` has no such root — the anchor has to be shared across branches rather than fixed to a wrapper. `workspace-branch-icon.tsx` is the same shape |
| A stored-as-data class name is a recurring finding, not a one-off | `avatar.md` found it first for `repo-icon-popover.tsx`'s fallback (unreachable, so omitted); this component makes the same finding on a **dominant, live** path, which changes the answer from "omit it" to "take it as a caller-supplied value with no fixture reference" |

---

## VERDICT: FAIL — 4 deltas over 1 anchor, but only **one** is a port defect (2026-08-03)

Drive: `--surface repo-avatar --width 24 --viewport-width 1714 --theme dark
--content normal --size xl --kind letter`. The cell was found by measurement,
not assumption — `sm`/`lg`/`xl` render 16/20/24px, and the live element is
24×24 at `text-[13px]`, so `xl` is the only candidate.

Reaching this surface at all took driving: `repo-avatar` renders in the context
pill only once a **repo workspace** is open, so the sidebar tree had to be
populated first (see `oracle/blocked/four-verdicts-needed-a-repo.md`) and a
workspace row clicked.

```
repo-avatar.text:             "RE",      expected "D"        (exact)
repo-avatar.bg:               #516a36ff, expected #00000000  (Δ g +106)
repo-avatar.text_width:       15.821,    expected 9.88       (Δ +5.941, tol ±1.0)
repo-avatar.font.line_height: 21.0,      expected 19.5       (Δ +1.5,   tol ±0.5)
```

### The one real defect

**`font.line_height` — 21.0 against 19.5.** `19.5 = 13 × 1.5`: the live element
carries `text-[13px]`, an **arbitrary** size with no paired Tailwind
line-height utility, so it inherits preflight's unitless `1.5`. Confirmed
directly on the live node — `getComputedStyle` reports `fontSize: 13px`,
`lineHeight: 19.5px`. This is the **same defect class P3.60 fixed in
`row_base`**, in a different component: a per-size line height picked by hand
where the cascade supplies one.

### The other three are fixture gaps, and two were already documented

- **`text` and `text_width`** — the fixture hard-codes `"RE"`; the live repo's
  letter is `"D"`. There is no `--letter`/`--label` flag, so **no drive can
  close this**. `repo-icon-popover` hit the identical wall the same day
  (`"R"` vs `"D"`). Both fixtures need a flag.
- **`bg`** — **not a defect, and this module's own header already says so**: it
  paints `theme.primary` as an explicit placeholder "with no claim to being any
  repo's actual colour", because the daemon hands out palette names like
  `avatar-slate` and **no `.avatar-*` rule exists anywhere in the stylesheets**.
  Re-verified here: the live node carries `class="… avatar-slate"` and computes
  `background-color: rgba(0, 0, 0, 0)`. The app renders **no** background, so
  the reference's `#00000000` is correct and the fixture's colour is the thing
  that should change.

That last one is worth stating plainly: **the port implemented a feature the
React app does not have.** The daemon assigns an avatar colour, the frontend
puts it in `className`, and nothing consumes it. That is a real (small) defect
in the React app, outside this port's scope.

### ⚠ Correction to the reasoning above — "arbitrary size ⇒ 1.5" is NOT a law

Written the same hour, before briefing anyone on it. The paragraph above says
`text-[13px]` has no paired utility "so it inherits preflight's unitless 1.5".
**The measured conclusion (19.5) is right — I read it off the live node — but
that justification is over-general, and applied as a rule it produces wrong
fixes.**

The counter-example was already in hand: `context-pill`'s own
`text-[13px] font-semibold` element, which **passed** its verdict, computes

```
fontSize: 13px   lineHeight: 16.25px   ratio 1.25
```

with no `leading-*` class on the element itself. A unitless `line-height` is
inherited from the **nearest ancestor that sets one** and recomputed against
each descendant's own font-size. Preflight's `html { line-height: 1.5 }` is
only the value that survives when *nothing* between `html` and the element
overrides it — and a parent carrying any `text-*` utility (which ships a paired
line-height) or a `leading-*` class does exactly that.

So the correct rule is: **resolve the ancestor chain, or measure the live
node.** `row_base` is 1.5 because nothing in its chain overrides; this avatar
is 1.5 because nothing in its chain overrides; the context pill is 1.25 because
something does. All three are consistent, and none of them follows from the
font size alone.
