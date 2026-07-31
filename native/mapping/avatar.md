# `avatar` (P3.3)

`web/src/components/ui/avatar.tsx` →
`crates/crowbar-ui/src/components/avatar.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

Three thin wrappers over `@base-ui/react` 1.6.0's `Avatar`. Unlike `resizable`,
the library writes **no inline styles** on any of the three — read
`node_modules/@base-ui/react/avatar/{root,image,fallback}/*.mjs` and there is
not a `style` object among them — so here the `cn(…)` call really is what
renders. What the library owns instead is **which elements exist at all**, and
that is the whole of this row.

Every "Compiles to" below came from running the app's own `src/index.css`
through its own `tailwindcss` 4.3.0 with the utility as a candidate.

## 0. The headline: the port's first image primitive, and the anchor set is a state

`AvatarImage` computes `isVisible = imageLoadingStatus === 'loaded'`, feeds it
through `useTransitionStatus`, and then:

```js
if (!mounted) { return null }
```

`AvatarFallback` is the mirror: `useRenderElement(…, { enabled:
imageLoadingStatus !== 'loaded' })`. So **the `<img>` does not exist in the
document until the bytes arrive**, and the fallback stops existing the moment
they do. Exactly one of the two is ever in the tree.

For the contract that is a shape no earlier surface had: **the two states differ
in their anchor set, not in a field.** An unloaded avatar has no `avatar-image`
for the differ to report `visible: false` on — the anchor is simply absent, which
`ANCHORS.md` ranks first. So a cell has to be driven to the same status on both
sides or the comparison is between two different documents. `--image` is the
option that says which.

**Both states were captured live**, from the same review thread in the fixture
workspace, which is what makes this a measurement:

| message | anchors | notes |
|---|---|---|
| `char2cs`'s (the reference) | `avatar`, `avatar-image` | `src` resolves through a GitHub redirect to `avatars.githubusercontent.com`; `naturalWidth` 460 |
| the agent's | `avatar`, `avatar-fallback` | `avatarUrl` is `null`, so `<AvatarImage>` is not rendered at all: `AG`, 24×24, `bg-muted`, fg `#a4a4a4ff`, 12px/600 |

**Which one the live app shows: the image.** The reference's state therefore
**depends on the network**, which is worth saying out loud — it is the first
quantity in this port that is neither a class nor a prop.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-8` (root) | `width`/`height` = `calc(--spacing * 8)` = **32px** | `SIZE_DEFAULT` | compared |
| `size-6` (`review-thread-item.tsx`) | **24px** | `SIZE_MESSAGE` | compared |
| `size-14` (`repo-icon-popover.tsx`) | **56px** | `SIZE_REPO_ICON` | compared |
| `size-full` (image, fallback) | `width: 100%; height: 100%` | `.w_full().h_full()` | compared |
| `rounded-full` | `border-radius: calc(infinity * 1px)` → **`f32::MAX`** in `WebKit` | `FULL_RADIUS = px(f32::MAX)` — **not** `rounded_full()`, see the traps | compared |
| `rounded-xl` (`repo-icon`) | `calc(var(--radius) * 1.4)` = **14px**, *not* Tailwind's stock 12 | `theme.radius_xl.value()` | compared |
| `bg-background` (root) | `var(--background)` | `theme.background` | compared |
| `bg-muted` (fallback) | `var(--muted)` | `theme.muted` | compared |
| `text-xs` | `--text-xs: 0.75rem` on `calc(1 / 0.75)` → **12px on 16px**, measured live | `theme.ui_text_sm.value()` + `relative(1.0/0.75)` | compared |
| `text-base` (`repo-icon`) | `1rem` on `calc(1.5 / 1)` | `theme.ui_text_lg.value()` + `relative(1.5)` | compared |
| `font-medium` / `font-semibold` / `font-bold` | 500 / 600 / 700 | `FontWeight::{MEDIUM,SEMIBOLD,BOLD}` | compared |
| `text-muted-foreground` (fallback, call site) | `var(--muted-foreground)` | `theme.muted_foreground` | compared |
| `mt-0.5` (`review-thread-item.tsx`) | `margin-top: 2px` | `.mt(MESSAGE_MARGIN_TOP)` | **absent from the snapshot** — §4 reports geometry relative to the root, and the root is the element the margin is on |

**There is not one `sm:` variant in `avatar.tsx`** — the first component in the
port with none. So `--viewport-width` is vacuous here, which is unusual enough to
state.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` (root) | `display: inline-flex` | `.flex()` — the reference's own computed `display` is **`flex`**, because CSS blockifies a flex item. **Measured** | compared |
| `flex` (fallback) | `display: flex` | `.flex()` | compared |
| `shrink-0` (call site) | `flex-shrink: 0` | `.flex_shrink_0()` | compared |
| `items-center justify-center` | both axes centred | the same two builders | compared |
| `overflow-hidden` (root) | `overflow: hidden` | `.overflow_hidden()` — what clips an overflowing fallback | compared via `visible` |
| `align-middle` | `vertical-align: middle` | **nothing.** gpui has no inline flow, and the property applies to inline-level boxes; the reference's own element is a blockified flex item, on which it is inert | absent |
| `select-none` | `user-select: none` | **nothing.** Not a visual property | absent |
| `object-cover` (image) | `object-fit: cover` | **nothing.** It decides which crop of a bitmap fills the box, and the box is what the contract measures | absent |
| the `<img>` itself | a network photograph | an **empty box** — the same call every component since `git_status_row` made about icons, one step further out. The reference agrees in the strongest form it has: `bg #00000000`, `radius 0`, `border.w 0` | compared |

## 3. No gpui equivalent / not ported

| React | Why | What the port does |
|---|---|---|
| `useImageLoadingStatus`, `useTransitionStatus`, `useOpenChangeComplete` | a network fetch and a transition | **modelled as a parameter** ([`ImageStatus`]), not reproduced. The port renders one status; it does not own one |
| `AvatarFallback`'s `delay` prop | a timer that holds the fallback back for N ms | **absent.** No live call site passes it, and §6 puts a timer outside a snapshot |
| `repo-icon-popover.tsx`'s `repo.avatarColor` fallback | a Tailwind `bg-*` **class chosen per repository and stored in the workspace record** | **not modelled.** A port cannot resolve a class name that arrives as data, and inventing one would put a colour on screen for a perceptual oracle to converge on. Recorded rather than approximated |
| the emoji fallback branch (`repo.avatarURL.slice(6)`) | a text child that is an emoji | **absent.** Same reason: the content is data |

## 4. Painted but invisible to the oracle

**Nothing.** There is no shadow, no ring, no transition and no opacity on this
component — `avatar.tsx` has no interaction rule of any kind. Every field it
paints is one the contract compares, which is unusual and is the reason its
`unmodelled` list is as long as it is.

## 5. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | `avatar`, `avatar-image`, `avatar-fallback`, each written *before* `{...props}` so a call site can override it — P2.1's convention. Plain JSX attributes here, unlike `badge.tsx`, because these three are `<AvatarPrimitive.*>` elements rather than a `useRender` props bag |
| the root | `avatar`. It has to be: §4 puts the origin on it, and both children are `size-full` of it |
| scope | **none needed.** The root's subtree contains only this surface's own anchors, so v1.8's declared-set mechanism does not apply — the same as `git-status-row` and unlike `resizable` |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty**, and empty *lists* rather than absent, for P2.1's reason. Every box here is `size-*` or `size-full`; nothing takes its size from what it contains |

## 6. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **`rounded-full` is not gpui's `rounded_full()`.** | Tailwind 4 compiles it to `border-radius: calc(infinity * 1px)`, and `WebKit` resolves that to the largest single-precision float — `getComputedStyle().borderTopLeftRadius` on the live avatar is `"340282346638528859811704183484516925440px"`, which is exactly `f32::MAX`. gpui's preset is `px(9999.)`. Both paint the same circle, because every renderer clamps a corner radius to half the box, and `radius` is a **compared field at ±0.5**: 9999 against 3.4e38. `px(f32::MAX)` is not a fudge — `f32::MAX` widened to `f64` is the same integer `WebKit` printed, and both extractors' round-to-a-thousandth leaves it unchanged, so the two agree **exactly**. Verified against the captured reference |
| **Expecting the unloaded image to be a hidden `<img>`.** | It is `return null`. There is no element, so there is no anchor — a `FieldPresence`-class delta rather than a `visible: false` one, and the loudest thing the differ has |
| **Treating `loading` and `error` as different pictures.** | `AvatarFallback` asks only `imageLoadingStatus !== 'loaded'`, so `idle`, `loading` and `error` render identically. The port's [`ImageStatus`] has three arms and only two pictures, and the third (`Absent`) is a *call-site* fact — `review-thread-item.tsx` writes `{display.avatarUrl && <AvatarImage …>}`, so an agent message has no image element to load |
| **Declaring the fallback `line_sized`.** | It paints text and has no `h-*`, which is exactly the shape v1.6's detector would fire on. Its height is `size-full` of an authored square: measured **24** against a **16px** line box, so declaring it would invent 8px |
| **Declaring the fallback `content_sized`.** | Same reason from the other axis: `size-full` is 100% of the parent, not the run's max-content width. The live pair is a 24px box around a 17.33px run |
| **Anchoring the root and expecting `overflow-hidden` to clip it.** | It clips the root's *children*, not the root — the finding P2.3 recorded for `sidebar-carousel`, and it holds here for the same two mechanisms |
| **`align-middle` doing something.** | `vertical-align` applies to inline-level and table-cell boxes. Every live `<Avatar>` is a flex item, which CSS blockifies, so the declaration is inert in the reference too |
| **Assuming the reference is deterministic.** | It is not. Whether `avatar-image` exists depends on a fetch from `avatars.githubusercontent.com` completing. A capture taken offline is a *different cell*, not a failed one, and it is a cell the port can draw — `--image pending` |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it.

- **Two of the four §8.3 axes are vacuous.** `--width` cannot move an anchor:
  every box is `size-*` or `size-full`. `--viewport-width` cannot either:
  `avatar.tsx` contains no `sm:` variant at all. `--content` is real **only with
  a fallback**, which the default cell does not show.
- **Four of the six state flags are unmodelled, and one is nearly a lie.**
  `hover`, `focus` and `selected` are unmodelled because the component has no
  rule for any of them. `loading` is unmodelled in the sense
  [`Surface::unmodelled`] means — driving `--flags loading` reaches nothing here,
  so the cell cannot fail — **while the state it names is real, reachable and
  captured.** It is driven by `--image` instead, the arrangement `button` uses
  for its `loading` prop and `resizable` for `--with-handle`. Unlike either of
  those, this branch has a live reference. Said plainly rather than left to the
  word.
- **`empty` has no reference.** A root with neither child is a real branch of the
  primitive — `<Avatar>` takes its children from the call site — and both live
  call sites always pass a fallback.
- **The `repo-icon` call site is live and uncapturable.** It is inside a
  `PopoverContent`, a portal that exists only while the popover is open, and
  synthetic pointer events are denied on this project's machines. It is also
  **the only live avatar with a finite radius**, so the one cell that would
  exercise `rounded-xl` against a reference is unreachable. The same shape of
  finding as `resizable`'s grip.
- **The primitive's own `size-8` has no reference either.** Neither live call
  site leaves the `className` alone.
- **`object-fit` is unfalsifiable.** The port draws no bitmap, so nothing on
  either side can disagree about the crop. The box is what is compared, and the
  box is `size-full` either way.

## 8. Cross-component notes added by this component

Things learned here that are **not** about `avatar`.

| Note | |
|---|---|
| `calc(infinity * 1px)` is `f32::MAX`, and it is *comparable* | any `rounded-full` in the remaining primitives ports as `px(f32::MAX)`, not as gpui's `rounded_full()`. The two extractors then agree exactly, because both round-trips are identities at that magnitude |
| A `base-ui` primitive can decide **which elements exist**, not just how they look | `AvatarImage`/`AvatarFallback` are the first case. For any primitive over a library, `git grep 'return null'` in its `.mjs` is worth doing before assuming a state is a style |
| A reference can depend on the network | and when it does, the surface needs an option that says which side of the fetch the cell is on. Otherwise a capture taken on a bad connection is silently a different cell |
| `vertical-align` is inert on every flex item | so `align-middle`, `align-top` and friends port to nothing wherever the element is a flex or grid item — which, in this app, is almost everywhere |
