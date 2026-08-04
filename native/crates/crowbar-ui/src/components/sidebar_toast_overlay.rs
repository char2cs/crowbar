//! `sidebar-toast-overlay` — the toast surface users actually see, and the
//! one `toast.rs` does not cover.
//!
//! The native half of
//! `web/src/components/layout/sidebar-toast-overlay.tsx`. See
//! `native/mapping/sidebar-toast-overlay.md`, and read
//! `native/mapping/toast.md` first: `crates/crowbar-ui/src/components/
//! toast.rs` ports `ui/toast.tsx`'s `AnchoredToasts`, which **no code path in
//! this application ever calls** (that file's own §2). Every real toast a
//! user sees goes through `toastManager` and this file's own hand-rolled
//! `SidebarToastItem` — a third, unrelated component. `toast.rs` does not
//! duplicate any part of this port: read side by side, `sidebar-toast-
//! overlay.tsx`'s `Toast.Root` carries `w-full overflow-hidden`, its padding
//! on the root rather than `Toast.Content`, a real dismiss `Toast.Close`
//! `toast.tsx` has no equivalent of, and a `transition-opacity` with no
//! `scale`/`before:` shadow layer at all — every one of `toast.rs`'s own
//! constants is either absent here or restated at a different number. See §5
//! for the comparison in full.
//!
//! # §1 — Two layout modes on one prop, confirmed
//!
//! `sidebarOpen` switches the **DOM parent, the positioning scheme and the
//! anchor set** — not merely a class. `true` renders `Toast.Viewport` inline,
//! `absolute inset-x-0 bottom-0`, docked to the sidebar column's own bottom
//! edge (`data-oracle-id="sidebar-toast-viewport"`); `false` renders it
//! through `Toast.Portal` into a `fixed bottom-4 left-4`/`right-4` corner
//! viewport (`data-oracle-id="sidebar-toast-viewport-fallback"`), width
//! authored at `w-72`. `Surface::root` requires one fixed string per
//! registry entry (`surface.rs`'s own doc comment), and the two boxes carry
//! **different** ids — so this port is **two** registered surfaces,
//! `sidebar-toast-overlay` and `sidebar-toast-overlay-fallback`, over one
//! shared [`SidebarToastOverlay`] struct's two render methods. `detach-
//! holder-modal`'s split from `dialog` is the precedent for "one React file,
//! two registry entries, because the registry's unique-root constraint says
//! so" — the difference here is the split is driven by the **component's
//! own** two DOM shapes, not by a call site reusing a shared primitive.
//!
//! # §2 — Pinned vs transient windowing, ported and exhaustively tested
//!
//! `SIDEBAR_TOAST_LIMIT` is **3**, but a toast with `timeout: 0` — modelled
//! here as [`ToastFixture::pinned`] — is never evicted, however many there
//! are. [`select_visible`] is [`selectVisibleToasts`]'s own algorithm, ported
//! statement for statement: every pinned toast is kept; a transient toast
//! fills a remaining slot only if one is left, walked in the list's own
//! order (newest first, the manager's own insertion order) — so when slots
//! run out it is the **oldest** transient toast that is dropped, never a
//! pinned one. Confirmed true, not merely inherited: `row_layout`'s own
//! `an_outage_keeps_the_pinned_toast_and_drops_the_oldest_transient` drives
//! exactly the shape the source's own comment names — a pinned "backend
//! unavailable" toast alongside more failure toasts than the remaining slots
//! hold — through a real render and reads the surviving titles back.
//!
//! **Only the inline (`sidebarOpen`) viewport windows at all.** The fallback
//! viewport's own JSX maps `toasts` directly, uncapped — read closely rather
//! than assumed, since it would be easy to believe the same cap applies
//! everywhere. [`SidebarToastOverlay::render_fallback`] therefore renders
//! every fixture toast handed to it; [`SidebarToastOverlay::render_inline`]
//! renders only what [`select_visible`] keeps.
//!
//! # §3 — Enter/exit is CSS-driven, and the `toast.rs` precedent extends by
//! principle only, not by value
//!
//! Both files rest on the identical base-ui mechanism —
//! `data-starting-style`/`data-ending-style` opacity transitions, a state
//! the oracle's static DOM snapshot cannot observe on either side — so
//! `toast.rs`'s own module docs' resolution ("transition out of oracle
//! reach, rest state is the target") extends here **as a principle**: this
//! port renders every toast at rest (`opacity: 1`, no `data-starting-style`/
//! `data-ending-style` selector to model at all — gpui has no CSS pseudo-
//! attribute selectors regardless). It does **not** extend as a set of
//! reusable numbers — see §5's table: the transition itself moves a
//! different property list (`opacity` alone here, `scale,opacity` plus a
//! `before:` shadow layer there), so there is nothing to import beyond the
//! shared resolution.
//!
//! # §4 — The singleton `Toast.Provider` is a React runtime fact, and it is
//! not this port's concern
//!
//! `SidebarToastOverlay`'s own doc comment (React source) states the reason
//! in full: `Toast.createToastManager()` is a stateless emitter, so a second
//! mounted provider double-renders every toast. This is true and load-
//! bearing for the running app, and it has **no counterpart on this side of
//! the port**: a `crowbar-app` cell is a pure function of its own fixture
//! list, rendered once, with no manager, no subscription and no second call
//! site to collide with. Recorded because the brief asks for it verified,
//! not because there is anything here to change.
//!
//! # §5 — `toast.rs` does not cover any part of this file
//!
//! | | `toast.rs`'s `Toast.Root` | this file's `Toast.Root` |
//! |---|---|---|
//! | width | none authored — `Positioner`'s `max-w-*` is a cap, shrinks to content | **`w-full`** — an authored, stretched width |
//! | padding | on `Toast.Content` (`px-3.5 py-3`/`px-2 py-1`) | **on `Toast.Root` itself** — `Content` carries none |
//! | overflow | not set | **`overflow-hidden`** |
//! | dismiss control | none — `toast.tsx` renders no `Toast.Close` anywhere | **`Toast.Close`**, `absolute top-3 right-3`, an `X` glyph |
//! | transition | `scale,opacity`, plus a `before:` pseudo two-shadow layer | **`opacity` alone**, `duration-200`, no `before:` layer at all |
//! | shadow | `shadow-lg/5`/`shadow-md/5` (variant-dependent) | **`shadow-lg/5`** always |
//!
//! Every row differs. Nothing here is read from `toast.rs`; every constant
//! below is this file's own.
//!
//! # Anchors, and why `SidebarToastItem` carries none
//!
//! A toast list is a queue whose length is app state — `select-item`'s own
//! precedent (`layout-denominator.md`'s brief, restated by this component's
//! own React module docs): per-item content of a live queue is a **cell**
//! property, not a surface's anchor set. `select.tsx`'s own
//! `oracleSurfaceScope` entry excludes `select-item` from the declared
//! anchors a `select` capture keeps for the identical reason. `SidebarToast
//! Item` here carries no `data-oracle-id` at all in the React source (read,
//! not assumed), so an undeclared capture of either viewport would include
//! whatever a real toast happens to render — which is exactly what a
//! **cell**, not a fixed surface shape, should determine. This port holds
//! the same line: [`ToastFixture`]'s own rendering is built by
//! [`SidebarToastOverlay::item`] and opts into no [`AnchorSink`] method at
//! all.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::AnchorSink;
use crate::theme::{Color, Theme, ui_sans_font};

/// The inline (`sidebarOpen`) viewport — this surface's root when docked to
/// the sidebar column.
pub const ID_VIEWPORT: &str = "sidebar-toast-viewport";
/// The fallback (`Toast.Portal`) viewport — this surface's root when the
/// sidebar is closed.
pub const ID_VIEWPORT_FALLBACK: &str = "sidebar-toast-viewport-fallback";

/// Neither viewport declares it — both are `w-full`/`w-72` authored widths,
/// never content-sized.
pub const CONTENT_SIZED: [&str; 0] = [];
/// Neither viewport declares it — both are flex columns, not text runs.
pub const LINE_SIZED: [&str; 0] = [];

/// `SIDEBAR_TOAST_LIMIT` — how many toasts the inline viewport shows at
/// once. **Not** a cap on the fallback viewport — see the module docs §2.
pub const SIDEBAR_TOAST_LIMIT: usize = 3;

/// `p-2` on the inline viewport.
pub const VIEWPORT_PADDING: Pixels = px(8.0);
/// `gap-2` on both viewports.
pub const VIEWPORT_GAP: Pixels = px(8.0);
/// `w-72` on the fallback viewport.
pub const FALLBACK_WIDTH: Pixels = px(288.0);
/// `bottom-4`/`left-4`/`right-4` on the fallback viewport.
pub const FALLBACK_INSET: Pixels = px(16.0);

/// `px-3.5` on an item's root (this file's own root carries the padding
/// `toast.tsx`'s `Toast.Content` does — module docs §5).
pub const ITEM_PADDING_X: Pixels = px(14.0);
/// `py-3` on an item's root.
pub const ITEM_PADDING_Y: Pixels = px(12.0);
/// `gap-2` inside `Toast.Content`.
pub const ITEM_GAP: Pixels = px(8.0);
/// `gap-0.5` in the title/description column.
pub const ITEM_COLUMN_GAP: Pixels = px(2.0);
/// `w-4` on the leading icon.
pub const ITEM_ICON_SIZE: Pixels = px(16.0);
/// `border` on an item's root — matches `.border_1()`'s own 1px, restated so
/// [`SidebarToastOverlay::item_height_estimate`] has a value to add rather
/// than a magic number.
pub const ITEM_BORDER_WIDTH: Pixels = px(1.0);
/// `text-sm` on the item — this crate's `--ui-text-*` trade
/// (`native/MAPPING.md`'s own table): Tailwind's `text-sm` is
/// `theme.ui_text_base`'s number (14px), **not** `theme.ui_text_sm`'s
/// (12px) — the mirror image of `placeholder_row_actions::REASON_TEXT_SIZE`'s
/// own note for `text-xs`. A first pass read `theme.ui_text_sm` here by
/// exactly the naming trap that file warns about, caught writing this
/// file's own mapping doc rather than by a test that existed at the time.
///
/// **Regression-checked**: reverted to `theme.ui_text_sm` after adding
/// `the_item_text_size_is_the_ui_text_base_number_not_ui_text_sm` below, and
/// the test caught it — `` 62.285713 against 68 ``, panicked at
/// `sidebar_toast_overlay.rs:528`. Re-fixed.
const ITEM_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `toast.type` — selects the icon's tint. Unanchored (module docs), painted
/// for visual completeness the way `toast.rs`'s own icon placeholder is.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    Error,
    Info,
    Loading,
    Success,
    Warning,
}

impl Kind {
    fn color(self, theme: &Theme) -> Color {
        match self {
            Self::Error => theme.destructive,
            Self::Info => theme.info,
            Self::Loading => theme.muted_foreground,
            Self::Success => theme.success,
            Self::Warning => theme.warning,
        }
    }
}

/// One entry in the queue this surface is driven with. Unanchored in full —
/// see the module docs' final section.
#[derive(Clone, Debug, PartialEq)]
pub struct ToastFixture {
    /// `toast.timeout === 0` — never evicted by [`select_visible`].
    pub pinned: bool,
    /// `toast.type`.
    pub kind: Kind,
    /// `Toast.Title`.
    pub title: SharedString,
    /// `Toast.Description`, when set.
    pub description: Option<SharedString>,
}

impl ToastFixture {
    /// A transient toast — `toast.show({ message, description, type })`'s
    /// own shape, `timeout` left at its default (nonzero).
    #[must_use]
    pub fn transient(kind: Kind, title: &'static str, description: &'static str) -> Self {
        Self {
            pinned: false,
            kind,
            title: SharedString::new_static(title),
            description: Some(SharedString::new_static(description)),
        }
    }

    /// A pinned toast — `toast.show({ duration: 0, … })`'s own shape.
    /// `ConnectionIndicator`'s "Backend unavailable" is the one live
    /// producer (`sidebar-toast-overlay.tsx`'s own doc comment).
    #[must_use]
    pub fn pinned(title: &'static str) -> Self {
        Self {
            pinned: true,
            kind: Kind::Error,
            title: SharedString::new_static(title),
            description: None,
        }
    }
}

/// `selectVisibleToasts`, ported statement for statement — see the module
/// docs §2. `toasts` is assumed newest-first, the manager's own insertion
/// order; the port makes no attempt to sort it.
///
/// **Three mutations run, all reverted after.**
///
/// 1. Replaced `toast.pinned` in the loop's own guard with `false`, so no
///    toast is ever treated as pinned. `every_pinned_toast_is_kept_even_
///    past_the_limit` caught it — `` left: 0 right: 4 ``, panicked at
///    `sidebar_toast_overlay.rs:553`.
/// 2. Replaced `limit.saturating_sub(pinned_count)` with `limit`, so a
///    pinned toast no longer shrinks the transient budget.
///    `a_pinned_toast_survives_a_wave_of_transient_failures` caught it —
///    `` left: 4 right: 3 ``, panicked at `sidebar_toast_overlay.rs:530`.
/// 3. Walked `toasts.iter().rev()` instead of `toasts` — the historical
///    `old_toasts.slice(0, 3)` bug's own shape, dropping the newest
///    transient toasts instead of the oldest.
///    `over_the_limit_with_no_pinned_toasts_keeps_the_newest` caught it —
///    `` left: ["oldest", "third", "middle"] right: ["newest", "middle",
///    "third"] ``, panicked at `sidebar_toast_overlay.rs:513`.
#[must_use]
pub fn select_visible(toasts: &[ToastFixture], limit: usize) -> Vec<&ToastFixture> {
    let pinned_count = toasts.iter().filter(|toast| toast.pinned).count();
    let mut transient_slots = limit.saturating_sub(pinned_count);

    let mut visible = Vec::new();
    for toast in toasts {
        if toast.pinned {
            visible.push(toast);
            continue;
        }
        if transient_slots > 0 {
            transient_slots -= 1;
            visible.push(toast);
        }
    }
    visible
}

/// Which corner the fallback viewport docks to — `sidebarSide`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Side {
    #[default]
    Left,
    Right,
}

/// One `<SidebarToastOverlay>` cell: the queue it is driven with, rendered
/// through whichever of the two layout modes owns this call.
#[derive(Clone, Debug, PartialEq)]
pub struct SidebarToastOverlay {
    /// The toast queue, newest first — `Toast.useToastManager().toasts`'s
    /// own order.
    pub toasts: Vec<ToastFixture>,
}

impl SidebarToastOverlay {
    /// The live outage shape `sidebar-toast-overlay.tsx`'s own comment
    /// names: a pinned "Backend unavailable" toast, plus more transient
    /// failure toasts than the two remaining slots (`SIDEBAR_TOAST_LIMIT`
    /// minus the one pinned toast) can hold.
    #[must_use]
    pub fn fixture_outage() -> Self {
        Self {
            toasts: vec![
                ToastFixture::pinned("Backend unavailable"),
                ToastFixture::transient(Kind::Error, "Couldn't retry fix-auth-bug", "ECONNREFUSED"),
                ToastFixture::transient(Kind::Error, "Couldn't rename main", "ECONNREFUSED"),
                ToastFixture::transient(Kind::Error, "Couldn't detach ci", "ECONNREFUSED"),
                ToastFixture::transient(Kind::Error, "Couldn't import repo", "ECONNREFUSED"),
            ],
        }
    }

    /// A single, ordinary transient toast — `toast.success(…)`'s shape.
    #[must_use]
    pub fn fixture_single() -> Self {
        Self {
            toasts: vec![ToastFixture::transient(
                Kind::Success,
                "Saved",
                "Your changes have been saved.",
            )],
        }
    }

    /// An estimate of the inline viewport's own content height — used only
    /// to size the `row_layout` harness's window, the same caveat `toast::
    /// Toast::popup_height` and `detach_holder_modal`'s own estimate both
    /// carry: never compared against a reference, because none exists.
    #[must_use]
    pub fn estimated_stack_height(&self, theme: &Theme, limit: usize) -> Pixels {
        let visible = select_visible(&self.toasts, limit);
        let mut height = f32::from(VIEWPORT_PADDING) * 2.0;
        for (index, toast) in visible.iter().enumerate() {
            if index > 0 {
                height += f32::from(VIEWPORT_GAP);
            }
            height += f32::from(Self::item_height_estimate(toast, theme));
        }
        px(height)
    }

    /// One item's own estimated height — see
    /// [`Self::estimated_stack_height`]'s caveat. Includes the item's own
    /// `border_1()` (2px, top and bottom) — omitted on a first pass and
    /// caught by `row_layout::sidebar_toast_overlay::
    /// the_outage_fixture_renders_shorter_here_than_on_the_uncapped_sibling`,
    /// which measured `199px` against an estimate of `193.7px` for three
    /// items — a ~5.3px gap, short of the missing `3 × 2px = 6px` only
    /// because the line-height arithmetic itself already carries its own
    /// sub-pixel rounding.
    #[must_use]
    pub fn item_height_estimate(toast: &ToastFixture, theme: &Theme) -> Pixels {
        let line = theme.ui_text_base.value().0 * 16.0 * ITEM_LINE_HEIGHT;
        let mut column = line;
        if toast.description.is_some() {
            column += f32::from(ITEM_COLUMN_GAP) + line;
        }
        px(f32::from(ITEM_BORDER_WIDTH) * 2.0 + f32::from(ITEM_PADDING_Y) * 2.0 + column)
    }

    /// The inline viewport, docked to a `column_height`-tall relative
    /// wrapper standing in for the real sidebar column — the harness's own
    /// scaffolding, unanchored, `sidebar_carousel`'s own `--height` shape.
    /// Windows through [`select_visible`] first (module docs §2).
    #[must_use]
    pub fn render_inline(
        &self,
        column_height: Pixels,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let visible = select_visible(&self.toasts, SIDEBAR_TOAST_LIMIT);

        let mut viewport = div()
            .absolute()
            .left(px(0.0))
            .bottom(px(0.0))
            // Authored rather than left to a dual `left`/`right` inset to
            // resolve its own width — `resizable`'s and `detach_holder_
            // modal`'s own "`w-full` by hand" workaround for the identical
            // taffy limitation `button::ICON_MARGIN_X`'s module docs name at
            // length, applied on the horizontal axis here rather than the
            // vertical one P3.59 first met it on.
            .w(relative(1.0))
            .flex()
            .flex_col_reverse()
            .gap(VIEWPORT_GAP)
            .p(VIEWPORT_PADDING);
        for toast in &visible {
            viewport = viewport.child(Self::item(toast, theme));
        }
        let viewport = anchors.root(ID_VIEWPORT.into(), viewport);

        div()
            .relative()
            .w(relative(1.0))
            .h(column_height)
            .child(viewport)
            .into_any_element()
    }

    /// The fallback viewport, `Toast.Portal`'d in the reference to a fixed
    /// corner. No `.relative()` ancestor exists in the row harness, so
    /// `.absolute()` resolves against the window itself — `fps_overlay`'s
    /// own precedent for `position: fixed` with nothing establishing a
    /// containing block, and the reason this surface is registered
    /// `full_bleed` (see its own module docs). **Uncapped** — module docs
    /// §2.
    #[must_use]
    pub fn render_fallback(
        &self,
        side: Side,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let mut viewport = div()
            .absolute()
            .bottom(FALLBACK_INSET)
            .w(FALLBACK_WIDTH)
            .flex()
            .flex_col()
            .gap(VIEWPORT_GAP);
        viewport = match side {
            Side::Left => viewport.left(FALLBACK_INSET),
            Side::Right => viewport.right(FALLBACK_INSET),
        };
        for toast in &self.toasts {
            viewport = viewport.child(Self::item(toast, theme));
        }
        anchors
            .root(ID_VIEWPORT_FALLBACK.into(), viewport)
            .into_any_element()
    }

    /// One `SidebarToastItem`, entirely unanchored — module docs' final
    /// section. Built for visual completeness only, the way `toast::Toast`'s
    /// own icon and action placeholders are: real, rendered, compared to
    /// nothing.
    fn item(toast: &ToastFixture, theme: &Theme) -> Div {
        let icon = div()
            .flex_shrink_0()
            .w(ITEM_ICON_SIZE)
            .h(ITEM_ICON_SIZE)
            .rounded(px(3.0))
            .bg(toast.kind.color(theme));

        let mut column = div()
            .flex()
            .flex_col()
            .min_w(px(0.0))
            .gap(ITEM_COLUMN_GAP)
            .child(
                div()
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(theme.popover_foreground)
                    .child(toast.title.clone()),
            );
        if let Some(description) = &toast.description {
            column = column.child(
                div()
                    .text_color(theme.muted_foreground)
                    .child(description.clone()),
            );
        }

        let row = div()
            .flex()
            .min_w(px(0.0))
            .gap(ITEM_GAP)
            .child(icon)
            .child(column);

        div()
            .relative()
            .w(relative(1.0))
            .rounded(theme.radius_lg.value())
            .border_1()
            .border_color(theme.border)
            .bg(theme.popover)
            .px(ITEM_PADDING_X)
            .py(ITEM_PADDING_Y)
            .shadow_lg()
            .font(ui_sans_font(theme))
            .text_size(theme.ui_text_base.value())
            .line_height(relative(ITEM_LINE_HEIGHT))
            .child(row)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        FALLBACK_INSET, FALLBACK_WIDTH, ID_VIEWPORT, ID_VIEWPORT_FALLBACK, Kind,
        SIDEBAR_TOAST_LIMIT, Side, SidebarToastOverlay, ToastFixture, VIEWPORT_GAP,
        VIEWPORT_PADDING, select_visible,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// `text-sm` is `theme.ui_text_base`'s own number under this crate's
    /// shifted `--ui-text-*` naming (`native/MAPPING.md`'s table), not
    /// `theme.ui_text_sm`'s — the mirror image of `placeholder_row_actions`'s
    /// identical check for `text-xs`/`ui_text_sm`. Checked through
    /// `item_height_estimate`'s own output, since the size is read inline at
    /// two call sites rather than named as one exported constant.
    #[test]
    fn the_item_text_size_is_the_ui_text_base_number_not_ui_text_sm() {
        let theme = Theme::DARK;
        let toast = ToastFixture::transient(Kind::Info, "t", "d");

        let base_height = f32::from(SidebarToastOverlay::item_height_estimate(&toast, &theme));
        // The line contribution using the WRONG token, hand-computed, to
        // prove the two really do disagree rather than coincide.
        let wrong_line = theme.ui_text_sm.value().0 * 16.0 * (1.25 / 0.875);
        let right_line = theme.ui_text_base.value().0 * 16.0 * (1.25 / 0.875);
        assert_ne!(wrong_line.to_bits(), right_line.to_bits());

        // The estimate must be built from the right one: two lines (title +
        // description) plus the column gap plus padding plus border.
        let expected = 2.0 * f32::from(crate::components::sidebar_toast_overlay::ITEM_BORDER_WIDTH)
            + 2.0 * f32::from(crate::components::sidebar_toast_overlay::ITEM_PADDING_Y)
            + f32::from(crate::components::sidebar_toast_overlay::ITEM_COLUMN_GAP)
            + 2.0 * right_line;
        assert!(
            (base_height - expected).abs() < 1e-3,
            "{base_height} against {expected}"
        );
    }

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        assert_eq!(VIEWPORT_PADDING, px(8.0)); // p-2
        assert_eq!(VIEWPORT_GAP, px(8.0)); // gap-2
        assert_eq!(FALLBACK_WIDTH, px(288.0)); // w-72
        assert_eq!(FALLBACK_INSET, px(16.0)); // bottom-4 / left-4 / right-4
        assert_eq!(SIDEBAR_TOAST_LIMIT, 3);
    }

    /// Fewer toasts than the limit: everything is kept, in order.
    #[test]
    fn under_the_limit_every_toast_is_kept() {
        let toasts = vec![
            ToastFixture::transient(Kind::Info, "a", "d"),
            ToastFixture::transient(Kind::Info, "b", "d"),
        ];
        let visible = select_visible(&toasts, SIDEBAR_TOAST_LIMIT);
        assert_eq!(visible.len(), 2);
        assert_eq!(visible[0].title, "a");
        assert_eq!(visible[1].title, "b");
    }

    /// More transient toasts than the limit, none pinned: the newest
    /// (earliest in the list) survive, the oldest (latest) are dropped —
    /// `old_toasts.slice(0, 3)`'s own bug this port must not reintroduce.
    #[test]
    fn over_the_limit_with_no_pinned_toasts_keeps_the_newest() {
        let toasts = vec![
            ToastFixture::transient(Kind::Error, "newest", "d"),
            ToastFixture::transient(Kind::Error, "middle", "d"),
            ToastFixture::transient(Kind::Error, "third", "d"),
            ToastFixture::transient(Kind::Error, "oldest", "d"),
        ];
        let visible = select_visible(&toasts, SIDEBAR_TOAST_LIMIT);
        let titles: Vec<_> = visible.iter().map(|t| t.title.as_ref()).collect();
        assert_eq!(titles, ["newest", "middle", "third"]);
    }

    /// The outage shape the React source's own comment names: a pinned
    /// toast survives no matter how many transient failures follow it, and
    /// exactly `limit - 1` of the newest transient toasts fill the rest.
    #[test]
    fn a_pinned_toast_survives_a_wave_of_transient_failures() {
        let overlay = SidebarToastOverlay::fixture_outage();
        assert_eq!(
            overlay.toasts.len(),
            5,
            "1 pinned + 4 transient in the fixture"
        );

        let visible = select_visible(&overlay.toasts, SIDEBAR_TOAST_LIMIT);
        assert_eq!(visible.len(), SIDEBAR_TOAST_LIMIT);
        assert!(visible[0].pinned);
        assert_eq!(visible[0].title, "Backend unavailable");
        // The two remaining slots go to the two newest transient toasts —
        // the ones immediately following the pinned toast in the fixture.
        assert!(!visible[1].pinned);
        assert_eq!(visible[1].title, "Couldn't retry fix-auth-bug");
        assert!(!visible[2].pinned);
        assert_eq!(visible[2].title, "Couldn't rename main");
    }

    /// Multiple pinned toasts are **all** kept even past the limit — the
    /// windowing only ever trims the transient pool, never the pinned one.
    #[test]
    fn every_pinned_toast_is_kept_even_past_the_limit() {
        let toasts = vec![
            ToastFixture::pinned("p1"),
            ToastFixture::pinned("p2"),
            ToastFixture::pinned("p3"),
            ToastFixture::pinned("p4"),
            ToastFixture::transient(Kind::Info, "t1", "d"),
        ];
        let visible = select_visible(&toasts, SIDEBAR_TOAST_LIMIT);
        // All four pinned toasts, and no transient slot left for `t1`.
        assert_eq!(visible.len(), 4);
        assert!(visible.iter().all(|t| t.pinned));
    }

    /// An empty queue windows to an empty list.
    #[test]
    fn an_empty_queue_stays_empty() {
        assert!(select_visible(&[], SIDEBAR_TOAST_LIMIT).is_empty());
    }

    /// A `limit` of zero keeps every pinned toast and no transient one —
    /// `saturating_sub` rather than a panicking subtraction, exercised
    /// directly rather than trusted by inspection.
    #[test]
    fn a_zero_limit_still_keeps_pinned_toasts() {
        let toasts = vec![
            ToastFixture::pinned("p"),
            ToastFixture::transient(Kind::Info, "t", "d"),
        ];
        let visible = select_visible(&toasts, 0);
        assert_eq!(visible.len(), 1);
        assert!(visible[0].pinned);
    }

    /// The two root anchors are distinct, and namespaced identically —
    /// `ANCHORS.md` v1.8's "unique root per surface" requirement, the
    /// concrete reason this component is two registered surfaces.
    #[test]
    fn the_two_viewport_anchors_are_distinct() {
        assert_ne!(ID_VIEWPORT, ID_VIEWPORT_FALLBACK);
        assert!(ID_VIEWPORT_FALLBACK.starts_with(ID_VIEWPORT));
    }

    /// `Side` defaults to `Left` — `sidebarSide`'s own prop default in the
    /// React source.
    #[test]
    fn side_defaults_to_left() {
        assert_eq!(Side::default(), Side::Left);
    }

    /// The two fixtures are the shapes their own doc comments claim.
    #[test]
    fn the_fixtures_match_their_own_claims() {
        let single = SidebarToastOverlay::fixture_single();
        assert_eq!(single.toasts.len(), 1);
        assert!(!single.toasts[0].pinned);

        let outage = SidebarToastOverlay::fixture_outage();
        assert_eq!(outage.toasts.iter().filter(|t| t.pinned).count(), 1);
        assert_eq!(outage.toasts.iter().filter(|t| !t.pinned).count(), 4);
        assert!(
            outage.toasts.len() - outage.toasts.iter().filter(|t| t.pinned).count()
                > SIDEBAR_TOAST_LIMIT - 1,
            "the fixture must genuinely exceed the remaining transient slots, or the outage \
             test above proves nothing",
        );
    }
}
