//! `dropdown` (P3.31) — a **hand-rolled** floating panel, wrapped through
//! `gpui_component::popover::Popover` for its deferred/anchored placement.
//!
//! # Distinct from `dropdown_menu`, on the evidence
//!
//! `web/src/components/ui/dropdown.tsx` is **not** base-ui's `Menu` and does
//! not compose [`super::dropdown_menu`]'s React original at all. `dropdown-menu`
//! (P2.1, `native/MAPPING.md`) ports `@base-ui-components/react`'s `Menu`
//! primitive — a portal, CSS anchor positioning, roving `focus:` highlighting.
//! `dropdown.tsx` is a **725-line bespoke component**: its own
//! `getBoundingClientRect`-driven positioning (anchor **or** point, with a
//! side-flip computed from available viewport space), `framer-motion` for the
//! mount/exit transition, a hand-rolled `ResizeObserver` + `wheel`/`scroll`/
//! `resize` listener stack, and its own keyboard traversal
//! (`ArrowUp`/`ArrowDown`/`Home`/`End`/`Enter`) — none of which
//! `dropdown-menu.tsx` has or needs, because base-ui supplies all of it.
//! `grep -c 'base-ui\|@radix-ui' dropdown.tsx` is **0**.
//!
//! The two files share no imports, no exported symbol and no class-list
//! constant. They are two independent primitives that happen to render a
//! similar-looking rounded popup, which is exactly the picture `dropdown-menu`'s
//! own `native-menu` entry (P2.14, `native/MAPPING.md`) does **not** touch:
//! the user's native-menu ruling was scoped to base-ui's `Menu` (used for
//! context menus and the review-thread comment menu); `dropdown.tsx` is never
//! mentioned by it and is not a menu-role primitive on every call site (see
//! below), so this item treats it as an ordinary Tier B port target rather than
//! re-litigating that ruling.
//!
//! # What the two exported `*ClassName` helpers say about this surface
//!
//! `dropdownTriggerClassName` and `dropdownItemClassName` are style-only
//! exports — `cn(variants(), className)` and nothing more, no anchor, no
//! element. Tracing every live call site (`web/src/features/**`) shows what
//! they are for: **5 of this component's 7 live `<Dropdown>` renders pass
//! `children`**, not `items`/`sections` — `provider-switch-dropdown.tsx`'s
//! empty-state branch, all three menus in `editor-status-actions.tsx`, and
//! `file-path-breadcrumb.tsx`'s directory listing all build their **own**
//! `<button>`/`<Button>` rows and reach for `dropdownItemClassName` (or,
//! for the trigger, `dropdownTriggerClassName`) purely to keep that
//! independently-built row looking like it belongs in this family — the same
//! relationship `buttonVariants` has to every button-shaped thing that isn't
//! literally `<Button>`. Only `provider-switch-dropdown.tsx`'s populated branch
//! and `file-explorer-tree.tsx`'s filter menu use the structured `items` prop
//! (`MenuItemsList`) at all.
//!
//! So the primitive's own item-row renderer is the **minority** shape in
//! practice, and the majority shape — arbitrary call-site content — is exactly
//! the situation [`super::popover`]'s module docs already worked through for
//! `PopoverContent`'s children: "none of that is this primitive, and a port
//! that reproduced one call site's contents would be measuring that call
//! site." This module follows the same call: the **shell** is ported and
//! anchored; the body is a measured extent, declared rather than reproduced,
//! exactly as [`super::popover::Popover::body_height`] is.
//!
//! `trigger` and `item` below give the two style-only exports a Rust
//! equivalent, at the values `dropdown.tsx`'s own classes specify, in case a
//! future call-site port wants to build a row that matches this family without
//! going through [`Dropdown`]'s (currently unbuilt) `items` path. Neither is
//! anchored — there is nothing to anchor a style function to — and neither
//! has a live Rust call site yet, so neither carries a `row_layout` capture.
//!
//! # The seam, and why it is the same one `popover` already proved
//!
//! `gpui_component::Popover::content`/`ParentElement` is the element seam
//! [`super::popover`]'s module docs already document in full — a `'static`
//! closure cannot capture the `&dyn AnchorSink` borrow this crate is handed, so
//! the popup body is still built here, anchored here, and passed as a child.
//! Nothing about `dropdown.tsx`'s different positioning algorithm changes that:
//! `native/oracle/ANCHORS.md` §4 fixes every `bounds` as **relative to `root`,
//! never absolute** — "the root itself at `{x: 0, y: 0}`" — so the *absolute*
//! page position a popup resolves to (which corner, which side, whether the
//! trigger was a real element or a synthesised point) is placement, invisible
//! to the differ, in exactly the shape `popover`'s own docs already establish
//! for Floating UI's output and `dropdown-menu`'s for `--anchor-width`. Only
//! the root's own **size** and its children's bounds **relative to it** are
//! compared. That is what makes wrapping the *same* `gpui_component::Popover`
//! — unconditionally `Anchor::TopLeft`, exactly as `popover.rs` already does,
//! proved convergent there — sufficient for both of `dropdown.tsx`'s
//! positioning modes (`anchorRef` **and** `point`) without modelling either:
//! neither the side-flip nor the point-vs-anchor distinction produces a field
//! this contract can see.
//!
//! # What had to be overridden, beyond what `popover` already found
//!
//! | | |
//! |---|---|
//! | [`GpuiPopover::appearance`]`(false)` | same reason as `popover`: the vendor's `true` paints its own theme, not ours. |
//! | the root box is **ours** | same mechanism as `popover`'s [`ID_POPUP`](super::popover::ID_POPUP): with `appearance(false)` the vendor's private content box paints nothing, and the box under [`ID_ROOT`] is the one this module builds. |
//! | [`Trigger`] | same bound (`Selectable + IntoElement`) as `popover::Trigger`, and **not reused from it** — this crate's sibling wraps stay independent even where they duplicate a few lines (`dialog`/`alert_dialog`/`sheet` are the precedent: `alert_dialog`'s numbers are `dialog`'s "by finding", and it still owns its own copy rather than importing `dialog`'s). |
//!
//! # One structural difference from `popover`: there is only one box
//! `PopoverPrimitive` splits its popup and its viewport — two nested boxes,
//! two different paddings. `dropdown.tsx`'s root (`m.div`, `dropdownRootVariants`)
//! carries its own `p-1` **and** is the scrolling viewport (`overflow-y-auto`)
//! in the same box; the `role="menu"` child (`menuClassName`) carries no
//! default padding of its own. So this module has exactly **one** anchor,
//! [`ID_ROOT`], where `popover` has two — not an omission, the shape the
//! markup actually has.
//!
//! # The border is real, like `popover`'s and unlike `dropdown_menu`'s ring
//!
//! `border border-border` — a bare `border` utility, **not** `ring-1`. This is
//! the same measurement `popover.rs`'s module docs record as the inverse of
//! `dropdown_menu`'s trap: there, `ring-1` compiles to a box-shadow and
//! `border.w` is `0` on both sides; here the class really is a border, and
//! `border.w` is `1`, the field `ANCHORS.md` v1.1 compares *exactly*. Measure,
//! never infer — this is the third component in the tree to make the same
//! point in the same direction as `popover`, after the first one made it in
//! the other.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement, ParentElement as _, Pixels, RenderOnce, Styled,
    Window, div, px,
};
use gpui_component::{Selectable, popover::Popover as GpuiPopover};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme, ui_sans_font};

/// The root anchor: `m.div`'s own box — bordered, padded, rounded, the
/// scrolling viewport itself. Every other bound this surface could carry
/// would be reported relative to it (`native/oracle/ANCHORS.md` §4); there are
/// none yet, because the body is declared rather than reproduced (see the
/// module docs).
pub const ID_ROOT: &str = "dropdown-root";

/// The anchors whose boxes size to their own text (`ANCHORS.md` v1.5).
///
/// **`ID_ROOT`, and measured rather than assumed.** The first draft of this
/// module reasoned the opposite way — `applyLockedWidth` locks the root's
/// width onto an inline `style.width` before layout settles, so the used
/// width should be an explicit pixel value, the same trade `popover`'s width
/// is. **Live capture of the one reachable cell refutes it.**
/// `file-explorer-tree.tsx`'s filter menu passes `className="w-fit
/// min-w-fit"`, and CSS's own rule for a conflict between `width` and
/// `min-width` is that **`min-width` wins**: the locked style read
/// `width: 197.378128px` while the box's live `getComputedStyle().width` —
/// what the extractor and every renderer actually use — was `201.40625px`,
/// because `min-width: fit-content` re-measures the three-row content on
/// every layout regardless of what the lock wrote. `data-oracle-content-sized`
/// is declared on `dropdown.tsx`'s root for this reason, and the reference
/// (`/tmp/p3-ref-dropdown.json`) carries `"content_sized": true`.
///
/// **This does not hold for every call site**, and that is recorded rather
/// than glossed: `provider-switch-dropdown.tsx` passes `className="min-w-0"`
/// with an explicit `style.width`, which clears the conflicting floor, so its
/// width *is* the locked pixel value there. The declaration here is the one
/// true of the reachable cell — the same epistemic move `popover::Popover`'s
/// `title` field makes for the one reachable popover's shape, not a claim
/// about every possible call site.
pub const CONTENT_SIZED: [&str; 1] = [ID_ROOT];

/// The anchors whose box height is their own line box (`ANCHORS.md` v1.6).
///
/// **None.** The root paints no text of its own — `dropdownRootVariants` has
/// no `ui-text-*` class, and this module's one anchor is a box, not a run. Its
/// height is border-plus-padding-plus-body, the same shape `popover`'s own
/// popup height is (and for the same reason, not line-sized either).
pub const LINE_SIZED: [&str; 0] = [];

/// `border border-border` — one real pixel, not a ring. See the module docs.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `p-1` on the root: `--spacing(1)`, Tailwind's stock `0.25rem` at a 16px
/// root.
pub const PADDING: Pixels = px(4.0);

/// `min-w-[240px]`: the primitive's own unnarrowed floor. **No live call site
/// renders it** — all four either narrow it (`min-w-0`) or replace it with
/// `min-w-fit`, so this constant documents a real CSS fact with no reference
/// to compare against, the same standing `popover::Variant::Tooltip` has.
pub const UNNARROWED_FLOOR: u16 = 240;

/// [`Dropdown::fixture`]'s width: `ceil(201.40625)`, the live
/// `file-explorer-tree.tsx` filter menu at rest, captured
/// 2026-08-02 (`/tmp/p3-ref-dropdown.json`). `ceil`, not the raw measurement,
/// because [`CONTENT_SIZED`] is declared on [`ID_ROOT`] and `ANCHORS.md` v1.5
/// compares a content-sized `bounds.w` against `ceil(reference.w)`.
pub const DEFAULT_WIDTH: u16 = 202;

/// [`Dropdown::fixture`]'s body height: `105 − 2×`[`BORDER_WIDTH`]` − 2×`[`PADDING`]
/// `= 95`, the same live capture's root height less its own chrome. Three
/// `dropdownItemVariants` rows (30px each) plus one `my-0.5 border-t`
/// separator (5px) between the second and third: `30 + 30 + 5 + 30 = 95`.
pub const DEFAULT_BODY_HEIGHT: u16 = 95;

/// The shell — `dropdown.tsx`'s `MenuPopover` root — and whatever a call site
/// puts inside it.
///
/// # Why the body is a **height**, not an element tree
///
/// See the module docs in full; the short version is the same one `popover`
/// already gives: the content is the call site's, five different pictures at
/// the seven live render sites, and none of it is this primitive.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Dropdown {
    /// The root's own width — see [`CONTENT_SIZED`]: this is a content-driven
    /// quantity on the one reachable cell, declared here exactly as
    /// `popover`'s `width` and `dropdown-menu`'s `--anchor-width` are.
    pub width: Pixels,
    /// How tall whatever a call site put inside the `role="menu"` div comes
    /// out, before this shell's own border and `p-1` padding are added.
    pub body_height: Pixels,
}

impl Dropdown {
    /// The live `file-explorer-tree.tsx` filter menu — the one `Dropdown`
    /// render a parity run can currently reach. `--width`/`--body-height`
    /// doc comments on [`DEFAULT_WIDTH`]/[`DEFAULT_BODY_HEIGHT`] carry the
    /// full measurement.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            width: px(f32::from(DEFAULT_WIDTH)),
            body_height: px(f32::from(DEFAULT_BODY_HEIGHT)),
        }
    }

    /// Renders the shell through `gpui_component::Popover`, opting [`ID_ROOT`]
    /// into `anchors`.
    ///
    /// `open` is forced, as `popover.rs`'s own render does: a surface under
    /// measurement is a picture, and a menu nobody has opened is not one.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let root = anchors.root(
            AnchorId::from(ID_ROOT).content_sized(),
            self.shell(theme).child(self.body()),
        );

        GpuiPopover::new("dropdown")
            // See the module docs: `true` paints `gpui-component`'s own theme
            // over ours.
            .appearance(false)
            .default_open(true)
            .open(true)
            .trigger(Trigger::new())
            .child(root)
            .into_any_element()
    }

    /// `m.div`'s own box: `fixed … rounded-xl border border-border bg-card/95
    /// p-1 shadow-[…] backdrop-blur-sm [overscroll-behavior:contain]
    /// overflow-y-auto`.
    ///
    /// `select-none`, `pointer-events-auto`, `z-[10040]` and
    /// `[overscroll-behavior:contain]` have no field on either side
    /// (`ANCHORS.md` §6). `backdrop-blur-sm` likewise — gpui has no backdrop
    /// filter, the same absence `popover`'s `not-dark:bg-clip-padding` is.
    /// `overflow-y-auto` becomes `.overflow_hidden()`, the same trade
    /// `popover`'s viewport and `dropdown_menu`'s popup both make: gpui's
    /// *scrolling* overflow lives on `StatefulInteractiveElement`, and taffy
    /// zeroes the automatic minimum size for non-visible overflow either way.
    fn shell(&self, theme: &Theme) -> Div {
        div()
            .w(self.width)
            .rounded(theme.radius_xl.value())
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.card.mix(BG_OPACITY, Color::TRANSPARENT))
            .p(PADDING)
            .overflow_hidden()
            .font(ui_sans_font(theme))
            // `shadow-[0_14px_30px_-24px_rgba(0,0,0,0.45)]`: §6 invisible on
            // both sides, painted for fidelity the way `popover`'s
            // `shadow_lg()` is — not the same curve, but the same kind of
            // thing, and the differ compares neither.
            .shadow_lg()
    }

    /// The call site's content, as the box it occupies — unanchored, for the
    /// reason the module docs give: it is not part of `dropdown.tsx`, it
    /// stands in for whatever a call site renders, and every live one is a
    /// different picture.
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }
}

/// `bg-card/95`: `color-mix(in oklab, var(--card) 95%, transparent)`.
const BG_OPACITY: f32 = 95.0;

/// The trigger.
///
/// Same bound and same shape as `popover::Trigger` — see the module docs for
/// why this is its own copy rather than a shared one.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, IntoElement)]
pub struct Trigger {
    selected: bool,
}

impl Trigger {
    /// A resting trigger.
    #[must_use]
    pub const fn new() -> Self {
        Self { selected: false }
    }
}

impl Selectable for Trigger {
    fn selected(mut self, selected: bool) -> Self {
        self.selected = selected;
        self
    }

    fn is_selected(&self) -> bool {
        self.selected
    }
}

impl RenderOnce for Trigger {
    fn render(self, _: &mut Window, _: &mut App) -> impl IntoElement {
        div()
    }
}

/// Which of `dropdownTriggerVariants`' two members a trigger takes.
///
/// `dropdown.tsx`'s own default is `default`; `ghost` is the one live call
/// site (`provider-switch-dropdown.tsx`, a trigger sitting on a surface
/// rather than beside content).
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum TriggerVariant {
    /// `buttonVariants({variant: 'default'})` plus [`TRIGGER_SHARED`]'s own
    /// overrides.
    #[default]
    Default,
    /// `buttonVariants({variant: 'ghost'})` plus the same overrides — no
    /// visible box at rest.
    Ghost,
}

/// `dropdownTriggerClassName(className?, variant?)`, as a style refinement
/// over a caller's own element rather than a `className` string.
///
/// **Style-only, unanchored, no live Rust call site.** See the module docs:
/// every current importer builds its own trigger element and merges this
/// family's look onto it; none of those call sites is ported yet, so this has
/// no `row_layout` capture and no `--surface` cell. It exists so that port,
/// when it happens, is a call site reaching for a token table rather than
/// re-deriving `TRIGGER_SHARED`'s five values by hand.
///
/// `TRIGGER_SHARED`'s own overrides — `min-w-0 gap-1 rounded-lg px-2
/// text-muted-foreground` — are applied **after** the base variant's, the
/// same order `cn(buttonVariants({variant}), TRIGGER_SHARED)` establishes and
/// the same reason `dropdown.tsx`'s own comment gives: a caller composing the
/// two the other way round would silently lose the trigger's own padding to
/// the button's.
#[must_use]
pub fn trigger<E: Styled>(mut element: E, theme: &Theme, variant: TriggerVariant) -> E {
    use crate::components::button::{ButtonState, Variant as ButtonVariant};

    let button_variant = match variant {
        TriggerVariant::Default => ButtonVariant::Default,
        TriggerVariant::Ghost => ButtonVariant::Ghost,
    };
    let state = ButtonState::resting();

    if let Some(bg) = button_variant.background(theme, state) {
        element = element.bg(bg);
    }
    element
        .text_color(theme.muted_foreground)
        .border_color(button_variant.border(theme, state))
        .rounded(theme.radius_lg.value())
        .px(TRIGGER_PADDING_X)
        .gap(TRIGGER_GAP)
}

/// `px-2`.
const TRIGGER_PADDING_X: Pixels = px(8.0);
/// `gap-1`.
const TRIGGER_GAP: Pixels = px(4.0);

/// `dropdownItemClassName(className?)`, as a style refinement — the same
/// shape as [`trigger`] and the same reason: no live Rust call site yet.
///
/// `dropdownItemVariants`' two independent axes, `disabled` and `focused`,
/// are the two `bool`s: `cursor-not-allowed opacity-50` against
/// `cursor-pointer hover:bg-muted`, and `bg-muted` against nothing. `hover:`
/// has no field on this side (interaction refinements are invisible to a
/// snapshot, `components/mod.rs`'s own standing rule) — `focused` is what a
/// driven cell can actually select.
#[must_use]
pub fn item<E: Styled>(mut element: E, theme: &Theme, disabled: bool, focused: bool) -> E {
    element = element
        .text_color(theme.foreground)
        .rounded(theme.radius_lg.value())
        .px(ITEM_PADDING_X)
        .py(ITEM_PADDING_Y)
        .gap(ITEM_GAP);

    if focused {
        element = element.bg(theme.muted);
    }
    if disabled {
        element = element.opacity(ITEM_DISABLED_OPACITY);
    }
    element
}

/// `px-2.5`.
const ITEM_PADDING_X: Pixels = px(10.0);
/// `py-1.5`.
const ITEM_PADDING_Y: Pixels = px(6.0);
/// `gap-3`.
const ITEM_GAP: Pixels = px(12.0);
/// `opacity-50`.
const ITEM_DISABLED_OPACITY: f32 = 0.5;

/// `dropdownSectionLabelVariants`'s own weight and colour — `ui-font
/// ui-text-sm text-muted-foreground`, no background, no border.
pub const SECTION_LABEL_WEIGHT: FontWeight = FontWeight::NORMAL;

#[cfg(test)]
mod tests {
    use super::{
        BG_OPACITY, BORDER_WIDTH, CONTENT_SIZED, DEFAULT_BODY_HEIGHT, DEFAULT_WIDTH, Dropdown,
        ID_ROOT, ITEM_GAP, ITEM_PADDING_X, ITEM_PADDING_Y, LINE_SIZED, PADDING, TRIGGER_GAP,
        TRIGGER_PADDING_X, Trigger, TriggerVariant, UNNARROWED_FLOOR, item, trigger,
    };
    use crate::theme::{Color, Theme};
    use gpui::{Styled as _, div, px};
    use gpui_component::Selectable as _;

    /// The one anchor, and it declares v1.5 but not v1.6 — see both consts'
    /// doc comments for the measurement.
    #[test]
    fn there_is_exactly_one_anchor_and_it_declares_content_sized_only() {
        assert_eq!(ID_ROOT, "dropdown-root");
        assert_eq!(CONTENT_SIZED, [ID_ROOT]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// **The border is real, like `popover`'s and unlike `dropdown_menu`'s
    /// ring** — see the module docs for the full argument.
    #[test]
    fn the_border_is_one_real_pixel_and_not_a_ring() {
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    /// `rounded-xl` and `bg-card/95` are `dropdown`'s own tokens, and they are
    /// **not** `popover`'s: `popover-popup` is `rounded-lg` (10px) over
    /// `bg-popover` with no opacity mix. A bare `radius_lg`/`theme.popover`
    /// here would be the `ANCHORS.md` v1.6 mistake `components/mod.rs`'s
    /// module docs warn about, one door over.
    ///
    /// **Measured, not inferred, and the measurement has a wrinkle worth
    /// recording**: `theme.css` aliases `--card: var(--color-white)` /
    /// `--popover: var(--color-white)` in light and `--card: var(--background)`
    /// / `--popover: var(--background)` in dark, so `theme.card` and
    /// `theme.popover` are the **same colour** on both tables — the two names
    /// diverge in the stylesheet's vocabulary, not in its palette. What still
    /// distinguishes this shell from `popover-popup` is real: the radius (14
    /// against 10, checked below) and the alpha the `/95` mix leaves on the
    /// composited paint (checked in `row_layout/dropdown.rs`, where it is
    /// compared as the `Paint` the differ actually reads) — `bg-popover`
    /// carries no such mix and paints fully opaque.
    #[test]
    fn the_shell_takes_this_surfaces_own_tokens_not_popovers() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_xl.value(), px(14.0));
            assert_ne!(theme.radius_xl, theme.radius_lg);
            // Same colour, different alpha once mixed — see the doc comment.
            assert_eq!(theme.card, theme.popover);
            assert!(theme.card.mix(BG_OPACITY, Color::TRANSPARENT).value().a < 1.0);
            assert!((theme.popover.value().a - 1.0).abs() < f32::EPSILON);
        }
        assert!((BG_OPACITY - 95.0).abs() < f32::EPSILON);
        assert_eq!(PADDING, px(4.0));
    }

    /// `min-w-[240px]` is a real CSS fact with no reachable reference — kept
    /// distinct from the fixture, which is the cell a parity run can actually
    /// compare against. See both consts' own doc comments.
    #[test]
    fn the_unnarrowed_floor_and_the_fixture_are_two_different_numbers() {
        assert_eq!(UNNARROWED_FLOOR, 240);
        assert_eq!(DEFAULT_WIDTH, 202);
        assert_eq!(Dropdown::fixture().width, px(202.0));
    }

    /// The fixture's height is exactly two borders, two paddings and the
    /// declared body — the same arithmetic `popover::Popover::fixture`'s own
    /// test checks, applied to a shell with one box instead of two. And it
    /// reproduces the live capture's own root height, 105px.
    #[test]
    fn the_fixture_body_height_is_declared_not_derived() {
        let dropdown = Dropdown::fixture();
        assert_eq!(dropdown.body_height, px(95.0));
        assert_eq!(DEFAULT_BODY_HEIGHT, 95);

        let shell_height = f32::from(BORDER_WIDTH) * 2.0
            + f32::from(PADDING) * 2.0
            + f32::from(dropdown.body_height);
        // 1 + 4 + 95 + 4 + 1 — `/tmp/p3-ref-dropdown.json`'s own `h: 105`.
        assert!(
            (shell_height - 105.0).abs() < f32::EPSILON,
            "{shell_height}"
        );
    }

    /// The trigger satisfies the vendor's bound and carries no paint of its
    /// own — `PopoverTrigger`-equivalent semantics, checked the same way
    /// `popover::Trigger`'s own test is.
    #[test]
    fn the_trigger_is_selectable_and_unstyled() {
        let trigger = Trigger::new();
        assert!(!trigger.is_selected());
        assert!(trigger.selected(true).is_selected());
        assert_eq!(Trigger::default(), Trigger::new());
    }

    /// `trigger()` applies `TRIGGER_SHARED`'s own five values regardless of
    /// which base variant it starts from — the property `dropdown.tsx`'s own
    /// comment insists on: the trigger's padding must beat the button's.
    #[test]
    fn trigger_style_applies_its_own_padding_and_gap_over_either_variant() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            for variant in [TriggerVariant::Default, TriggerVariant::Ghost] {
                let mut styled = trigger(div(), &theme, variant);
                assert_eq!(styled.style().padding.left, Some(TRIGGER_PADDING_X.into()));
                assert_eq!(styled.style().gap.width, Some(TRIGGER_GAP.into()));
                assert_eq!(
                    styled.style().text.color,
                    Some(theme.muted_foreground.value()),
                );
            }
        }
        assert_ne!(TriggerVariant::default(), TriggerVariant::Ghost);
    }

    /// `item()`'s two axes move independently: `focused` paints a background,
    /// `disabled` sets opacity, and neither implies the other — the same
    /// independence `dropdownItemVariants`' two `cva` axes have.
    #[test]
    fn item_style_moves_focused_and_disabled_independently() {
        let theme = Theme::DARK;

        let mut resting = item(div(), &theme, false, false);
        assert!(resting.style().background.is_none());
        assert_eq!(resting.style().opacity, None);

        let mut focused = item(div(), &theme, false, true);
        assert!(focused.style().background.is_some());
        assert_eq!(focused.style().opacity, None);

        let mut disabled = item(div(), &theme, true, false);
        assert!(disabled.style().background.is_none());
        assert_eq!(disabled.style().opacity, Some(0.5));

        let mut both = item(div(), &theme, true, true);
        assert!(both.style().background.is_some());
        assert_eq!(both.style().opacity, Some(0.5));

        assert_eq!(ITEM_PADDING_X, px(10.0));
        assert_eq!(ITEM_PADDING_Y, px(6.0));
        assert_eq!(ITEM_GAP, px(12.0));
    }
}
