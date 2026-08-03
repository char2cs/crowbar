//! `row_base` — the shared row chrome every sidebar row composes: the base
//! shell (height, padding, gap, border, radius, type) and the two idle/
//! selected variants every sidebar row renders identically.
//!
//! The native half of `web/src/components/layout/workspace-row-base.ts`.
//! **Not a measurable surface of its own** — the source file has no JSX and
//! no `data-oracle-id` anywhere, so this module carries no anchor, no
//! `SURFACE`, and paints nothing by itself. It exists so
//! [`super::project_home_row`] and every future consumer
//! (`workspace-tree-item.tsx`, `repo-section.tsx`, `pending-create-row.tsx`
//! — `native/mapping/layout-denominator.md` §8's Cluster 8) read the *same*
//! numbers, the way the source's own comment insists on: "Keep both in sync
//! here, never inline." Every value below came out of the app's own
//! `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes.
//!
//! # `mx-1.5`/`my-0.5` are data here, not baked into [`base`]
//!
//! `ROW_BASE`'s own outer margin only has somewhere to show up in the
//! context of a *surrounding list*: a row captured as its own root (every
//! consumer so far, [`super::project_home_row`] included) has its bounds
//! reported starting at its own border box — `getBoundingClientRect()`
//! excludes margin on the React side exactly as a gpui anchor's own bounds
//! exclude it on this one — so a standalone row has no anchor these two
//! lengths could move, and [`base`] does not apply them. [`MARGIN_X`] and
//! [`MARGIN_Y`] are exported anyway, as the numbers a *list*-shaped consumer
//! (`project-switcher-panel`, eventually `workspace-tree`) applies to each
//! row itself, the way `project_switcher_panel.rs` does.
//!
//! # `focus-visible:ring-2 ring-offset-1` is invisible, `button.rs`'s own
//! finding
//!
//! Two box-shadows, and `native/oracle/ANCHORS.md` §6 has no field for
//! either. Not modelled. `cursor-pointer`/`select-none`/`outline-none` are
//! not visual properties either.
//!
//! # Hover is real and has no seam here
//!
//! `ROW_INACTIVE`'s `hover:bg-accent` and `ROW_SUB_ACTION`'s
//! `hover:bg-sidebar-element-hover hover:text-foreground` are colour-only
//! rules a pointer drives, with no captured reference — synthetic pointer
//! events are denied on this project's machines, `button.rs`'s own finding —
//! and, unlike `button.rs`'s own `Interaction`, nothing here carries a
//! runtime hover field to express it: the same shape `context_pill.rs`'s
//! trigger hover already is. Every consumer of this module renders its
//! resting picture only; each surface's own `unmodelled` list is where that
//! is formally declared.

use gpui::{Div, FontWeight, Pixels, Styled as _, div, px, relative};

use super::button;
use crate::theme::{Color, Theme};

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `h-9` — the row's own authored height.
pub const HEIGHT: Pixels = px(SPACING * 9.0);
/// `px-1.5`.
pub const PADDING_X: Pixels = px(SPACING * 1.5);
/// `gap-1.5`.
pub const GAP: Pixels = px(SPACING * 1.5);
/// `mx-1.5` — not applied by [`base`]. See the module docs.
pub const MARGIN_X: Pixels = px(SPACING * 1.5);
/// `my-0.5` — not applied by [`base`]. See the module docs.
pub const MARGIN_Y: Pixels = px(SPACING * 0.5);

/// `text-[13px]` — an arbitrary literal, no paired `line-height` utility, so
/// the box is CSS `normal` and the differ compares it against the
/// reference's own measured line box (v1.6) rather than against a number
/// this port asserts. A concrete ratio is still supplied below
/// ([`LINE_HEIGHT_RELATIVE`]) because gpui still has to paint *something*.
pub const TEXT: Pixels = px(13.0);
/// The ratio [`base`] paints `text-[13px]` at.
///
/// `text-[13px]` under `font-mono` (`var(--editor-font-family)`) is the
/// combination `context_pill::LARGE_LINE_HEIGHT`/`workspace_switcher::
/// CONTENT_HEIGHT` already measured at 18px — `18.0 / 13.0`, transferred
/// rather than re-derived. The one label in this item that is *not*
/// `font-mono` (`project-switcher-panel.tsx`'s "Import project" row) has no
/// equivalent live measurement for the default UI sans stack at this size;
/// the same ratio is carried over as the best available approximation,
/// flagged here rather than silently asserted, and the anchor it paints is
/// still declared `line_sized` so the differ compares against whatever the
/// reference's own font metrics actually produce.
pub const LINE_HEIGHT_RELATIVE: f32 = 18.0 / 13.0;

/// `ROW_SUB_ACTION`'s call-site override in this item: a fixed `size-6`
/// (24px) box at every width — see [`super::project_home_row`]'s module
/// docs for the tailwind-merge arithmetic that makes it win over the
/// `icon-xs` variant's own `size-7 sm:size-6`.
pub const SUB_ACTION_SIZE: Pixels = px(SPACING * 6.0);
/// The glyph a `ROW_SUB_ACTION` button wraps — a call site's own `size-3`.
pub const SUB_ACTION_GLYPH: Pixels = px(SPACING * 3.0);

/// `ROW_BASE`: `flex cursor-pointer select-none items-center gap-1.5
/// rounded-lg border h-9 px-1.5 text-[13px] font-medium outline-none
/// focus-visible:…`. `mx-1.5 my-0.5` are not applied — see the module docs.
/// The border's **colour** is left to the caller ([`active`]/[`inactive`]);
/// its **width** is always [`button::BORDER_WIDTH`], `button.rs`'s own
/// headline finding reproduced here (`border` with no variant able to
/// change its width, only its colour).
#[must_use]
pub fn base(theme: &Theme) -> Div {
    div()
        .flex()
        .items_center()
        .gap(GAP)
        .rounded(theme.radius_lg.value())
        .border(button::BORDER_WIDTH)
        .h(HEIGHT)
        .px(PADDING_X)
        .text_size(TEXT)
        .line_height(relative(LINE_HEIGHT_RELATIVE))
        .font_weight(FontWeight::MEDIUM)
}

/// `ROW_ACTIVE`: `border-background bg-background text-foreground shadow-xs
/// shadow-black/10 not-disabled:inset-shadow-[…] active:inset-shadow-[…]
/// active:shadow-none`. The three shadow layers are invisible
/// (`ANCHORS.md` §6, the same call `button.rs`'s own `::before` overlay
/// shadows make); the modelled fields are the border colour, the
/// background and the text colour.
#[must_use]
pub fn active(theme: &Theme) -> Div {
    base(theme)
        .border_color(theme.background)
        .bg(theme.background)
        .text_color(theme.foreground)
}

/// `ROW_INACTIVE`: `border-transparent text-foreground hover:bg-accent`.
/// The hover rule is not modelled — see the module docs. Resting: no
/// background at all (there is no un-prefixed `bg-*` in this class), a
/// transparent but still one-pixel-wide border (`button.rs`'s own
/// `border-transparent` finding), `theme.foreground` text.
#[must_use]
pub fn inactive(theme: &Theme) -> Div {
    base(theme)
        .border_color(Color::TRANSPARENT)
        .text_color(theme.foreground)
}

/// One `ROW_SUB_ACTION` button, as this item's two call sites override it:
/// `inline-flex shrink-0 cursor-pointer rounded-lg p-1.5
/// text-muted-foreground hover:… focus-visible:…`, plus the call site's own
/// `size-6`. See [`super::project_home_row`]'s module docs for why `size-6`
/// wins over the `icon-xs` variant's own box at every width, and why
/// `rounded-lg` wins over `icon-xs`'s own `rounded-md` — the same
/// arithmetic `button.rs` already measured for the workspace header's two
/// `icon-xs` buttons (host **10px**, `::before` **7px** — irrelevant here,
/// since this box has no `::before` overlay to begin with; only the host
/// radius carries over).
///
/// Built off `button::Size`/`RadiusClass`'s public values without going
/// through `Button::render`'s own anchor machinery —
/// `sidebar_project_header.rs`'s precedent, for the same reason: nesting a
/// second `anchors.root(…)` inside this surface's own would contest which
/// anchor `ANCHORS.md` §4 means.
#[must_use]
pub fn sub_action_box(theme: &Theme) -> Div {
    div()
        .flex()
        .flex_shrink_0()
        .items_center()
        .justify_center()
        .w(SUB_ACTION_SIZE)
        .h(SUB_ACTION_SIZE)
        .rounded(button::RadiusClass::Lg.value(theme))
        .border(button::BORDER_WIDTH)
        .border_color(Color::TRANSPARENT)
        .text_color(theme.muted_foreground)
}

/// A `ROW_SUB_ACTION` button's own empty glyph box — the call site's own
/// `size-3` `<svg>`, unpainted (no native equivalent; drawing a substitute
/// would put a shape on screen for a perceptual oracle to converge on, the
/// convention every icon in this port already follows).
#[must_use]
pub fn sub_action_glyph() -> Div {
    div()
        .flex_shrink_0()
        .w(SUB_ACTION_GLYPH)
        .h(SUB_ACTION_GLYPH)
}

/// A truncating row label's own box: `min-w-0 flex-1 truncate`, at
/// `text-[13px] font-medium` (`ROW_BASE`'s own type, which every label in
/// this item inherits) and the caller's own colour. **The font family is
/// the caller's**: this component's own two labels split between
/// `font-mono` and the default UI sans stack, and the two are set through
/// different gpui calls (`.font_family(&str)` vs `.font(Font)`), so the
/// caller chains its own family call onto the [`Div`] this returns rather
/// than this function choosing one signature for both.
///
/// The text run goes inside as a bare [`super::AnchorSink::text`] call, not
/// [`super::AnchorSink::boxed_text`] — `git_status_row.rs`'s own
/// `name_and_directory` established why: a block child with no explicit
/// height takes its box from its own line box (`items-center`, not
/// `stretch`, is what makes that true regardless of the row's own authored
/// [`HEIGHT`]), so the run's box already *is* the span's box, and a second
/// wrapping anchor would record the same bounds twice.
#[must_use]
pub fn label_container(color: Color) -> Div {
    div()
        .min_w(px(0.0))
        .flex_1()
        .overflow_hidden()
        .text_size(TEXT)
        .line_height(relative(LINE_HEIGHT_RELATIVE))
        .font_weight(FontWeight::MEDIUM)
        .text_color(color)
}

#[cfg(test)]
mod tests {
    use super::{
        GAP, HEIGHT, LINE_HEIGHT_RELATIVE, MARGIN_X, MARGIN_Y, PADDING_X, SUB_ACTION_GLYPH,
        SUB_ACTION_SIZE, TEXT,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple_or_a_literal() {
        const STEP: f32 = 4.0;
        assert_eq!(HEIGHT, px(STEP * 9.0)); // h-9
        assert_eq!(PADDING_X, px(STEP * 1.5)); // px-1.5
        assert_eq!(GAP, px(STEP * 1.5)); // gap-1.5
        assert_eq!(MARGIN_X, px(STEP * 1.5)); // mx-1.5
        assert_eq!(MARGIN_Y, px(STEP * 0.5)); // my-0.5
        assert_eq!(SUB_ACTION_SIZE, px(STEP * 6.0)); // size-6
        assert_eq!(SUB_ACTION_GLYPH, px(STEP * 3.0)); // size-3
        // The one literal, not a spacing multiple.
        assert_eq!(TEXT, px(13.0));
    }

    /// `18.0 / 13.0`, the transferred ratio, not re-derived.
    #[test]
    fn the_line_height_ratio_matches_the_transferred_measurement() {
        let expected = 18.0_f32 / 13.0_f32;
        assert!((LINE_HEIGHT_RELATIVE - expected).abs() < f32::EPSILON);
    }
}
