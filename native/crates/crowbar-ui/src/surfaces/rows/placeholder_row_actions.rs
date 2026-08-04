//! `placeholder-row-actions` — the reconstructed reason plus Retry/Detach…
//! pair a placeholder workspace row expands into.
//!
//! The native half of `web/src/components/layout/placeholder-row-actions.tsx`.
//! See `native/mapping/placeholder-row-actions.md`.
//!
//! # Two real `<Button>`s, composed rather than nested — `inline-error`'s
//! precedent, twice
//!
//! `inline_error::InlineError`'s own module docs establish why a call site
//! cannot nest a second `Button::render` inside another surface:
//! `AnchorSink::root` clears the driver's anchor registry as it enters
//! `prepaint`, so a nested root would discard every anchor laid out before it.
//! This component has the identical shape with the identical fix — both boxes
//! are built from `button`'s own public values ([`Size::Sm`]'s extent, padding,
//! gap, radius; [`Variant::Outline`]'s and [`Variant::Default`]'s colours) — and
//! it is the first surface in this port to need **two** variants of the same
//! composed control rather than one.
//!
//! # The reason line is wrapped prose, not a `dialog::Description`
//!
//! `placeholderReason` produces one of three real strings (`native/mapping/
//! placeholder-row-actions.md` §1 has the exact three), and at this
//! component's real 262px content width (`workspace-tree-item.tsx`'s
//! `mx-1.5 px-2.5` detail wrapper, measured against the sidebar's own 294px)
//! every one of them wraps to two or more lines — measured live, not assumed.
//! So [`ID_REASON`] takes neither v1.5 nor v1.6: it is a plain wrapped
//! paragraph, `dialog::Dialog::description_box`'s own shape, not
//! `inline-error.tsx`'s single unbreakable run.
//!
//! # Both buttons are `content_sized`, and it is the `inline-error` finding,
//! not a new one
//!
//! `size="sm"` authors no width — only the five non-icon sizes do that, and
//! `sm` is one of them (`button.rs`'s own module docs) — so each button's used
//! width is its label's max-content width, `Retry`/`Detach…` verbatim. Neither
//! is `line_sized`: `size="sm"` authors `h-8 sm:h-7`, `badge`'s rule.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::primitives::button::{ButtonState, Size, Variant};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::Theme;

/// The root anchor: `flex flex-col gap-1.5`.
pub const ID_PANEL: &str = "placeholder-row-actions";
/// The reconstructed-reason `<p>`.
pub const ID_REASON: &str = "placeholder-row-actions-reason";
/// The `flex justify-end gap-1.5` row the two buttons sit in.
pub const ID_ACTIONS: &str = "placeholder-row-actions-actions";
/// The Retry `<Button variant="outline" size="sm">`, renamed from `button`'s
/// own default id — v1.8, `git-row-badge`'s precedent, applied twice on this
/// surface.
pub const ID_RETRY: &str = "placeholder-row-actions-retry";
/// The Detach… `<Button size="sm">` — present only when `workspace.heldByPath`
/// is set.
pub const ID_DETACH: &str = "placeholder-row-actions-detach";

/// Both buttons: `size="sm"` authors no width, so each is its label's own.
pub const CONTENT_SIZED: [&str; 2] = [ID_RETRY, ID_DETACH];

/// Neither button: `size="sm"` authors `h-8 sm:h-7`, `badge`'s rule. The
/// reason line is not here either — see the module docs' [`ID_REASON`] note.
pub const LINE_SIZED: [&str; 0] = [];

/// `gap-1.5` between the reason and the action row, and — reused — between the
/// two buttons themselves.
///
/// **Mutation run**: changed `6.0` to `8.0`. Caught by the unit test
/// `every_length_is_the_compiled_spacing_multiple` — `` left: 8px right: 6px
/// ``, panicked at `placeholder_row_actions.rs:218`. **Not** caught by
/// `row_layout`'s own `the_column_is_the_gap_below_the_wrapped_reason` or
/// `the_actions_row_is_right_justified_with_the_gap_between` — both read
/// this constant rather than a second, independent literal, so they hold
/// under any wrong value that agrees with itself; the exact pixel figure is
/// pinned by the unit test alone. Recorded rather than smoothed over: a
/// worker relying on the layout suite to catch a wrong constant here would
/// be wrong. Reverted.
pub const GAP: Pixels = px(6.0);

/// `text-xs` on the reason line — this crate's `--ui-text-*` trade
/// (`native/MAPPING.md`'s own table): Tailwind's `text-xs` is
/// `theme.ui_text_sm`, one step off its own name.
pub const REASON_TEXT_SIZE: Rems = Rems(0.75);

/// `leading-relaxed` on the reason line — Tailwind's stock `1.625`,
/// unredefined by `theme.css` (checked, `detach_holder_modal::
/// DESCRIPTION_LINE_HEIGHT`'s own neighbouring check).
pub const REASON_LINE_HEIGHT: f32 = 1.625;

/// One `<PlaceholderRowActions>`.
#[derive(Clone, Debug, PartialEq)]
pub struct PlaceholderRowActions {
    /// `placeholderReason(workspace)` — one of the three real strings; see the
    /// mapping doc for the exact three and which `heldByPath` state produces
    /// each.
    pub reason: SharedString,
    /// Whether `workspace.heldByPath` is set — gates the Detach… button and
    /// therefore its anchor.
    pub detach_available: bool,
    /// Which side of the `sm` breakpoint the viewport is on — both buttons'
    /// height moves at it.
    pub breakpoint: Breakpoint,
}

impl PlaceholderRowActions {
    /// The live "held by another checkout" cell — `placeholderReason`'s first
    /// arm, both buttons rendered.
    #[must_use]
    pub fn fixture_held() -> Self {
        Self {
            reason: SharedString::new_static(
                "`fix-auth-bug` is checked out at /Users/dev/crowbar-worktrees/fix-auth-bug — \
                 detach it to let Crowbar manage this branch.",
            ),
            detach_available: true,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The live "no holder recorded" cell — `placeholderReason`'s generic
    /// third arm, Retry only.
    #[must_use]
    pub fn fixture_unheld() -> Self {
        Self {
            reason: SharedString::new_static(
                "Crowbar couldn't set up `fix-auth-bug`. Retry to provision it.",
            ),
            detach_available: false,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The panel's own box: `flex flex-col gap-1.5`.
    fn shell() -> Div {
        div().flex().flex_col().gap(GAP)
    }

    /// The reason `<p>`: `text-xs leading-relaxed text-muted-foreground`.
    fn reason_box(theme: &Theme) -> Div {
        div()
            .text_size(REASON_TEXT_SIZE)
            .line_height(relative(REASON_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The action row: `flex justify-end gap-1.5`.
    fn actions_row() -> Div {
        div().flex().justify_end().gap(GAP)
    }

    /// One composed `<Button size="sm">`, built from `button`'s own public
    /// values — the `inline_error::InlineError::retry_box` shape, generalised
    /// over [`Variant`].
    fn button_box(&self, theme: &Theme, variant: Variant) -> Div {
        let state = ButtonState::resting();
        let step = Size::Sm.type_step(theme, self.breakpoint);
        let mut element = div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .whitespace_nowrap()
            .h(Size::Sm.extent(self.breakpoint))
            .px(Size::Sm.padding_x())
            .gap(Size::Sm.gap())
            .rounded(Size::Sm.radius(theme))
            .border_1()
            .border_color(variant.border(theme, state))
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(FontWeight::MEDIUM)
            .text_color(variant.foreground(theme));

        if let Some(background) = variant.background(theme, state) {
            element = element.bg(background);
        }
        element
    }

    /// The element and its anchors: the panel, the reason, the action row and
    /// Retry always; Detach… only when [`Self::detach_available`].
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let reason = anchors.boxed_text(
            AnchorId::from(ID_REASON),
            Self::reason_box(theme),
            self.reason.clone(),
        );

        let retry = anchors.boxed_text(
            AnchorId::from(ID_RETRY).content_sized(),
            self.button_box(theme, Variant::Outline),
            SharedString::new_static("Retry"),
        );
        let mut actions_row = Self::actions_row().child(retry);
        if self.detach_available {
            let detach = anchors.boxed_text(
                AnchorId::from(ID_DETACH).content_sized(),
                self.button_box(theme, Variant::Default),
                SharedString::new_static("Detach…"),
            );
            actions_row = actions_row.child(detach);
        }
        let actions_row = anchors.boxed(ID_ACTIONS.into(), actions_row);

        let panel = Self::shell().child(reason).child(actions_row);
        anchors.root(ID_PANEL.into(), panel).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, GAP, ID_ACTIONS, ID_DETACH, ID_PANEL, ID_REASON, ID_RETRY, LINE_SIZED,
        PlaceholderRowActions, REASON_LINE_HEIGHT, REASON_TEXT_SIZE,
    };
    use crate::primitives::button::{ButtonState, Size, Variant};
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::Theme;
    use gpui::px;

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        assert_eq!(GAP, px(6.0)); // gap-1.5
        assert!((REASON_TEXT_SIZE.0 - 0.75).abs() < f32::EPSILON);
        assert!((REASON_LINE_HEIGHT - 1.625).abs() < f32::EPSILON);
    }

    /// `text-xs` is `theme.ui_text_sm`'s own number under this crate's shifted
    /// `--ui-text-*` naming (`native/MAPPING.md`'s table), not
    /// `theme.ui_text_xs`'s — pinned here as a value comparison so a future
    /// reader cannot fix the constant by reaching for the name that sounds
    /// right.
    #[test]
    fn the_reason_text_size_is_the_ui_text_sm_number_not_ui_text_xs() {
        let theme = Theme::DARK;
        assert_eq!(REASON_TEXT_SIZE, theme.ui_text_sm.value());
        assert_ne!(REASON_TEXT_SIZE, theme.ui_text_xs.value());
    }

    /// Both buttons declare `content_sized`; neither declares `line_sized`.
    #[test]
    fn both_buttons_are_content_sized_and_neither_is_line_sized() {
        assert_eq!(CONTENT_SIZED, [ID_RETRY, ID_DETACH]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The two fixtures are the two real `placeholderReason` shapes, gated on
    /// `heldByPath` exactly as the React source gates the Detach… button.
    #[test]
    fn the_fixtures_are_the_two_live_placeholder_reason_shapes() {
        let held = PlaceholderRowActions::fixture_held();
        assert!(held.detach_available);
        assert!(held.reason.contains("checked out at"));
        assert!(held.reason.contains("detach it"));

        let unheld = PlaceholderRowActions::fixture_unheld();
        assert!(!unheld.detach_available);
        assert!(unheld.reason.contains("Retry to provision it"));
        assert!(!unheld.reason.contains("checked out at"));
    }

    /// Every anchor id is distinct and namespaced under this call site's own
    /// prefix, never `button`'s own bare `"button"`.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_PANEL, ID_REASON, ID_ACTIONS, ID_RETRY, ID_DETACH];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(
            ids.iter()
                .all(|id| *id == ID_PANEL || id.starts_with("placeholder-row-actions-"))
        );
        assert!(ids.iter().all(|id| *id != "button"));
    }

    /// Both composed buttons reuse `button`'s own public values rather than
    /// re-deriving them — reads, not copies, `inline_error`'s own test of the
    /// identical claim.
    #[test]
    fn both_buttons_reuse_the_button_primitives_values() {
        let theme = Theme::DARK;
        let state = ButtonState::resting();
        assert_eq!(Size::Sm.extent(Breakpoint::Sm), px(28.0));
        assert_eq!(Size::Sm.extent(Breakpoint::Base), px(32.0));
        assert_eq!(Size::Sm.radius(&theme), theme.radius_lg.value());
        assert_eq!(
            Variant::Outline.border(&theme, state),
            theme.input,
            "outline borders with --input",
        );
        assert_eq!(Variant::Outline.foreground(&theme), theme.foreground);
        assert_eq!(
            Variant::Default.foreground(&theme),
            theme.primary_foreground,
        );
        assert_ne!(
            Variant::Outline.foreground(&theme),
            Variant::Default.foreground(&theme),
            "the two buttons must not read as the same variant",
        );
    }

    /// The Detach… button's own height moves at the `sm` breakpoint, the
    /// identical axis `inline_error`'s retry control carries.
    #[test]
    fn the_button_heights_move_at_the_breakpoint() {
        assert!(Size::Sm.extent(Breakpoint::Base) > Size::Sm.extent(Breakpoint::Sm));
    }
}
