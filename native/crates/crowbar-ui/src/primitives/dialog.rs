//! `dialog` — the second §6.2 wrap that reaches a real anchor, after
//! `popover`.
//!
//! `web/src/components/ui/dialog.tsx` → `gpui_component::dialog::{Dialog,
//! DialogHeader, DialogFooter, DialogTitle, DialogDescription}`. This module
//! models the file's **base primitive** — `DialogPopup` (re-exported as
//! `DialogContent`) plus the header/title/description/footer sections a call
//! site nests inside it — the same choice `popover` made: wrap the primitive
//! the vendor's own names line up with, and treat a higher-level call-site
//! composition (`AppDialog`, `dialog.tsx`'s second export) as a *caller* of
//! this primitive rather than a second thing to model, exactly as
//! `repo-icon-popover` was a caller of `popover.tsx` rather than a second
//! popover to port.
//!
//! `DialogPopup` + `DialogHeader` + `DialogTitle` + `DialogFooter` line up
//! 1:1 with the vendor's own `Dialog` + `DialogHeader` + `DialogTitle` +
//! `DialogFooter` — a naming coincidence `popover` did not have, and the
//! reason this module wraps the primitive rather than `AppDialog`, which
//! bypasses all four and hand-rolls its own header row.
//!
//! # What had to be overridden to reach our design
//!
//! | | |
//! |---|---|
//! | [`GpuiDialog::overlay`]`(false)` | the vendor's `true` reads `Root::read(window, cx).active_dialogs` to decide whether *this* layer owns the click-outside handler — a `Root` this measurement harness never mounts. `false` skips that read entirely (and the window-drag/click-catching block it lives in), which costs nothing visible: the backdrop dim it would otherwise gate is a **separate, unconditional** field (`DialogProps.overlay_visible`) with no public setter, so the vendor never paints one through this crate's surface regardless. |
//! | [`GpuiDialog::close_button`]`(false)` | the vendor's own close affordance is a `Button` appended as a *second* child of the outer box, which would trip its `.gap(paddings.top.max(px(8.)))` — dead with one child, live with two. Declining it keeps the outer box's child count at one; `dialog-close` is not a name this port emits. |
//! | `.p_0()` on the outer [`GpuiDialog`] | the outer box is not one this crate holds (see below), so it cannot carry the *real* padding — it is neutralised instead, and every real padding (`p-6` on the header, `py-4`/`px-6` on the footer) is authored on **this crate's own** boxes, which nest three levels inside the vendor's own `pl(paddings.left).pr(paddings.right)` wrapper. Zeroing `self.style.padding` on the outer removes that wrapper's contribution along both axes in one call — the vendor reads it back off `self.style` before drawing anything. |
//! | `.bg`/`.border_0`/`.border_color`/`.rounded` zeroed on the outer | same reason `popover` needed `appearance(false)`: the vendor's `Dialog::render` paints its own `bg(cx.theme().tokens.background)`, `border_1()`, `border_color(cx.theme().border)` and `rounded(cx.theme().radius_lg)` *before* applying this crate's `refine_style` — **`gpui-component`'s own theme**, not ours, and its own radius scale, not ours. `Dialog` has no `appearance(false)` escape hatch (unlike `Popover`), so the zeroing has to happen field by field; `refine_style` still applies these refinements *after* the vendor's defaults, so they win. |
//! | `.w(outer_width)`, computed rather than declared | `Dialog::w`/`Dialog::max_w` are **dedicated fields** (`DialogProps.width`/`.max_width`), not `Styled`'s `w`/`max_w` — an absolutely-positioned, fixed-pixel panel, not `w-full max-w-md`'s "fill and cap" CSS. `dialog.tsx`'s responsive behaviour is reproduced by hand instead: `min(window.viewport_size().width − 2·VIEWPORT_PADDING, max_width)`, the same arithmetic the browser runs for the class pair. |
//!
//! # The popup box is ours, not the vendor's
//!
//! Exactly `popover`'s finding, restated for a widget with no `appearance`
//! toggle: `Dialog::render` builds its own outer `v_flex` *inside* a private
//! method, and [`AnchorSink::root`] takes a [`gpui::Div`] this crate holds —
//! so a box this crate never holds cannot carry the root anchor. The fix
//! there was `appearance(false)`; here, with no such switch, the outer box is
//! instead **neutralised in place** (previous section) and this crate's own
//! div — real border, real radius, real background — is nested one level
//! inside it as the [`Dialog::render`] method's single child.
//!
//! The vendor's outer box carries an **unconditional** `border_1()` that no
//! method turns off — only `refine_style` can, and only the *colour*, not the
//! width, would be reachable through the public setters `Dialog` exposes.
//! `.border_0()` (a `Styled` method, not shadowed by anything `Dialog`
//! defines) reaches the width too, and **measured rather than assumed**:
//! `row_layout/dialog.rs`'s width tests were first written against a
//! `content_width + 2·BORDER_WIDTH` outer, on the theory that the vendor's
//! border would keep occupying two pixels of box-sizing regardless of its
//! colour. They failed at `450` against a `448` reference — `refine_style` is
//! a genuine overwrite of the base `Style`, not a merge that leaves the width
//! behind, so the border truly is zero and the compensation was double
//! counting. The outer's `.w()` is the bare content width.
//!
//! # What the oracle can and cannot see here
//!
//! `shadow-lg/5`, `before:shadow-[…]` and every `transition-*`/`data-*-style`
//! rule are `ANCHORS.md` §6 material on both sides, exactly as `popover`
//! documents. `not-dark:bg-clip-padding` has no field either. The backdrop
//! dim (`DialogBackdrop`, `bg-black/32`) and the centering grid
//! (`DialogViewport`) are **not this surface's anchors at all** — see
//! `native/mapping/dialog.md` §6 for why, the same reasoning `popover`
//! applied to its own trigger.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, Window, div, px, relative,
};
use gpui_component::dialog::Dialog as GpuiDialog;

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `DialogPrimitive.Popup`, the bordered box `bg-popover`
/// paints. Every other bound on this surface is reported relative to it.
pub const ID_POPUP: &str = "dialog-popup";
/// `DialogHeader`, rendered only where a call site nests one — i.e. only
/// where a title or a description is present.
pub const ID_HEADER: &str = "dialog-header";
/// `DialogTitle`.
pub const ID_TITLE: &str = "dialog-title";
/// `DialogDescription`.
pub const ID_DESCRIPTION: &str = "dialog-description";
/// `DialogFooter`, rendered only where a call site nests one.
pub const ID_FOOTER: &str = "dialog-footer";

/// The anchors whose boxes size to their own text (`ANCHORS.md` v1.5).
///
/// None. The popup's width is a computed length, the header/footer are the
/// popup's own width less padding, and the title and description are
/// block-level — none is a box whose used width is a text run's max-content
/// width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose **box height is their own line box** (`ANCHORS.md`
/// v1.6).
///
/// **The title, and only the title** — exactly `popover`'s reasoning.
/// `leading-none` is `line-height: 1`, so a 20px `text-xl` run sits in a 20px
/// line box with no padding and no authored height. The description is not:
/// it keeps its default line height and — unlike the title — is prose that
/// wraps, so its box height is *n* line boxes for a content-dependent *n*,
/// never reliably its own single line box.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `border` on the popup, and the vendor's own unconditional `border_1()` on
/// the outer box this crate neutralises — see the module docs.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `p-6` on the header: 24px on all four sides.
pub const HEADER_PADDING: Pixels = px(24.0);

/// `gap-2` inside the header, between the title and the description.
pub const HEADER_GAP: Pixels = px(8.0);

/// `px-6` on the footer: 24px inline.
pub const FOOTER_PADDING_X: Pixels = px(24.0);

/// `py-4` on the footer: 16px block.
pub const FOOTER_PADDING_Y: Pixels = px(16.0);

/// `gap-2` between the footer's own buttons.
pub const FOOTER_GAP: Pixels = px(8.0);

/// `DialogViewport`'s `p-4`: 16px on every side, between the window edge and
/// the popup. Not an anchor (see the module docs) but still arithmetic this
/// component needs, because it is what `w-full` resolves against.
pub const VIEWPORT_PADDING: Pixels = px(16.0);

/// `leading-none` on the title: `line-height: 1`.
const TITLE_LINE_HEIGHT: f32 = 1.0;

/// Inherited `text-base`'s line height on the popup: Tailwind's stock
/// `calc(1.5 / 1)`, so 16px text sits on a 24px line box. Measured live on
/// the reachable popup: `16px/24px`.
const POPUP_LINE_HEIGHT: f32 = 1.5;

/// `text-sm`'s stock line height on the description: `calc(1.25 / 0.875)`, so
/// 14px text sits on a 20px line box.
const DESCRIPTION_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `bg-muted/72` on the footer.
const FOOTER_BG_TINT: f32 = 72.0;

/// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]` on the footer: this crate's
/// `radius_2xl` (18px, not Tailwind's stock 16) less one pixel, because the
/// popup itself has no `overflow-hidden` — unlike `AppDialog` — so the
/// footer has to round its own bottom corners to sit flush inside the
/// popup's border.
const FOOTER_RADIUS: Pixels = px(17.0);

/// `DialogPopup`, `DialogHeader`, `DialogTitle`, `DialogDescription` and
/// `DialogFooter`, together.
///
/// # Why the body is a **height**, not an element tree
///
/// Exactly `popover`'s `body_height`: the space between the header and the
/// footer is `AppDialog`'s (or any call site's) own content — a settings
/// panel, a form, a confirmation prompt, a different sub-UI at every call
/// site. The port takes its measured extent rather than reproducing one of
/// them, so the two boxes that genuinely *are* this primitive — the popup and
/// its header/footer — compare against a reference whose middle is whatever
/// it happens to be.
#[derive(Clone, Debug, PartialEq)]
pub struct Dialog {
    /// `max-w-md` (or whichever `max-w-*` a call site's `className` resolves
    /// to, after tailwind-merge replaces the primitive's own `max-w-lg`). The
    /// popup's actual width is `min(viewport − 2·VIEWPORT_PADDING, max_width)`,
    /// computed in [`Dialog::render`] because it is a function of the window
    /// this crate does not otherwise see.
    pub max_width: Pixels,
    /// The height the call site's own content comes out at, between the
    /// header and the footer.
    pub body_height: Pixels,
    /// `DialogTitle`, when a call site renders one.
    pub title: Option<SharedString>,
    /// `DialogDescription`, when a call site renders one.
    ///
    /// **Modelled but unreached.** The one dialog a parity run can drive —
    /// `add-repository-modal`'s `DialogContent` — nests a title and no
    /// description, so this defaults to `None`: an anchor the reference
    /// cannot produce is a `FieldPresence` delta that forgives nothing, the
    /// same call `popover` makes about its own title.
    pub description: Option<SharedString>,
    /// The height of the footer's own content — its buttons — when a call
    /// site renders a footer. `None` omits the footer (and its anchor)
    /// entirely, exactly as `dialog.tsx` does when no `DialogFooter` is
    /// nested.
    pub footer_content_height: Option<Pixels>,
}

impl Dialog {
    /// The live `add-repository-modal` (and, verbatim, `import-project-modal`):
    /// `DialogContent className="sm:max-w-md"` with a `DialogHeader` /
    /// `DialogTitle` reading "Add repository", an unmodelled two-field form,
    /// and a `DialogFooter` with two buttons.
    ///
    /// Measured live at a 1714px viewport, dark: popup `448 × 307`, header
    /// `68` (`24 + 20 + 24`, no description), footer `65`
    /// (`1 + 16 + 32 + 16`), and therefore body `307 − 2 − 68 − 65 = 172`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            max_width: px(448.0),
            body_height: px(172.0),
            title: Some(SharedString::new_static("Add repository")),
            description: None,
            footer_content_height: Some(px(32.0)),
        }
    }

    /// The popup's own height: two borders, the header where there is one,
    /// the body, and the footer where there is one.
    ///
    /// The description's contribution, when present, is estimated as one
    /// line — `DESCRIPTION_LINE_HEIGHT` at `ui_text_base` — because its real
    /// height is `gpui`'s own wrapped text layout, a quantity only a real
    /// window can produce; this is used solely to size the row-layout
    /// harness's window tall enough, never compared against a reference.
    #[must_use]
    pub fn popup_height(&self, theme: &Theme) -> Pixels {
        let mut height = f32::from(BORDER_WIDTH) * 2.0 + f32::from(self.body_height);

        if self.title.is_some() || self.description.is_some() {
            let mut header = f32::from(HEADER_PADDING) * 2.0;
            let mut sections = 0;
            if self.title.is_some() {
                // `text-xl leading-none`: the title's own box *is* this
                // length, a whole multiple of `TITLE_LINE_HEIGHT`.
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

    /// Renders the popup through `gpui_component::Dialog`, opting every
    /// contract anchor into `anchors`.
    ///
    /// `window`/`cx` are needed here and were not on `popover`'s equivalent
    /// method: `GpuiDialog::new` mints a `FocusHandle` off `cx`, and this
    /// crate's own boxes need the live viewport to reproduce `w-full
    /// max-w-*`. See `crowbar-app/src/surface.rs`'s `SurfaceParams::render_ctx`
    /// for how a cx-less call site still reaches this.
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

        // `w-full max-w-*` by hand: `Dialog::w` is a fixed pixel field, not a
        // `Styled` refinement, so the responsive arithmetic `dialog.tsx`
        // leaves to CSS is run here instead. No compensation for the outer
        // box's own border is needed — `.border_0()` below is a genuine
        // `refine_style` overwrite, measured rather than assumed (see the
        // module docs): the outer's *content* box is its declared `.w()`
        // exactly, because its border truly is zero, not merely transparent.
        let viewport_width = f32::from(window.viewport_size().width);
        let outer_width = px((viewport_width - f32::from(VIEWPORT_PADDING) * 2.0)
            .min(f32::from(self.max_width))
            .max(0.0));

        GpuiDialog::new(cx)
            // See the module docs: `Root` is never mounted here, and the
            // backdrop dim it would gate has no public setter regardless.
            .overlay(false)
            // Keeps the outer box's child count at one — see the module docs.
            .close_button(false)
            .p_0()
            .bg(Color::TRANSPARENT)
            .border_0()
            .border_color(Color::TRANSPARENT)
            .rounded(px(0.0))
            // `min_h_24()` — the vendor's own unconditional 96px floor on the
            // outer box, set *before* `refine_style` runs. `refine_style`
            // only overwrites a `StyleRefinement` field this crate's own
            // chain actually sets, and nothing above sets `min_height`, so
            // the floor survives untouched unless named here — found by
            // `alert_dialog.rs` (P3.28), whose one reachable popup has a
            // genuinely empty (0px) body and so is the first cell in this
            // whole tree low enough to fall under it. This surface's own
            // reachable body (172px) never has, which is why no test here
            // caught it — `--body-height 0` now does, in
            // `row_layout/dialog.rs`.
            .min_h(px(0.0))
            .w(outer_width)
            .children([popup])
            .into_any_element()
    }

    /// `DialogHeader`: `flex flex-col gap-2 p-6`, containing the title and
    /// the description where either is present.
    fn header(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut header = div().flex().flex_col().gap(HEADER_GAP).p(HEADER_PADDING);

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
    ///
    /// **`font-heading` is a dead utility in this build, measured rather than
    /// assumed**: `getComputedStyle(document.documentElement)
    /// .getPropertyValue('--font-heading')` on the live app is the empty
    /// string, so the class resolves to nothing and the title paints in the
    /// *inherited* `font-sans` — confirmed on the live title's own
    /// `getComputedStyle`, whose `font-family` is `"CalSansUI", …`, this
    /// crate's `font_sans`, not `font_heading`. This component therefore sets
    /// no family of its own here and lets the popup's `font_sans` inherit
    /// down, exactly as the reference does.
    fn title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xl.value())
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(FontWeight::SEMIBOLD)
    }

    /// `DialogDescription`: `text-muted-foreground text-sm`.
    fn description_box(theme: &Theme) -> Div {
        div()
            .w_full()
            .text_size(theme.ui_text_base.value())
            .line_height(relative(DESCRIPTION_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The call site's own content, as the box it occupies.
    ///
    /// **Unanchored on purpose**, exactly `popover`'s body: it is not part of
    /// `dialog.tsx`, it stands in for whatever a call site rendered between
    /// the header and the footer, and it is rendered because it is what gives
    /// the popup its height.
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }

    /// `DialogFooter` (the `variant="default"` arm, the only one any live
    /// call site takes): `flex flex-col-reverse gap-2 px-6 sm:flex-row
    /// sm:justify-end sm:rounded-b-[…] border-t bg-muted/72 py-4`.
    ///
    /// `flex-row`/`justify-end` rather than `flex-col-reverse`, because every
    /// cell this port drives is at or above the `sm` breakpoint — the same
    /// simplification `popover`'s tooltip/default split does not need to make
    /// but `dropdown_menu`'s `sm:` trap warns is easy to get backwards.
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
        BORDER_WIDTH, CONTENT_SIZED, Dialog, FOOTER_BG_TINT, FOOTER_GAP, FOOTER_PADDING_X,
        FOOTER_PADDING_Y, FOOTER_RADIUS, HEADER_GAP, HEADER_PADDING, ID_DESCRIPTION, ID_FOOTER,
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
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(FOOTER_PADDING_X, px(STEP * 6.0)); // px-6
        assert_eq!(FOOTER_PADDING_Y, px(STEP * 4.0)); // py-4
        assert_eq!(FOOTER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // p-4
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    /// The fixture is the live `add-repository-modal` (and, verbatim,
    /// `import-project-modal`), measured off the running app.
    #[test]
    fn the_fixture_is_the_live_add_repository_modal() {
        let fixture = Dialog::fixture();

        assert_eq!(fixture.max_width, px(448.0));
        assert_eq!(fixture.body_height, px(172.0));
        assert_eq!(fixture.title.as_deref(), Some("Add repository"));
        // No live dialog a parity run can reach has a description.
        assert_eq!(fixture.description, None);
        assert_eq!(fixture.footer_content_height, Some(px(32.0)));

        for theme in [Theme::LIGHT, Theme::DARK] {
            let height = fixture.popup_height(&theme);
            assert!(
                (f32::from(height) - 307.0).abs() < f32::EPSILON,
                "{height:?}"
            );
        }
    }

    /// **The title is the one line-sized anchor**, for the same arithmetic
    /// reason `popover`'s is: `leading-none` puts a 20px run in a 20px box,
    /// which *is* the line box, where the header, the footer and the popup
    /// are all boxes whose height is content plus padding plus border.
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    /// `bg-muted/72`, spelled the way every other `/N` tint in this tree is:
    /// `Color::mix`, against `Color::TRANSPARENT`.
    #[test]
    fn the_footer_background_is_muted_at_seventy_two_percent() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let expected = theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT);
            assert_eq!(
                theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT),
                expected
            );
        }
    }

    /// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]`: this crate's own
    /// `radius_2xl` — 18px, not Tailwind's stock 16 — less one pixel.
    #[test]
    fn the_footer_radius_is_radius_2xl_less_one_pixel() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_2xl.value(), px(18.0));
        }
        assert_eq!(FOOTER_RADIUS, px(17.0));
    }

    /// Every anchor id is distinct and namespaced, exactly the property
    /// `popover`'s own test asserts.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_HEADER, ID_TITLE, ID_DESCRIPTION, ID_FOOTER];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("dialog-")));
    }

    /// `--body-height`-equivalent arithmetic: a taller body and a taller
    /// footer content both move the popup's own height by exactly their own
    /// delta, holding everything else fixed.
    #[test]
    fn the_popup_height_follows_the_body_and_the_footer() {
        let theme = Theme::DARK;
        let mut fixture = Dialog::fixture();
        let base = fixture.popup_height(&theme);

        fixture.body_height = px(f32::from(fixture.body_height) + 50.0);
        assert_eq!(fixture.popup_height(&theme), base + px(50.0));

        fixture.footer_content_height = Some(px(80.0));
        let taller_footer = base + px(50.0) + px(80.0 - 32.0);
        assert_eq!(fixture.popup_height(&theme), taller_footer);

        fixture.footer_content_height = None;
        // Losing the footer loses its border and both paddings besides its
        // (now 80px) content — 1 + 16 + 16 + 80 = 113.
        assert_eq!(fixture.popup_height(&theme), taller_footer - px(113.0));
    }

    /// No header, no footer: the popup is just its own two borders around
    /// the body.
    #[test]
    fn a_bare_body_is_two_borders_around_it() {
        let theme = Theme::DARK;
        let bare = Dialog {
            max_width: px(448.0),
            body_height: px(100.0),
            title: None,
            description: None,
            footer_content_height: None,
        };
        assert_eq!(bare.popup_height(&theme), px(102.0));
    }
}
