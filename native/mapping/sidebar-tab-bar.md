# `sidebar-tab-bar` (P3.55) — verdict: **no surface**, argued fresh

`web/src/components/layout/sidebar-tab-bar.tsx` →
`crates/crowbar-ui/src/components/sidebar_tab_bar.rs`. **No
`crates/crowbar-app/src/surfaces/sidebar_tab_bar.rs` exists, and this
doc's own conclusion is that none should.** A `row_layout` geometry suite
does exist —
`crates/crowbar-app/src/row_layout/sidebar_tab_bar.rs` — because that
coverage does not require a surface. See §3.

> Cluster 3, "standalone sidebar chrome" (`native/mapping/layout-denominator.md`
> §8).

## 0. The question this item's own brief posed, and the instruction to decide it rather than assume it

`sidebar_tab_bar.rs`'s own module docs already carry a ruling, in a section
titled *"Why this stays a `crowbar-ui` field rather than becoming its own
`--surface`."* This item's brief was explicit that inheriting that
conclusion without re-deriving it would not be enough — so what follows is
this item's own check of the claim, not a restatement of it. The check
agrees with the existing ruling; had it not, this doc would say so and argue
the surface into existence instead.

## 1. What the React source actually does — read directly, not inferred

`sidebar-tab-bar.tsx`'s own wrapper `<div className="@container flex
shrink-0 items-center px-2 py-1.5">` carries **no `data-oracle-id`**, checked
by reading the file directly (`web/src/components/layout/sidebar-tab-bar.tsx`,
confirmed in this session). The file's own comment, immediately above the
component, states the omission is deliberate and names the reason: the Rust
port returns the wrapped `Tabs` element directly rather than opting a second,
fabricated root into its anchor sink, and every anchor this wrapper's own
render produces belongs to `tabs.tsx`'s own `data-oracle-id`s, unmodified.
Nothing about this file's own `<Tabs>`, `<TabsList>` or `<TabsTab>` usage
adds an id of its own either — they are the same `ui/tabs.tsx` primitive
`tabs.rs` already wraps, called with no `id`/`data-oracle-id` override
anywhere in this file.

## 2. What `crowbar_ui::components::sidebar_tab_bar` actually does — read directly, not inferred

`SidebarTabBar::render` is:

```rust
pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
    self.shell()
        .child(self.tabs().render(theme, anchors))
        .into_any_element()
}
```

`self.shell()` — the `@container flex shrink-0 items-center px-2 py-1.5`
wrapper — is **never anchored**: no `anchors.root(...)` call, no
`anchors.boxed(...)` call wraps it. `ID_SIDEBAR_TAB_BAR` exists as a `pub
const` in the file, but its own doc comment says plainly: *"this anchor never
renders… kept as a name for the caption/`--help` machinery only."* The
element `render` returns is `self.tabs().render(theme, anchors)`'s own output
— `tabs::Tabs::render`, which calls `anchors.root(ID_ROOT.into(), …)` and
becomes its own frame boundary. So the claim in §1 is not merely asserted by
the React file's comment; it is confirmed independently on the Rust side by
reading the one function that would have to opt a second root in, and
finding that it does not.

## 3. Why this decides against a surface, given the registry's own rule

`crowbar-app/src/surface.rs`'s own test,
`every_registered_surface_has_its_own_name_and_root`, asserts that no two
registered surfaces share a `root` anchor. A hypothetical
`--surface sidebar-tab-bar` would have to declare *some* `root` — and the
only anchor this composition's own render ever produces is `tabs::ID_ROOT`
("tabs"), already claimed by the registered `tabs` surface. Two paths are
available, and neither is honest:

1. **Register `tabs::ID_ROOT` as this surface's own root too.** Rejected
   outright by `every_registered_surface_has_its_own_name_and_root` — this is
   not a hypothetical refusal, it is the literal assertion that test makes.
2. **Mint a new, unused anchor on the wrapper `<div>` and register that
   instead.** This is the fabricated-anchor move the brief that assigned this
   item names explicitly and forbids, the identical move `ANCHORS.md` and
   every module in this crate already refuse elsewhere — the anchor would
   exist in the Rust port and nowhere in the real DOM, so a differ comparing
   it against a live capture would either find nothing to compare (an id the
   reference can never produce) or, worse, a worker under time pressure
   could be tempted to patch the React source to match it — the exact
   "hand-fabricated a reference" failure this item's own brief names as the
   worst available outcome.

So the honest answer is: **this composition has no root of its own to
register, and inventing one is worse than having no surface at all.**

## 4. What this verdict does *not* cost — the geometry is already reachable, and this item adds direct proof besides

Two things soften "no surface" from a coverage gap into a scoping call with
nothing left on the table:

**`tabs`'s own surface already reaches the identical geometry.**
`crowbar_ui::components::tabs::Tabs::fixture`'s own doc comment names this
exact call site as its reference — *"the live sidebar tab bar, as
`sidebar-tab-bar.tsx` renders it on the home route"* — and `tabs.md` §9
carries the captured numbers this port converged on. `--surface tabs --tabs
workspaces,chats,files` and `--tabs workspaces,chats,files,git` already drive
the identical three-tab/four-tab geometry `include_git` produces, generically
rather than route-flavoured. What `tabs`'s own surface cannot reach is this
wrapper's own `px-2`/`py-1.5`/`@container` arithmetic — which is what §4's
second half, and this item's own `row_layout` suite, close.

**This item adds a `row_layout` suite that drives the real composition
directly, with no `Surface`/`Cell` in the way.** A registered surface is not
a precondition for a `#[gpui::test]` that lays a component out in a real
window — `crowbar-app/src/row_layout.rs`'s own shared harness
(`lay_out`) is happy to measure any `Render` implementation, and `build.rs`'s
own docs are explicit that `src/row_layout/` requires nothing but a file that
compiles and passes: "there is nothing to register." So
`crates/crowbar-app/src/row_layout/sidebar_tab_bar.rs` builds a one-view
`Stage` around a bare `SidebarTabBar`, calls `SidebarTabBar::render` directly
and reads the resulting geometry back, exactly as every registered surface's
own suite does — the only thing missing is the `--surface` word to select it
from a command line, because there is no anchor of this wrapper's own for a
`Surface::root` to name. That suite proves, through a real taffy layout
rather than the module docs' hand arithmetic:

* `SidebarTabBar::render` never opts a second root into the sink — the
  structural claim §1–2 make, held as a test rather than left as a comment
* the fixture's own 294px column reproduces `tabs.md`'s own captured 278px
  width exactly (`294 − 2 × 8`)
* that `16px` arithmetic holds at other column widths, not only the one
  captured cell
* the wrapper's own padding genuinely **offsets** the nested `tabs` root
  (its top-left corner sits at `(8, 6)`), not only shrinks its width — a
  distinction a width-only check could not make
* `include_git` adds a real, observable fourth anchor (`tabs-tab-git`) to
  the rendered tree

One thing this suite cannot reach, and says so rather than approximating:
**label visibility has no anchor of its own to observe.** `Tabs`' own
`tab_sizing: TabSizing::Fill` gives every tab equal `flex-grow` width
regardless of its content (`tabs.rs`'s own doc comment: *"under
`TabSizing::Fill` it cannot move [an anchor's width]"*), and a tab's label is
an unanchored `div().child(label.clone())` — the same "plain child, no
`data-oracle-id`" shape `fps-overlay`'s seven runs take. So whether
`shows_label` returns `true` or `false` changes no box this contract can
read back; that logic is instead pinned at the unit level, in
`crowbar_ui::components::sidebar_tab_bar`'s own
`labels_appear_in_the_order_the_container_query_promises` test. Naming this
here rather than silently working around it is the same discipline
`fps-overlay.md` §7 and `sidebar-toggle-icon.md` §5 already apply to their
own unmodellable halves.

## 5. `include_git`, and the fixture-bug finding already on record

`crowbar_ui::components::sidebar_tab_bar`'s own module docs §3 record a real
port defect the P3.52 gate caught: an early fixture combined `include_git:
true` with the 294px/278px capture, which was taken on the home route
(`include_git: false`) — a combination the live app never produces at once.
Fixed by setting the fixture to `include_git: false` and adding a direct
regression guard. This item's own `include_git_adds_the_fourth_tab_anchor`
`row_layout` test is additional, independent coverage of the same axis from
the geometry side, not a restatement of that unit-level fix.

## 6. Reachability

`ide-shell.tsx` is the one importer
(`native/mapping/layout-denominator.md` §2) — mounted unconditionally as the
sidebar's own tab strip.
