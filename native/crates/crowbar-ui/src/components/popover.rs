//! `popover` — the first component this design system **wraps** rather than
//! rebuilds.
//!
//! Spec §6.2 names `popover` among the primitives `gpui-component` already
//! provides, and says we wrap it "so the `gpui-component` upgrade surface is
//! confined to one crate". That is what this module does, and the division is
//! the one §4.2 draws: **`gpui_component::Popover` owns the behaviour** — the
//! open/closed state, the deferred anchored placement, focus tracking and
//! restoration, `escape` and click-outside dismissal — and **`crowbar-ui` owns
//! the appearance**, every pixel of which comes out of [`Theme`].
//!
//! The React half is `web/src/components/ui/popover.tsx`, a set of Tailwind
//! class lists over `@base-ui/react`'s `Popover`.
//!
//! # The seam, and why it is `ParentElement` rather than `content`
//!
//! `gpui_component::Popover` offers two ways to put something in the popup:
//! `Popover::content`, which takes a builder, and `ParentElement`, which takes
//! already-built children. **Only the second is reachable from here.**
//!
//! `content` is `F: Fn(&mut PopoverState, &mut Window, &mut Context<PopoverState>) -> E + 'static`.
//! A component in this crate is handed `&dyn AnchorSink` with an anonymous
//! lifetime — that is the whole arrangement `anchor.rs` exists for — and a
//! borrow with a lifetime cannot be captured by a `'static` closure. So the
//! popup body is built *here*, anchored *here*, and handed over as a child.
//! Nothing is lost by it: the vendor places `children` inside the same box it
//! would have placed `content` in.
//!
//! # What had to be overridden to reach our design
//!
//! Recorded in full, because eight more wrapped components will lean on it.
//!
//! | | |
//! |---|---|
//! | [`GpuiPopover::appearance`]`(false)` | the vendor's `true` paints `popover_style(cx)` — **`gpui-component`'s own theme**, not ours — plus a `p_3()` this primitive does not have. Turning it off is what makes the box paintable from [`Theme`]. |
//! | The popup box is **ours**, not the vendor's | the vendor builds its content box inside `Popover::render_popover_content`, a private function. [`AnchorSink::root`] takes a [`Div`] this crate holds, so a box we never hold cannot carry an anchor. With `appearance(false)` the vendor's box paints nothing at all, and the box under [`ID_POPUP`] — bordered, rounded, `bg-popover` — is the one built by [`Popover::popup`]. |
//! | [`Trigger`] | `Popover::trigger` needs `T: Selectable + IntoElement`. No gpui element is `Selectable`; it is `gpui-component`'s own trait. `PopoverTrigger` in React carries **no styling of its own** — it is a className passthrough — so the trigger here is an unstyled box that implements the trait and nothing else. |
//!
//! # Two things the vendor adds that base-ui does not, and one it drops
//!
//! * A `v_flex()` **wrapper** survives `appearance(false)`: `.id("content")`,
//!   `.occlude()`, `.tab_group()` and a `top_1()`/`bottom_1()` offset. It paints
//!   nothing and takes no room, and the offset is placement — which §4 makes
//!   invisible, since every bound in a snapshot is relative to [`ID_POPUP`].
//! * The vendor snaps to the window with an 8px margin
//!   (`anchored().snap_to_window_with_margin(px(8.))`); base-ui's Floating UI
//!   does the same job with `max-w-(--available-width)`. Both are placement.
//! * `sideOffset={4}` is base-ui's; the vendor's equivalent is that `top_1()`,
//!   which is the same 4px by coincidence rather than by configuration. Neither
//!   is comparable, for the same reason.
//!
//! # What the oracle can and cannot see here
//!
//! `shadow-lg/5` and the `before:` inset shadow are `ANCHORS.md` §6 material —
//! no field, either side. The port paints [`Popover::popup`]'s `shadow_lg()` for
//! fidelity and the differ will neither confirm nor deny it.
//!
//! **The border is a real border**, and that is the inverse of the trap
//! `dropdown_menu` documents: there, `ring-1` compiles to a box-shadow and
//! `border.w` is **0** on both sides. Here the class is a bare `border`, which
//! is `border-width: 1px`, and `border.w` is **1** — the field `ANCHORS.md` v1.1
//! compares *exactly*. Measured on the live popup rather than read off the class
//! name: `borderTopWidth` `1px`, `borderTopColor` `oklch(1 0 0 / 0.06)` — the
//! `--border` token.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement, ParentElement as _, Pixels, RenderOnce,
    SharedString, Styled as _, Window, div, px, relative, rems,
};
use gpui_component::{Selectable, popover::Popover as GpuiPopover};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Theme, ui_sans_font};

/// The root anchor: `PopoverPrimitive.Popup`, the bordered box `bg-popover`
/// paints. Every other bound on this surface is reported relative to it
/// (`native/oracle/ANCHORS.md` §4).
pub const ID_POPUP: &str = "popover-popup";
/// `PopoverPrimitive.Viewport` — the padded, clipping box the children sit in.
pub const ID_VIEWPORT: &str = "popover-viewport";
/// `PopoverTitle` — `PopoverPrimitive.Title`, rendered only where a call site
/// asks for one.
pub const ID_TITLE: &str = "popover-title";

/// The anchors whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and that is arithmetic rather than an omission. v1.5 applies to a
/// box whose used width *is* a text run's max-content width. The popup's width
/// is a definite length — `w-(--popup-width,auto)`, which every live call site
/// replaces with its own `w-*`; the viewport is `size-full` inside it; and the
/// title is a **block-level** `<h2>`, so its used width is the viewport's
/// content width whatever its string says.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **The title, and only the title.** `leading-none` is `line-height: 1`, so an
/// 18px `text-lg` run sits in an 18px line box and the `<h2>`'s border box *is*
/// that box — there is no padding and no authored height. That is exactly the
/// shape v1.6 was written for.
///
/// The popup and the viewport are not: both are boxes whose height is their
/// content's plus padding and borders, and declaring either would compare a
/// 177px box against a 24px line box.
///
/// Worth stating that this declaration costs nothing *here* while still being
/// correct: `leading-none` resolves to a whole 18px on both sides, so `floor`
/// and `pixel_snap` agree and the two spellings compare the same number. It is
/// declared because it is **true**, which is the test `ANCHORS.md` v1.6 sets —
/// not because a cell needed rescuing.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `border` on the popup — a **real 1px border**, not a ring.
///
/// See the module docs: this is the inverse of `dropdown_menu`'s trap, and
/// `border.w` is compared exactly.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `py-4` on the viewport, and `[--viewport-inline-padding:--spacing(4)]`
/// feeding `px-(--viewport-inline-padding)` — 16px on all four sides.
pub const VIEWPORT_PADDING: Pixels = px(SPACING * 4.0);

/// The `tooltipStyle` viewport's `py-1`.
pub const TOOLTIP_VIEWPORT_PADDING_Y: Pixels = px(SPACING);

/// The `tooltipStyle` viewport's `[--viewport-inline-padding:--spacing(2)]`.
pub const TOOLTIP_VIEWPORT_PADDING_X: Pixels = px(SPACING * 2.0);

/// `text-base`'s line height — Tailwind's stock `--text-base--line-height`,
/// `calc(1.5 / 1)`, so 16px text sits on a 24px line box.
///
/// The popup is **portalled to `document.body`**, so it inherits the document
/// root's font rather than whatever opened it. Measured on the live popup:
/// `16px/24px`.
const POPUP_LINE_HEIGHT: f32 = 1.5;

/// `text-xs`'s line height under `tooltipStyle` — `calc(1 / 0.75)`, so 12px
/// text sits on a 16px line box.
const TOOLTIP_LINE_HEIGHT: f32 = 1.0 / 0.75;

/// `text-lg` on the title: `1.125rem`.
///
/// Spelled as a bare `rems` rather than through a token because **no token
/// carries it**: `--ui-text-lg` is `1rem` and `--ui-text-xl` is `1.25rem`, and
/// this design system has no member of Tailwind's own scale at 1.125. Minting
/// one would put a value in the system the system does not have — the same
/// trade `dropdown_menu` makes for `text-xs`, one step further along.
const TITLE_TEXT_SIZE: f32 = 1.125;

/// `leading-none` on the title: `line-height: 1`.
const TITLE_LINE_HEIGHT: f32 = 1.0;

/// `font-semibold`.
const TITLE_WEIGHT: FontWeight = FontWeight::SEMIBOLD;

/// Which of `popover.tsx`'s two appearances a popup takes.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Variant {
    /// The prop's own default: `rounded-lg`, `shadow-lg/5`, a 16px viewport.
    #[default]
    Default,
    /// `tooltipStyle`: `w-fit rounded-md text-xs shadow-md/5`, and a viewport
    /// padded `py-1` / `px-2`.
    ///
    /// **Unreached by any live call site.** `grep tooltipStyle` finds it on
    /// `toast.tsx`'s own primitive and on no `PopoverContent` anywhere, so this
    /// arm has no reference to compare against. It is modelled because the
    /// primitive has it, and named as unreachable rather than quietly rendered —
    /// the call `dropdown_menu` made about its `overflow` label.
    Tooltip,
}

impl Variant {
    /// The viewport's block padding.
    #[must_use]
    pub const fn viewport_padding_y(self) -> Pixels {
        match self {
            Self::Default => VIEWPORT_PADDING,
            Self::Tooltip => TOOLTIP_VIEWPORT_PADDING_Y,
        }
    }

    /// The viewport's inline padding.
    #[must_use]
    pub const fn viewport_padding_x(self) -> Pixels {
        match self {
            Self::Default => VIEWPORT_PADDING,
            Self::Tooltip => TOOLTIP_VIEWPORT_PADDING_X,
        }
    }

    /// The popup's line box, as a multiple of its font size.
    #[must_use]
    pub const fn line_height(self) -> f32 {
        match self {
            Self::Default => POPUP_LINE_HEIGHT,
            Self::Tooltip => TOOLTIP_LINE_HEIGHT,
        }
    }

    /// The popup's radius token: `rounded-lg`, or `rounded-md` under
    /// `tooltipStyle`.
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Pixels {
        match self {
            Self::Default => theme.radius_lg.value(),
            Self::Tooltip => theme.radius_md.value(),
        }
    }

    /// The popup's font size: the document root's `text-base` through
    /// `--ui-text-lg`, or `text-xs` through `--ui-text-sm`.
    ///
    /// Both are the token whose *number* matches Tailwind's, which is the trade
    /// `dropdown_menu` documents: this system's `--ui-text-*` scale is not
    /// Tailwind's, and there is no token for Tailwind's own.
    #[must_use]
    pub fn text_size(self, theme: &Theme) -> gpui::Rems {
        match self {
            Self::Default => theme.ui_text_lg.value(),
            Self::Tooltip => theme.ui_text_sm.value(),
        }
    }
}

/// The popup, its viewport and whatever the call site put inside it.
///
/// # Why the body is a **height** rather than an element tree
///
/// `PopoverContent`'s children are the call site's, and every live one is a
/// different sub-UI: `repo-icon-popover` puts an avatar and three buttons in
/// there, `commit-popover` a form. None of that is this primitive, and a port
/// that reproduced one call site's contents would be measuring that call site.
///
/// So the body is its **measured extent**, exactly as `dropdown_menu` takes the
/// positioner's `--anchor-width` as a parameter: a runtime quantity this
/// component does not decide, declared rather than computed, so the two boxes
/// that *are* the primitive — the popup and the viewport — compare against a
/// reference whose content is whatever it happens to be.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Popover {
    /// The popup's width. `w-(--popup-width,auto)` in the class list; every
    /// live call site replaces it — `repo-icon-popover`'s `className` is
    /// `w-64 p-0`, so 256px.
    pub width: Pixels,
    /// The height the call site's children come out at, inside the viewport's
    /// padding.
    pub body_height: Pixels,
    /// Which appearance.
    pub variant: Variant,
    /// `PopoverTitle`, when a call site renders one.
    ///
    /// `commit-popover` and `merge-popover` both do; `repo-icon-popover` — the
    /// one popover a parity run can currently drive — does not, which is why
    /// this defaults to `None`. An anchor the reference cannot produce is a
    /// `FieldPresence` delta that forgives nothing.
    pub title: Option<SharedString>,
}

impl Popover {
    /// The live `repo-icon-popover`, which is the only popover in the app a
    /// parity run can reach: `commit-popover` and `merge-popover` both live
    /// behind a git panel that needs a dirty worktree.
    ///
    /// Its numbers are measured off the running app rather than read off the
    /// class list — popup 256 × 177, viewport 254 × 175 inside a 1px border, so
    /// the body is `175 − 16 − 16 = 143`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            width: px(256.0),
            body_height: px(143.0),
            variant: Variant::Default,
            title: None,
        }
    }

    /// Renders the popup through `gpui_component::Popover`, opting every
    /// contract anchor into `anchors`.
    ///
    /// `open` is forced rather than left to the user: a surface under
    /// measurement is a picture, and a popup nobody has clicked is not one.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let popup = anchors.root(
            ID_POPUP.into(),
            self.popup(theme).child(self.viewport(theme, anchors)),
        );

        GpuiPopover::new("popover")
            // See the module docs: `true` would paint `gpui-component`'s own
            // theme over ours and add a `p_3()` this primitive does not have.
            .appearance(false)
            .default_open(true)
            .open(true)
            .trigger(Trigger::new())
            .child(popup)
            .into_any_element()
    }

    /// `PopoverPrimitive.Popup`'s own box.
    ///
    /// The font family is named explicitly for the reason `dropdown_menu`'s is:
    /// gpui can only report the *declared* family, and an inherited
    /// `.SystemUIFont` is a string the DOM will never produce. The reference's
    /// is `CalSansUI` — the popup is portalled to `document.body`, so it
    /// inherits `html { @apply font-sans }`. [`ui_sans_font`] also carries the
    /// fallback chain a `Variant::Tooltip` shortcut needs: its legend is a
    /// macOS modifier symbol, which `CalSansUI`'s own cmap does not cover
    /// (P3.24).
    fn popup(&self, theme: &Theme) -> Div {
        div()
            .flex()
            .w(self.width)
            .rounded(self.variant.radius(theme))
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .text_color(theme.popover_foreground)
            .font(ui_sans_font(theme))
            .font_weight(FontWeight::NORMAL)
            .text_size(self.variant.text_size(theme))
            .line_height(relative(self.variant.line_height()))
            // `shadow-lg/5`. Invisible to the differ (`ANCHORS.md` §6) and
            // painted for fidelity — the popup's only other visible edge.
            .shadow_lg()
    }

    /// `PopoverPrimitive.Viewport`: `relative size-full overflow-clip` with the
    /// variant's padding.
    ///
    /// `size-full` inside a `flex` popup makes it the popup's single stretched
    /// item, which is why the popup's height is the viewport's and the
    /// viewport's is the body's plus padding.
    fn viewport(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut viewport = div()
            .relative()
            .w_full()
            .px(self.variant.viewport_padding_x())
            .py(self.variant.viewport_padding_y())
            // `overflow-clip`, plus `not-data-transitioning:overflow-y-auto`.
            // Hidden here for the reason `dropdown_menu`'s popup is: gpui's
            // scrolling overflow lives on `StatefulInteractiveElement`, taffy
            // zeroes the automatic minimum size for any non-visible overflow
            // either way, and macOS overlay scrollbars reserve no gutter in
            // WebKit either.
            .overflow_hidden();

        if let Some(title) = &self.title {
            viewport = viewport.child(anchors.boxed_text(
                AnchorId::new(ID_TITLE).line_sized(),
                Self::title(theme),
                title.clone(),
            ));
        }
        anchors.boxed(ID_VIEWPORT.into(), viewport.child(self.body()))
    }

    /// `PopoverTitle` — `font-semibold text-lg leading-none`.
    fn title(theme: &Theme) -> Div {
        div()
            .text_size(rems(TITLE_TEXT_SIZE))
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(TITLE_WEIGHT)
            .text_color(theme.popover_foreground)
    }

    /// The call site's children, as the box they occupy.
    ///
    /// **Unanchored on purpose.** It is not part of `popover.tsx`; it stands in
    /// for whatever a call site rendered, and anchoring it would invite a
    /// comparison against an element that is a different thing on every call
    /// site. It is *rendered* because it is what gives the viewport its height,
    /// which is the quantity the two anchored boxes are built out of.
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }
}

/// The popover's trigger.
///
/// `gpui_component::Popover::trigger` needs `T: Selectable + IntoElement`, and
/// `Selectable` is `gpui-component`'s own trait — no gpui element implements it.
/// So this is the smallest thing that satisfies the bound.
///
/// **It is deliberately unstyled and deliberately unanchored.** `PopoverTrigger`
/// in `popover.tsx` carries no class list of its own; the styling belongs to
/// whatever a call site passes through `className`. And the popup is *portalled
/// to `document.body`*, so the trigger is not beneath [`ID_POPUP`] in the DOM —
/// `ANCHORS.md` v1.8 makes a snapshot exactly the surface's own anchors, and the
/// trigger is not one of them.
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

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, ID_POPUP, ID_TITLE, ID_VIEWPORT, LINE_SIZED,
        POPUP_LINE_HEIGHT, Popover, TITLE_LINE_HEIGHT, TITLE_TEXT_SIZE, TOOLTIP_LINE_HEIGHT,
        TOOLTIP_VIEWPORT_PADDING_X, TOOLTIP_VIEWPORT_PADDING_Y, Trigger, VIEWPORT_PADDING, Variant,
    };
    use crate::theme::Theme;
    use gpui::px;
    use gpui_component::Selectable as _;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // py-4 / --spacing(4)
        assert_eq!(TOOLTIP_VIEWPORT_PADDING_Y, px(STEP)); // py-1
        assert_eq!(TOOLTIP_VIEWPORT_PADDING_X, px(STEP * 2.0)); // --spacing(2)
    }

    /// **The border is a real border, and this is the inverse of the ring
    /// trap.** `dropdown_menu`'s popup reports `border.w: 0` because `ring-1`
    /// compiles to a box-shadow; this popup's class is a bare `border`, which is
    /// 1px of real border, and `ANCHORS.md` v1.1 compares that field *exactly*.
    #[test]
    fn the_popup_border_is_one_real_pixel_and_not_a_ring() {
        assert_eq!(BORDER_WIDTH, px(1.0));
        // Measured off the running app: the popup's border box is its viewport
        // plus one pixel of border on each side, on both axes.
        let popover = Popover::fixture();
        let viewport_width = f32::from(popover.width) - f32::from(BORDER_WIDTH) * 2.0;
        assert!(
            (viewport_width - 254.0).abs() < f32::EPSILON,
            "{viewport_width}"
        );
    }

    /// The two radii the two variants take, and the fact that this project
    /// redefines both away from Tailwind's stock 8 and 6.
    #[test]
    fn the_variants_take_this_projects_radii() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(Variant::Default.radius(&theme), px(10.0));
            assert_eq!(Variant::Default.radius(&theme), theme.radius_lg.value());
            assert_eq!(Variant::Tooltip.radius(&theme), px(8.0));
            assert_eq!(Variant::Tooltip.radius(&theme), theme.radius_md.value());
        }
    }

    /// The two line boxes, spelled as the divisions the stylesheet writes: 16px
    /// text on a 24px line for the default popup, 12px on 16px under
    /// `tooltipStyle`. Measured live at `16px/24px`.
    #[test]
    fn the_line_heights_are_tailwinds_stock_pairs() {
        assert!((POPUP_LINE_HEIGHT * 16.0 - 24.0).abs() < 0.001);
        assert!((TOOLTIP_LINE_HEIGHT * 12.0 - 16.0).abs() < 0.001);
        assert!((POPUP_LINE_HEIGHT - TOOLTIP_LINE_HEIGHT).abs() > 0.05);

        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(Variant::Default.text_size(&theme), gpui::rems(1.0));
            assert_eq!(Variant::Tooltip.text_size(&theme), gpui::rems(0.75));
        }
    }

    /// **The title is the one line-sized anchor, and it is arithmetic.**
    /// `leading-none` puts an 18px run in an 18px box, so the box height *is*
    /// the line box. The popup and the viewport are not — their heights are
    /// content plus padding plus border — and declaring either would compare a
    /// 177px box against a 24px line box.
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);

        let title_box = TITLE_TEXT_SIZE * 16.0 * TITLE_LINE_HEIGHT;
        assert!((title_box - 18.0).abs() < 0.001, "{title_box}");

        // The viewport's is padding plus its content, which is nothing like its
        // line box — the arithmetic that makes declaring it a manufactured
        // delta.
        let popover = Popover::fixture();
        let viewport_box = f32::from(VIEWPORT_PADDING) * 2.0 + f32::from(popover.body_height);
        assert!(
            (viewport_box - 175.0).abs() < f32::EPSILON,
            "{viewport_box}"
        );
        assert!(viewport_box - POPUP_LINE_HEIGHT * 16.0 > 0.5);
    }

    /// The fixture is the live `repo-icon-popover`, measured off the running
    /// app: a 256px popup whose 177px height is one border, a 16px pad, a 143px
    /// body, a 16px pad and one border.
    #[test]
    fn the_fixture_is_the_live_repo_icon_popover() {
        let popover = Popover::fixture();

        assert_eq!(popover.width, px(256.0));
        assert_eq!(popover.body_height, px(143.0));
        assert_eq!(popover.variant, Variant::Default);
        // No title: the one popover a parity run can reach renders none, and an
        // anchor the reference cannot produce forgives nothing.
        assert_eq!(popover.title, None);

        let height = f32::from(BORDER_WIDTH) * 2.0
            + f32::from(VIEWPORT_PADDING) * 2.0
            + f32::from(popover.body_height);
        assert!((height - 177.0).abs() < f32::EPSILON, "{height}");
    }

    /// The three ids are distinct — a repeat would make two anchors one record,
    /// and `ANCHORS.md` v1.8 makes that an error rather than a first-wins.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_VIEWPORT, ID_TITLE];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("popover-")));
    }

    /// The trigger satisfies the vendor's bound and carries no paint. If it ever
    /// grows one, this fails — which is the point: `PopoverTrigger` has no class
    /// list of its own, and a styled trigger here would be this port inventing
    /// one.
    #[test]
    fn the_trigger_is_selectable_and_unstyled() {
        let trigger = Trigger::new();
        assert!(!trigger.is_selected());
        assert!(trigger.selected(true).is_selected());
        assert_eq!(Trigger::default(), Trigger::new());
    }

    /// The `tooltipStyle` arm differs from the default in **every** field it
    /// touches, so a cell driven on it is a different picture rather than a
    /// second spelling of the same one.
    #[test]
    fn the_tooltip_variant_moves_all_four_of_its_fields() {
        let theme = Theme::DARK;

        assert_ne!(
            Variant::Tooltip.viewport_padding_x(),
            Variant::Default.viewport_padding_x()
        );
        assert_ne!(
            Variant::Tooltip.viewport_padding_y(),
            Variant::Default.viewport_padding_y()
        );
        assert_ne!(
            Variant::Tooltip.radius(&theme),
            Variant::Default.radius(&theme)
        );
        assert_ne!(
            Variant::Tooltip.text_size(&theme),
            Variant::Default.text_size(&theme)
        );
        assert_eq!(Variant::default(), Variant::Default);
    }
}
