//! `detach-holder-modal` — a **call site** of `dialog.tsx`'s own primitive,
//! not a distinct React file.
//!
//! `web/src/components/layout/detach-holder-modal.tsx` renders exclusively
//! through `dialog.tsx`'s `Dialog`/`DialogPopup`/`DialogHeader`/`DialogTitle`/
//! `DialogDescription`/`DialogFooter` — the identical primitive
//! `crates/crowbar-ui/src/components/dialog.rs` already wraps. It is **not**
//! a second React component the way `alert-dialog.tsx` is: `dialog.tsx`'s own
//! `data-oracle-id`s (`"dialog-popup"`, `"dialog-header"`, …) are hardcoded
//! literals in that file, not passed through from a call site, so the *real*
//! DOM this call site paints carries the *same* `dialog-*` ids
//! `add-repository-modal` does — confirmed by reading `dialog.tsx` directly,
//! not assumed. `native/mapping/detach-holder-modal.md` §0 records this
//! finding in full, including why this module's own anchor ids are a
//! `detach-holder-modal-*` namespace anyway (a registry constraint, not a
//! rendering difference — see that section before treating the ids below as
//! evidence of a second primitive).
//!
//! # What is genuinely this call site's own, not `dialog`'s
//!
//! Two real deltas, both `className` overrides `dialog.tsx`'s own primitive
//! does not carry, and the reason this module exists rather than reusing
//! `dialog::Dialog` unchanged:
//!
//! | | `dialog`'s `add-repository-modal` cell | `detach-holder-modal.tsx` |
//! |---|---|---|
//! | `DialogHeader` | `p-6` (24px, all four sides) | `p-6 pr-10` — tailwind-merge overrides only the **right** side, to 40px; top/left/bottom stay 24 |
//! | `DialogDescription` | *(not rendered)* | `leading-relaxed` (1.625), not the primitive's own default `text-sm` line height (1.25/0.875) |
//!
//! Every other constant below is `dialog`'s own, restated here because the
//! two are measured as **separate surfaces** (`ANCHORS.md` compares by id,
//! and `Surface::root` has to be unique per the registry's own test — see the
//! mapping doc §0) — not because a second number was derived. This is
//! `alert_dialog.rs`'s own justification for restating rather than importing,
//! applied one level down: there the two components' whole *value sets*
//! coincided; here only the sizes this call site does not override do.
//!
//! # The footer is unmodelled content, exactly `dialog`'s is
//!
//! `detach-holder-modal.tsx`'s `DialogFooter` holds two default-sized
//! `Button`s (`variant="ghost"` "Cancel", default "Detach") — no `size` prop
//! on either, so [`FOOTER_CONTENT_HEIGHT_DEFAULT`]'s value (32px) is
//! `button::Size::Default` at the `sm` breakpoint, the identical number
//! `dialog`'s own default carries for the identical reason (`add-repository-
//! modal`'s two buttons are also default-sized). Neither button is rendered
//! as its own anchor — `dialog::Dialog::footer` never renders one for a real
//! call site's buttons either, and this module holds the same line.
//!
//! # The description is one text run, and the mono-font spans are inert to
//! this contract
//!
//! The real JSX interleaves plain prose with two `<span className="font-mono
//! text-foreground">` runs (the held-by path, the branch name) inside one
//! `DialogDescription`. `font-mono`/`text-foreground` on an inline span move
//! glyph rendering *inside* the description's own box, not the box's bounds —
//! the identical reasoning `alert-dialog.md` §2 gives for `text-center`/
//! `sm:text-left`: real, and not a field `ANCHORS.md` §3 tracks. This module
//! therefore renders the description as one flattened [`SharedString`],
//! losing only the run-level font distinction.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, Window, div, px, relative,
};
use gpui_component::dialog::Dialog as GpuiDialog;

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor. **Not** `"dialog-popup"` — see the module docs' §0
/// pointer and `native/mapping/detach-holder-modal.md` §0 for why the real
/// DOM would carry that id and this surface still cannot.
pub const ID_POPUP: &str = "detach-holder-modal-popup";
/// `DialogHeader`, always rendered — the one live call site always nests a
/// title and a description.
pub const ID_HEADER: &str = "detach-holder-modal-header";
/// `DialogTitle`.
pub const ID_TITLE: &str = "detach-holder-modal-title";
/// `DialogDescription`.
pub const ID_DESCRIPTION: &str = "detach-holder-modal-description";
/// `DialogFooter`, always rendered.
pub const ID_FOOTER: &str = "detach-holder-modal-footer";

/// See `dialog::CONTENT_SIZED` — identical reasoning, identical empty answer:
/// nothing here is a box whose used width is a text run's max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// See `dialog::LINE_SIZED` — the title, and only the title, for the
/// identical `leading-none` arithmetic.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `border` on the popup, and the vendor's own unconditional `border_1()` on
/// the outer box this crate neutralises. **Identical to
/// `dialog::BORDER_WIDTH`.**
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `p-6`'s top/left/bottom, unaffected by `pr-10`. **Identical to
/// `dialog::HEADER_PADDING`.**
pub const HEADER_PADDING: Pixels = px(24.0);

/// `pr-10` — the one side `p-6` does **not** win on tailwind-merge, because a
/// call site's axis-specific utility beats the shorthand's contribution to
/// that one side while leaving the other three. **This call site's own
/// delta**, not `dialog`'s: `add-repository-modal` never overrides its
/// header's padding.
pub const HEADER_PADDING_RIGHT: Pixels = px(40.0);

/// `gap-2` inside the header. **Identical to `dialog::HEADER_GAP`** — and,
/// unlike `dialog`'s own reachable cell (title only), **live** here: this
/// call site always renders both a title and a description, so the gap
/// between them actually moves a bound.
pub const HEADER_GAP: Pixels = px(8.0);

/// `px-6` on the footer. **Identical to `dialog::FOOTER_PADDING_X`.**
pub const FOOTER_PADDING_X: Pixels = px(24.0);

/// `py-4` on the footer. **Identical to `dialog::FOOTER_PADDING_Y`.**
pub const FOOTER_PADDING_Y: Pixels = px(16.0);

/// `gap-2` between the footer's own buttons. **Identical to
/// `dialog::FOOTER_GAP`** and, as there, inert on this module's own opaque
/// footer content box (one child, not two) — see `dialog::Dialog::footer`'s
/// doc for why a call site's real buttons are never individually anchored.
pub const FOOTER_GAP: Pixels = px(8.0);

/// `DialogViewport`'s `p-4`. **Identical to `dialog::VIEWPORT_PADDING`** —
/// same primitive, same class.
pub const VIEWPORT_PADDING: Pixels = px(16.0);

/// `button::Size::Default` at the `sm` breakpoint (`h-9 sm:h-8` → 32px) —
/// both of this call site's buttons take no `size` prop. **Identical to
/// `dialog`'s own default footer content height**, for the identical reason:
/// `add-repository-modal`'s two buttons are default-sized too.
pub const FOOTER_CONTENT_HEIGHT_DEFAULT: Pixels = px(32.0);

/// `leading-none` on the title. **Identical to `dialog`'s.**
const TITLE_LINE_HEIGHT: f32 = 1.0;

/// Inherited `text-base`'s line height on the popup. **Identical to
/// `dialog::POPUP_LINE_HEIGHT`** — same primitive, same inheritance path.
const POPUP_LINE_HEIGHT: f32 = 1.5;

/// `leading-relaxed` on the description — Tailwind's stock `1.625`,
/// unredefined by `theme.css` (checked: no `--leading-*` custom property
/// anywhere in `web/src/styles/theme.css`). **This call site's own delta**:
/// `dialog`'s own `DESCRIPTION_LINE_HEIGHT` is the primitive's default
/// (`text-sm`'s stock `1.25/0.875`), which this call site's `className`
/// overrides.
const DESCRIPTION_LINE_HEIGHT: f32 = 1.625;

/// `bg-muted/72` on the footer. **Identical to `dialog::FOOTER_BG_TINT`.**
const FOOTER_BG_TINT: f32 = 72.0;

/// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]` on the footer. **Identical to
/// `dialog::FOOTER_RADIUS`.**
const FOOTER_RADIUS: Pixels = px(17.0);

/// `DialogPopup`, `DialogHeader`, `DialogTitle`, `DialogDescription` and
/// `DialogFooter`, as this one call site configures them.
///
/// See the module docs for what makes this a separate module from
/// `dialog::Dialog` rather than a second construction of it: two real
/// `className` overrides (`pr-10` on the header, `leading-relaxed` on the
/// description) that `dialog::Dialog`'s struct has no field for.
#[derive(Clone, Debug, PartialEq)]
pub struct DetachHolderModal {
    /// `max-w-md` (no `sm:` prefix, unlike `dialog`'s own `sm:max-w-md`) —
    /// the same **numeric** Tailwind step, so at every width this port
    /// drives (≥ the `sm` breakpoint, the convention this whole tree rests
    /// on) the two resolve identically. The popup's actual width is
    /// `min(viewport − 2·VIEWPORT_PADDING, max_width)`, `dialog::Dialog::
    /// render`'s own arithmetic.
    pub max_width: Pixels,
    /// The height between the header and the footer. **Real, and zero**: the
    /// one live call site nests no content there at all — `DialogHeader` is
    /// followed directly by `DialogFooter`.
    pub body_height: Pixels,
    /// `DialogTitle` — always rendered on the one live call site
    /// (`Detach to manage {target.branch}`; `"main"` here stands in for the
    /// interpolated branch name).
    pub title: Option<SharedString>,
    /// `DialogDescription` — always rendered on the one live call site. The
    /// path and branch name are illustrative fills for
    /// `target.heldByPath`/`target.branch`; the surrounding prose is the
    /// real, literal string. See the module docs for why the two embedded
    /// `font-mono` spans are flattened into this one run.
    pub description: Option<SharedString>,
    /// The footer's own content height. `None` omits the footer (and its
    /// anchor) entirely — never the real call site's shape, but the same
    /// "port it and say so" primitive-level state `dialog`'s own `empty` arm
    /// takes.
    pub footer_content_height: Option<Pixels>,
}

impl DetachHolderModal {
    /// The live `detach-holder-modal.tsx`, with illustrative fills for the
    /// two interpolated fields (`target.branch`, `target.heldByPath`) — the
    /// surrounding text is the real, literal source string.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            max_width: px(448.0),
            body_height: px(0.0),
            title: Some(SharedString::new_static("Detach to manage main")),
            description: Some(SharedString::new_static(
                "The checkout at /Users/dev/crowbar-worktrees/main will move to a detached \
                 HEAD, releasing main so Crowbar can manage it in its own worktree. Your \
                 files are safe — only the working directory's current branch changes; \
                 uncommitted changes and commits are preserved.",
            )),
            footer_content_height: Some(FOOTER_CONTENT_HEIGHT_DEFAULT),
        }
    }

    /// An **estimate** of the popup's own height, exactly `dialog::Dialog::
    /// popup_height`'s own caveat: the description's contribution here is a
    /// single-line guess, used only to size the row-layout harness's window
    /// tall enough — this call site's description is three sentences and
    /// **does wrap** in practice, so this number is never compared against
    /// a rendered popup; the row-layout test reads the real wrapped height
    /// back instead. See that test's own doc comment.
    #[must_use]
    pub fn popup_height_estimate(&self, theme: &Theme) -> Pixels {
        let mut height = f32::from(BORDER_WIDTH) * 2.0 + f32::from(self.body_height);

        if self.title.is_some() || self.description.is_some() {
            let mut header = f32::from(HEADER_PADDING) * 2.0;
            let mut sections = 0;
            if self.title.is_some() {
                header += theme.ui_text_xl.value().0 * 16.0 * TITLE_LINE_HEIGHT;
                sections += 1;
            }
            if self.description.is_some() {
                header += theme.ui_text_base.value().0 * 16.0 * DESCRIPTION_LINE_HEIGHT;
                sections += 1;
            }
            if sections > 1 {
                header += f32::from(HEADER_GAP);
            }
            height += header;
        }
        if let Some(content) = self.footer_content_height {
            height +=
                f32::from(BORDER_WIDTH) + f32::from(FOOTER_PADDING_Y) * 2.0 + f32::from(content);
        }
        px(height)
    }

    /// Renders the popup through `gpui_component::Dialog` — the same vendor
    /// type `dialog::Dialog::render` wraps, with the identical
    /// neutralisation (see that method's docs for the full account of
    /// `.overlay(false)`/`.close_button(false)`/the zeroed outer box).
    #[must_use]
    pub fn render(
        &self,
        window: &mut Window,
        cx: &mut App,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");

        let mut inner = div()
            .flex()
            .flex_col()
            .size_full()
            .rounded(theme.radius_2xl.value())
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .text_color(theme.popover_foreground)
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            .text_size(theme.ui_text_lg.value())
            .line_height(relative(POPUP_LINE_HEIGHT));

        if self.title.is_some() || self.description.is_some() {
            inner = inner.child(self.header(theme, anchors));
        }
        inner = inner.child(self.body());
        if let Some(content_height) = self.footer_content_height {
            inner = inner.child(Self::footer(theme, anchors, content_height));
        }

        let popup = anchors.root(ID_POPUP.into(), inner);

        // `w-full max-w-*` by hand — see `dialog::Dialog::render`'s identical
        // comment. No border compensation, for the identical reason: the
        // outer box's border is a genuine `refine_style` overwrite to zero.
        let viewport_width = f32::from(window.viewport_size().width);
        let outer_width = px((viewport_width - f32::from(VIEWPORT_PADDING) * 2.0)
            .min(f32::from(self.max_width))
            .max(0.0));

        GpuiDialog::new(cx)
            .overlay(false)
            .close_button(false)
            .p_0()
            .bg(Color::TRANSPARENT)
            .border_0()
            .border_color(Color::TRANSPARENT)
            .rounded(px(0.0))
            // `min_h_24()` — the vendor's own unconditional 96px floor,
            // neutralised for the identical reason `dialog::Dialog::render`
            // and `alert_dialog::AlertDialog::render` both carry this line:
            // this call site's real body is 0px, well under the floor.
            .min_h(px(0.0))
            .w(outer_width)
            .children([popup])
            .into_any_element()
    }

    /// `DialogHeader`: `flex flex-col gap-2 p-6 pr-10` — every side but the
    /// right at `dialog`'s own 24px, the right side at this call site's own
    /// 40px.
    fn header(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut header = div()
            .flex()
            .flex_col()
            .gap(HEADER_GAP)
            .pt(HEADER_PADDING)
            .pl(HEADER_PADDING)
            .pb(HEADER_PADDING)
            .pr(HEADER_PADDING_RIGHT);

        if let Some(title) = &self.title {
            header = header.child(anchors.boxed_text(
                AnchorId::new(ID_TITLE).line_sized(),
                Self::title_box(theme),
                title.clone(),
            ));
        }
        if let Some(description) = &self.description {
            header = header.child(anchors.boxed_text(
                AnchorId::new(ID_DESCRIPTION),
                Self::description_box(theme),
                description.clone(),
            ));
        }
        anchors.boxed(ID_HEADER.into(), header)
    }

    /// `DialogTitle`: `font-heading font-semibold text-xl leading-none`.
    /// **Identical to `dialog::Dialog::title_box`** — `font-heading` is the
    /// same dead utility on this call site too (`--font-heading` is
    /// undefined at `:root` regardless of which dialog call site asks for
    /// it), so no family is set here either and the popup's own `font_sans`
    /// inherits down.
    fn title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xl.value())
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(FontWeight::SEMIBOLD)
    }

    /// `DialogDescription`: `text-muted-foreground text-sm leading-relaxed`.
    /// The line height is this call site's own delta — see
    /// [`DESCRIPTION_LINE_HEIGHT`].
    fn description_box(theme: &Theme) -> Div {
        div()
            .w_full()
            .text_size(theme.ui_text_base.value())
            .line_height(relative(DESCRIPTION_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The call site's own content, as the box it occupies. **Unanchored on
    /// purpose**, exactly `dialog::Dialog::body`'s reasoning — and on the one
    /// reachable call site this is always a zero-height box.
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }

    /// `DialogFooter` (the `variant="default"` arm, the only one the
    /// reachable call site takes): byte-identical class list to `dialog::
    /// Dialog::footer`'s — see that method's docs.
    fn footer(theme: &Theme, anchors: &dyn AnchorSink, content_height: Pixels) -> AnyElement {
        let footer = div()
            .flex()
            .flex_row()
            .items_center()
            .justify_end()
            .gap(FOOTER_GAP)
            .px(FOOTER_PADDING_X)
            .py(FOOTER_PADDING_Y)
            .border_t(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT))
            .rounded_b(FOOTER_RADIUS)
            .child(div().h(content_height));
        anchors.boxed(ID_FOOTER.into(), footer)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, DetachHolderModal, FOOTER_BG_TINT,
        FOOTER_CONTENT_HEIGHT_DEFAULT, FOOTER_GAP, FOOTER_PADDING_X, FOOTER_PADDING_Y,
        FOOTER_RADIUS, HEADER_GAP, HEADER_PADDING, HEADER_PADDING_RIGHT, ID_DESCRIPTION, ID_FOOTER,
        ID_HEADER, ID_POPUP, ID_TITLE, LINE_SIZED, VIEWPORT_PADDING,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(HEADER_PADDING, px(STEP * 6.0)); // p-6
        assert_eq!(HEADER_PADDING_RIGHT, px(STEP * 10.0)); // pr-10
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(FOOTER_PADDING_X, px(STEP * 6.0)); // px-6
        assert_eq!(FOOTER_PADDING_Y, px(STEP * 4.0)); // py-4
        assert_eq!(FOOTER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // p-4
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(FOOTER_CONTENT_HEIGHT_DEFAULT, px(STEP * 8.0)); // h-9 sm:h-8
    }

    /// `dialog`'s own constants, restated rather than re-derived — see the
    /// module docs for why this is a registry constraint, not a second
    /// measurement.
    #[test]
    fn the_unoverridden_constants_match_dialogs_own() {
        assert_eq!(HEADER_PADDING, crate::components::dialog::HEADER_PADDING);
        assert_eq!(HEADER_GAP, crate::components::dialog::HEADER_GAP);
        assert_eq!(
            FOOTER_PADDING_X,
            crate::components::dialog::FOOTER_PADDING_X
        );
        assert_eq!(
            FOOTER_PADDING_Y,
            crate::components::dialog::FOOTER_PADDING_Y
        );
        assert_eq!(FOOTER_GAP, crate::components::dialog::FOOTER_GAP);
        assert_eq!(
            VIEWPORT_PADDING,
            crate::components::dialog::VIEWPORT_PADDING
        );
        assert_eq!(BORDER_WIDTH, crate::components::dialog::BORDER_WIDTH);
    }

    /// The two real deltas are genuinely different from `dialog`'s own
    /// numbers — the point of this module existing at all.
    #[test]
    fn the_two_overrides_genuinely_differ_from_dialogs_defaults() {
        assert_ne!(
            HEADER_PADDING_RIGHT,
            crate::components::dialog::HEADER_PADDING
        );
        assert_eq!(HEADER_PADDING_RIGHT, px(40.0));

        // `dialog::DESCRIPTION_LINE_HEIGHT` is private to that module (not
        // `pub`), so this compares against its known literal value —
        // `1.25 / 0.875`, restated in `dialog.rs`'s own doc comment — rather
        // than importing it directly.
        assert_ne!(super::DESCRIPTION_LINE_HEIGHT, 1.25 / 0.875);
        assert!((super::DESCRIPTION_LINE_HEIGHT - 1.625).abs() < f32::EPSILON);
    }

    /// The fixture is the live `detach-holder-modal.tsx`, with illustrative
    /// fills for its two interpolated fields.
    #[test]
    fn the_fixture_is_the_live_detach_holder_modal() {
        let fixture = DetachHolderModal::fixture();

        assert_eq!(fixture.max_width, px(448.0));
        assert_eq!(fixture.body_height, px(0.0));
        assert_eq!(fixture.title.as_deref(), Some("Detach to manage main"));
        assert!(
            fixture
                .description
                .as_deref()
                .is_some_and(|d| d.contains("detached HEAD")
                    && d.contains("Crowbar can manage it in its own worktree")),
        );
        assert_eq!(
            fixture.footer_content_height,
            Some(FOOTER_CONTENT_HEIGHT_DEFAULT)
        );
    }

    /// **The title is the one line-sized anchor**, the identical arithmetic
    /// `dialog`'s and `alert-dialog`'s both hold.
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    /// `bg-muted/72`, the same tint `dialog`'s footer takes.
    #[test]
    fn the_footer_background_is_muted_at_seventy_two_percent() {
        assert!((FOOTER_BG_TINT - 72.0).abs() < f32::EPSILON);
        for theme in [Theme::LIGHT, Theme::DARK] {
            let expected = theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT);
            assert_eq!(
                theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT),
                expected
            );
        }
    }

    /// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]`, the same corner
    /// `dialog`'s footer rounds.
    #[test]
    fn the_footer_radius_is_radius_2xl_less_one_pixel() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_2xl.value(), px(18.0));
        }
        assert_eq!(FOOTER_RADIUS, px(17.0));
    }

    /// Every anchor id is distinct, namespaced under `detach-holder-modal-`
    /// and never collides with `dialog::ID_*`.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_HEADER, ID_TITLE, ID_DESCRIPTION, ID_FOOTER];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("detach-holder-modal-")));
        assert!(
            ids.iter()
                .all(|id| *id != crate::components::dialog::ID_POPUP)
        );
    }

    /// A bare body (no header, no footer) is two borders around it — the
    /// degenerate case `dialog`'s and `alert-dialog`'s equivalents both hold.
    #[test]
    fn a_bare_popup_is_two_borders_around_the_body() {
        let theme = Theme::DARK;
        let bare = DetachHolderModal {
            max_width: px(448.0),
            body_height: px(100.0),
            title: None,
            description: None,
            footer_content_height: None,
        };
        assert_eq!(bare.popup_height_estimate(&theme), px(102.0));
    }
}
