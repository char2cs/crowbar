# `skeleton` (P3.6)

`web/src/components/ui/skeleton.tsx` →
`crates/crowbar-ui/src/components/skeleton.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

A bare `<div>` with one class list, no variants, no library. Every "Compiles to"
below came from running the app's own `src/index.css` through its own
`tailwindcss` 4.3.0 with the utility as a candidate.

**Reference: none.** See §0 — this is a measurement, not an omission.

## 0. The headline: the only call site **cannot render**

`<Skeleton>` has exactly one importer,
`web/src/components/layout/sidebar-skeleton.tsx`, and `SidebarSkeleton` has
exactly one call site:

```text
sidebar-carousel.tsx:131   <Suspense fallback={<SidebarSkeleton />}>
                             <FileExplorerTree … />
                           </Suspense>
```

A `<Suspense>` boundary shows its fallback only while a descendant suspends.
`FileExplorerTree` is a **static import** with no suspending source: a grep over
`web/src` finds no `React.lazy` anywhere inside that subtree, no
`useSuspenseQuery`, and no `use()`. Every `lazy()` in the tree is under
`features/panes/`, outside this boundary.

So the fallback has nothing to wait for and never mounts. A live count of
`[data-slot="skeleton"]` was **0** in every state, including immediately after a
full page reload with the dev server freshly restarted.

This is `git-row-dir`'s precedent — rendered by the port, absent from the
product — in its **stronger** form: `git-row-dir` is absent because of the
fixture, and this is absent by construction. Nothing below is a guess; the values
are the app's compiled CSS.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `rounded-sm` | `border-radius: calc(var(--radius) * 0.6)` = **6px** | `theme.radius_sm` | `radius` — **dead at every live call site**, see §3 |
| `[background: linear-gradient(120deg, …) var(--color-muted) 0 0/200% 100% fixed]` | colour layer `var(--muted)` = `oklch(1 0 0 / 4%)`, plus a gradient **image** layer | `theme.color_muted` | `bg` = the colour layer only |
| `animate-skeleton` | `animation: skeleton 2s -1s infinite linear` over `@keyframes skeleton { to { background-position: -200% 0 } }` | `Skeleton::sweep` (the duration only) | **no field** — see §2 |
| `[--skeleton-highlight: --alpha(var(--color-white)/64%)]`, `dark:` `/4%` | the gradient's mid stop | unmodelled | no field |
| — (no size) | | the call site's | `bounds` |

## 2. `ANCHORS.md` v1.9 does **not** apply, and that was checked

The element is animated, so v1.9 — *a snapshot has no way to say when it was
taken* — is the first thing to rule out. It does not reach this component.

The only property in flight is **`background-position`**, and no field in the
contract records it:

* `bounds` are the call site's `h-*`/`w-*`; nothing in the keyframes touches
  geometry.
* `bg` is read by the React extractor as
  `oracleNormalizeColor(paint.backgroundColor)` — `web/src/lib/oracle/extract.ts`
  — which is the `background-color` layer, `var(--color-muted)`. The sweeping
  sheen is a `background-image` gradient and is **not read at all**.

So a capture of this surface would be timing-independent in every recorded field,
whether taken at rest or mid-sweep. That is a stronger statement than "captured
at rest", and it is why neither the component nor the surface carries a settling
caveat. It is also the answer to the brief's question in the form the brief asked
for: the snapshot cannot say *when* it was taken, and here it does not need to.

**The negative-delay is worth noting even though nothing reads it.** `-1s` on a
2s loop means the animation starts half-swept rather than at the keyframe origin,
so there is no instant — not even the first frame — at which the sheen is at its
declared start.

## 3. Every live call site overrides the primitive's own radius

`sidebar-skeleton.tsx` builds **seven** shapes and every one of them names its
own corner — five `rounded` (4px, the literal `0.25rem`) and two `rounded-md`
(`calc(var(--radius) * 0.8)` = 8px). The primitive's `rounded-sm` is therefore
**dead at every call site that exists**.

That is the same shape of finding as `label`'s dead `ui-text-sm`, pointing the
other way: here it is the *primitive's* class that never renders. It is why
`CallSite` carries the radius rather than the component defaulting it, and why
`row_layout/skeleton.rs` asserts the three radii are three different numbers.

A skeleton also has **no size of its own** — the bare primitive pins neither
axis, so §8.3's `empty` is a zero-area box reporting `visible: false`. That is
the same cell `--call-site none` reaches from the other direction, and the two are
asserted to agree; `avatar`'s precedent for checking that two spellings of
"nothing in it" land in one place.

## 4. Declarations

Both lists are **empty**.

* `content_sized` — a skeleton paints no text, and every live shape pins its box
  or stretches with `flex-1`; none of those is a content width.
* `line_sized` — no text at all, so no line box. v1.6 makes the declaration valid
  only on an anchor carrying a `font`.

## 5. What is **not** modelled, and this one is a real visual gap

**The sheen is not drawn.** The port paints the resting colour and the corner;
it does not sweep the gradient that makes a skeleton read as *loading*.

Stated plainly rather than buried: this is the one place in P3.6 where the port
is visibly less than the original. It is also invisible to the oracle in both
directions — the contract records neither the gradient nor its position — so
drawing it would be unverifiable by the gate either way, and it needs a frame
loop this leaf has no other use for. `Skeleton::sweep` carries the duration off
the sealed `--animate-skeleton` token so the number has one home if it is ever
drawn.

It is deliberately **not** recorded as a `const … : bool = true` with a test
asserting it: that is a guard that tests its own declaration and no behaviour.
