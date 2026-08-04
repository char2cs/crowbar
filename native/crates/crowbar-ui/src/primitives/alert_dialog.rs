//! `alert-dialog` — P3.28's first item, and a §6.2 wrap whose whole finding is
//! that it needs **no new arithmetic**.
//!
//! `web/src/components/ui/alert-dialog.tsx` → `gpui_component::dialog::{Dialog,
//! DialogHeader, DialogFooter, DialogTitle, DialogDescription}` — the **same**
//! vendor widget `dialog::Dialog` already wraps. This module models
//! `AlertDialogPopup` (re-exported as `AlertDialogContent`) plus whichever of
//! `AlertDialogHeader`/`AlertDialogTitle`/`AlertDialogDescription`/
//! `AlertDialogFooter` a call site nests inside it — the primitive-first choice
//! `dialog` and `popover` both make.
//!
//! # `alert-dialog` is a variant of `dialog`, not a distinct surface — read the
//! diff, not a guess
//!
//! Base UI ships `AlertDialog` as a separate React primitive
//! (`@base-ui/react/alert-dialog`, not `@base-ui/react/dialog`) for
//! **accessibility semantics** — `role="alertdialog"`, no light-dismiss on
//! backdrop click, no `Escape` — none of which `gpui_component::dialog::Dialog`
//! has a second implementation of; `gpui-component` is not a Base UI clone and
//! carries one "modal box" primitive, which is the one `dialog.rs` already
//! wraps. So the question this item was handed is not "wrap or build" — it is
//! "does this need its own primitive at all", and the answer comes from
//! diffing the two `.tsx` files' compiled class lists, not from the accessibility
//! semantics neither side's geometry contract can see.
//!
//! `diff <(sed classes) <(sed classes)` between `dialog.tsx`'s `DialogPopup` and
//! `alert-dialog.tsx`'s `AlertDialogPopup`, on the reachable configuration (see
//! §3), is one token:
//!
//! | | `dialog.tsx` | `alert-dialog.tsx` |
//! |---|---|---|
//! | popup | …`shadow-lg/5 `**`outline-none `**`transition-…` | …`shadow-lg/5 transition-…` (no `outline-none`) |
//! | header | `p-6 gap-2` | `p-6 gap-2` **`text-center sm:text-left`** (see §2) |
//! | footer (`variant="default"`, the only arm either reachable call site takes) | `border-t bg-muted/72 py-4` | `border-t bg-muted/72 py-4` — **byte-identical** |
//! | title | `font-heading font-semibold text-xl leading-none` | **byte-identical** |
//! | description | `text-muted-foreground text-sm` | **byte-identical** |
//!
//! `outline-none` sets the CSS `outline` property, which paints outside the
//! border box and has no `ANCHORS.md` field on either side (§6 material, same
//! as `dialog`'s `shadow-lg/5`) — geometrically inert. `text-center sm:text-left`
//! is addressed in §2. Every length constant, every radius, every colour token
//! and every line-height this module needs is therefore **`dialog`'s own**,
//! restated here under this surface's own anchor ids because the two are
//! measured as separate surfaces (their `data-slot`s are `alert-dialog-*`, not
//! `dialog-*`, and `ANCHORS.md` compares snapshots by id) — not because a single
//! number differs. Cross-reference: every constant below carries a doc comment
//! pointing at `dialog::Dialog`'s equivalent.
//!
//! # §2. `text-center sm:text-left` is real and geometrically inert here
//!
//! `AlertDialogHeader`'s class list opens `flex flex-col gap-2 p-6 text-center
//! max-sm:pb-4 sm:text-left` — `dialog`'s has no `text-center`/`sm:text-left`
//! pair at all. `text-align` moves where a run of text sits *inside* its own
//! box; it moves neither the box's own width nor its height, and every anchor
//! under this header is block-level with no authored width of its own (§4's
//! `CONTENT_SIZED` is empty, exactly as `dialog`'s is), so the header's, the
//! title's and the description's *boxes* are unaffected regardless of which
//! side the cascade resolves to. And on every cell this port drives — at or
//! above the `sm` breakpoint, the same convention `dialog`'s
//! `flex-row`/`justify-end` footer choice and `dropdown_menu`'s `sm:` trap
//! warning both rest on — `sm:text-left` is what wins, which is what a header
//! with no `text-align` class at all resolves to by default in a left-to-right
//! document. So the class is real, it is not modelled (`ANCHORS.md` carries no
//! `text-align` field on either side — ordinary text position within a box is
//! not part of the bounds/colour/radius/border contract §3 defines), and even
//! if it were, the reachable cell's own resolved value is the same one `dialog`
//! never had to name.
//!
//! # §3. Reachability: real code, blocked by an environmental defect this item
//! did not introduce
//!
//! `alert-dialog.tsx` has **one** importer anywhere in `web/src`:
//! `features/git/components/review-thread-item.tsx`, whose `<AlertDialogContent>`
//! (no `className`, so the primitive's own `max-w-lg`) nests exactly
//! `AlertDialogHeader` → `AlertDialogTitle` + `AlertDialogDescription`, then
//! `AlertDialogFooter` (`variant="default"`, the only arm reached — a `Cancel`
//! and a `Delete` button, both `size="sm"`) — **no body between them**, unlike
//! `dialog`'s `add-repository-modal` reference, which has a two-field form.
//! `pendingDelete !== null` is what opens it, set by a message row's own
//! "Delete" menu item, reachable on **every** thread regardless of authorship
//! (`canDelete = isRoot ? Boolean(onDeleteThread) : Boolean(onDeleteMessage)` —
//! no author check, unlike `canEdit`).
//!
//! This is real, driveable code — not dead like `toast`'s `AnchoredToasts` (see
//! `toast.rs`'s module docs). A live capture was attempted in full: the
//! dev-fixture project's `demo` repo and its locked review workspace
//! (`repos/…/workspaces/…`) both exist and answer real `GET`s over the daemon's
//! REST API (confirmed with `curl --unix-socket`), and a real review thread was
//! opened against it (`POST …/threads`, `201`, a genuine persisted row — not a
//! fabricated DOM element). But the running dev webview's `IndexedDB`-backed
//! entity cache (`web/src/lib/persistence/entity-cache.ts`, read by
//! `lib/store/workspace-list.ts`'s `buildTreeFromCache`) is failing every open
//! with `"An attempt was made to open a database using a lower version than the
//! existing version"` — logged continuously since long before this item started
//! and unrelated to anything this item touched. `IndexedDB` is shared **by
//! bundle id across every build, including production** (a standing hazard, not
//! a discovery of this item), so a newer schema written by some other session
//! sharing the same webview poisons this older bundle's own open. The practical
//! effect: `useSidebarStore.repos` reads back empty, so
//! `workspace-route-guard.ts`'s `shouldRedirectUnknownWorkspace` treats *every*
//! repo-scoped workspace route as unknown and bounces it back to the project's
//! own `/home` — confirmed by navigating there directly
//! (`location.hash = '#/ide/<project>/<repo>/<ws>'`) and watching the daemon log
//! immediately 404 `.../home/review/*` instead of ever reaching the repo-scoped
//! endpoint that answers over `curl`. Resetting the shared `IndexedDB` was not
//! attempted — the same store also backs the **production** app (`Dev Home
//! Isolation`'s own standing warning), and a fix for a shared-storage defect is
//! not this item's to make unilaterally.
//!
//! So: **no capture, and no fabrication** — the geometry-determining source is
//! identical to `dialog`'s to the byte on every field this contract carries,
//! which is evidence of a different, load-bearing kind: it means the numbers
//! `dialog.md`'s own live capture already converged on (§0 of that file) are
//! this surface's numbers too, without this item inventing a second live
//! reference to prove a fact the class diff above already proves. A future item
//! that can reach the fixture past the `IndexedDB` defect should regenerate
//! `/tmp/p3-ref-alert-dialog.json` through `extractSnapshotSource` on the live
//! `review-thread-item.tsx` delete confirmation and confirm it reproduces
//! `dialog`'s numbers exactly, on the fields the two share.
//!
//! # What the fixture is, and how it is not a guess either
//!
//! `AlertDialog::fixture` is the one reachable call site's own real shape —
//! **no live pixels**, but real source facts: `hasReplies = false`'s
//! (`isRoot`) title `"Delete this thread?"` and description `"This permanently
//! deletes the comment and its thread."`, both literal strings in
//! `review-thread-item.tsx`; `body_height = 0`, because that call site nests no
//! content between the header and the footer at all (§3) — not a placeholder,
//! the real shape; and `footer_content_height` derived from `button::Size::Sm`'s
//! own already-ported, already-tested height (`h-8 sm:h-7` → 28px at the `sm`
//! breakpoint every driven cell uses), not invented. `max_width` is
//! `alert-dialog.tsx`'s own unmodified `max-w-lg` — Tailwind's stock container
//! step, 512px, uncustomised by this app's `theme.css` — because the one live
//! call site passes no `className` to override it, unlike `dialog`'s
//! `sm:max-w-md`.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, Window, div, px, relative,
};
use gpui_component::dialog::Dialog as GpuiDialog;

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `AlertDialogPrimitive.Popup`. Every other bound on this
/// surface is reported relative to it.
pub const ID_POPUP: &str = "alert-dialog-popup";
/// `AlertDialogHeader`, rendered only where a call site nests one — the one
/// reachable call site always does (§3).
pub const ID_HEADER: &str = "alert-dialog-header";
/// `AlertDialogTitle`.
pub const ID_TITLE: &str = "alert-dialog-title";
/// `AlertDialogDescription`.
pub const ID_DESCRIPTION: &str = "alert-dialog-description";
/// `AlertDialogFooter`, rendered only where a call site nests one — the one
/// reachable call site always does.
pub const ID_FOOTER: &str = "alert-dialog-footer";

/// See `dialog::CONTENT_SIZED` — same reasoning, same empty answer: the popup's
/// width is a computed length, the header/footer are the popup's own width
/// less its border, and the title/description are block-level.
pub const CONTENT_SIZED: [&str; 0] = [];

/// See `dialog::LINE_SIZED` — **the title, and only the title**, for the
/// identical arithmetic: `leading-none` puts a 20px `text-xl` run in a 20px box
/// with no padding and no authored height. The description keeps its default
/// line height and is prose that can wrap, exactly as `dialog`'s is not.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `border` on the popup, and the vendor's own unconditional `border_1()` on
/// the outer box this crate neutralises. **Identical to `dialog::BORDER_WIDTH`**
/// — same class, same vendor, same neutralisation (see the module docs).
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `p-6` on the header. **Identical to `dialog::HEADER_PADDING`.**
pub const HEADER_PADDING: Pixels = px(24.0);

/// `gap-2` inside the header. **Identical to `dialog::HEADER_GAP`.**
pub const HEADER_GAP: Pixels = px(8.0);

/// `px-6` on the footer. **Identical to `dialog::FOOTER_PADDING_X`.**
pub const FOOTER_PADDING_X: Pixels = px(24.0);

/// `py-4` on the footer. **Identical to `dialog::FOOTER_PADDING_Y`.**
pub const FOOTER_PADDING_Y: Pixels = px(16.0);

/// `gap-2` between the footer's own buttons. **Identical to
/// `dialog::FOOTER_GAP`.**
pub const FOOTER_GAP: Pixels = px(8.0);

/// `AlertDialogViewport`'s `p-4`: 16px, between the window edge and the popup.
/// **Identical to `dialog::VIEWPORT_PADDING`** — same primitive
/// (`fixed inset-0 … p-4`), same class.
pub const VIEWPORT_PADDING: Pixels = px(16.0);

/// `leading-none` on the title. **Identical to `dialog`'s.**
const TITLE_LINE_HEIGHT: f32 = 1.0;

/// Inherited `text-base`'s line height on the popup. **Identical to
/// `dialog::POPUP_LINE_HEIGHT`** — the popup is portalled to `document.body` on
/// both files, and neither authors its own `text-*` on the popup itself.
const POPUP_LINE_HEIGHT: f32 = 1.5;

/// `text-sm`'s line height on the description. **Identical to
/// `dialog::DESCRIPTION_LINE_HEIGHT`.**
const DESCRIPTION_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `bg-muted/72` on the footer. **Identical to `dialog::FOOTER_BG_TINT`.**
const FOOTER_BG_TINT: f32 = 72.0;

/// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]` on the footer. **Identical to
/// `dialog::FOOTER_RADIUS`** — `radius_2xl` (18px) less one pixel.
const FOOTER_RADIUS: Pixels = px(17.0);

/// `AlertDialogPopup`, `AlertDialogHeader`, `AlertDialogTitle`,
/// `AlertDialogDescription` and `AlertDialogFooter`, together.
///
/// # The body is a height, and on the one live call site it is zero
///
/// Exactly `dialog`'s `body_height` in shape, but not in value: the one
/// reachable call site (`review-thread-item.tsx`'s delete confirmation) nests
/// **nothing** between its header and its footer, so `body_height` defaults to
/// `0` — a real fact about that call site, not a placeholder standing in for an
/// unmodelled form the way `dialog`'s 172px is.
#[derive(Clone, Debug, PartialEq)]
pub struct AlertDialog {
    /// `max-w-lg` (or whichever `max-w-*` a call site's `className` resolves
    /// tailwind-merge to — the one live call site passes none). The popup's
    /// actual width is `min(viewport − 2·VIEWPORT_PADDING, max_width)`, exactly
    /// `dialog::Dialog::max_width`'s arithmetic.
    pub max_width: Pixels,
    /// The height the call site's own content comes out at, between the header
    /// and the footer. Real content between P3.21's `add-repository-modal`; a
    /// real **zero** here, because `review-thread-item.tsx`'s
    /// `AlertDialogContent` nests no such content at all (see the module docs
    /// §3).
    pub body_height: Pixels,
    /// `AlertDialogTitle`, when a call site renders one — the one reachable
    /// call site always does, one of two literal strings depending on
    /// `deletingIsRoot`.
    pub title: Option<SharedString>,
    /// `AlertDialogDescription`, when a call site renders one.
    ///
    /// **Present on the fixture, unlike `dialog`'s own `None`.** The one
    /// reachable `alert-dialog` always nests a description (one of three
    /// literal strings depending on `deletingIsRoot`/`hasReplies`); `dialog`'s
    /// one reachable call site never does. Different real shapes, not a
    /// default carried over by accident.
    pub description: Option<SharedString>,
    /// The height of the footer's own content — its buttons — when a call site
    /// renders a footer. `None` omits the footer (and its anchor) entirely.
    pub footer_content_height: Option<Pixels>,
}

impl AlertDialog {
    /// `review-thread-item.tsx`'s delete-thread confirmation
    /// (`deletingIsRoot && !hasReplies`): `AlertDialogContent` with no
    /// `className`, a header carrying `"Delete this thread?"` and
    /// `"This permanently deletes the comment and its thread."`, no body, and a
    /// footer with a `size="sm"` Cancel and a `size="sm"` destructive Delete.
    ///
    /// **No live pixels — see the module docs §3 for why, and for what was
    /// actually tried.** Every number here is derived from source rather than
    /// measured: `max_width` from `alert-dialog.tsx`'s own unmodified
    /// `max-w-lg` (Tailwind's stock 512px container step — this app's
    /// `theme.css` does not redefine the container scale, only colour/radius/
    /// spacing), `body_height` from the real absence of a body (§3), and
    /// `footer_content_height` from `button::Size::Sm::extent` at the `sm`
    /// breakpoint every driven cell in this tree uses — `SPACING * 7.0` = 28px,
    /// not `dialog`'s 32px, because that call site's buttons are explicitly
    /// `size="sm"` where `add-repository-modal`'s are the default size.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            max_width: px(512.0),
            body_height: px(0.0),
            title: Some(SharedString::new_static("Delete this thread?")),
            description: Some(SharedString::new_static(
                "This permanently deletes the comment and its thread.",
            )),
            footer_content_height: Some(px(28.0)),
        }
    }

    /// The popup's own height: two borders, the header where there is one, the
    /// body, and the footer where there is one. **Identical arithmetic to
    /// `dialog::Dialog::popup_height`** — see that method's docs for why the
    /// description's contribution is a same-shaped estimate, used only to size
    /// the row-layout harness's window and never compared against a reference.
    #[must_use]
    pub fn popup_height(&self, theme: &Theme) -> Pixels {
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

    /// Renders the popup through `gpui_component::Dialog` — **the same vendor
    /// type `dialog::Dialog::render` wraps**, with the same neutralisation (see
    /// that method's docs for the full account of what `.overlay(false)`/
    /// `.close_button(false)`/the zeroed outer box are for; nothing here
    /// differs).
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
        // comment. No border compensation, for the identical reason: the outer
        // box's border is a genuine `refine_style` overwrite to zero.
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
            // applied *before* `refine_style` runs — see the module docs'
            // §"finding" this item made past `dialog`'s own account: neither
            // it nor `refine_style` clears `min_height` on its own, because
            // `refine_style` only overwrites a `StyleRefinement` field this
            // crate's own chain actually sets, and `dialog::Dialog::render`
            // never had to set one (its one reachable body is 172px, always
            // above the floor). This surface's real body is 0px, so the
            // floor is the first thing in this whole tree with a genuinely
            // empty popup to expose it — measured, not assumed:
            // `row_layout/alert_dialog.rs`'s own `empty` test read `96px`
            // against a `2px` claim before this line existed.
            .min_h(px(0.0))
            .w(outer_width)
            .children([popup])
            .into_any_element()
    }

    /// `AlertDialogHeader`: `flex flex-col gap-2 p-6 text-center max-sm:pb-4
    /// sm:text-left`. The `text-center`/`sm:text-left` pair is not modelled —
    /// see the module docs §2 for why it moves nothing this contract carries.
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

    /// `AlertDialogTitle`: `font-heading font-semibold text-xl leading-none`.
    /// **Identical to `dialog::Dialog::title_box`** — `font-heading` is the
    /// same dead utility on this file too (`AlertDialogTitle` is portalled to
    /// `document.body` the same way, and `--font-heading` is undefined at
    /// `:root` regardless of which dialog primitive asks for it), so no family
    /// is set here either and the popup's own `font_sans` inherits down.
    fn title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xl.value())
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(FontWeight::SEMIBOLD)
    }

    /// `AlertDialogDescription`: `text-muted-foreground text-sm`. **Identical
    /// to `dialog::Dialog::description_box`.**
    fn description_box(theme: &Theme) -> Div {
        div()
            .w_full()
            .text_size(theme.ui_text_base.value())
            .line_height(relative(DESCRIPTION_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The call site's own content, as the box it occupies. **Unanchored on
    /// purpose**, exactly `dialog::Dialog::body`'s reasoning — and on the one
    /// reachable call site this is always a zero-height box (module docs §3).
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }

    /// `AlertDialogFooter` (the `variant="default"` arm, the only one the
    /// reachable call site takes): `flex flex-col-reverse gap-2 px-6
    /// sm:flex-row sm:justify-end sm:rounded-b-[…] border-t bg-muted/72 py-4`.
    /// **Byte-identical class list to `dialog::Dialog::footer`'s** — see the
    /// module docs' table.
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
        AlertDialog, BORDER_WIDTH, CONTENT_SIZED, FOOTER_BG_TINT, FOOTER_GAP, FOOTER_PADDING_X,
        FOOTER_PADDING_Y, FOOTER_RADIUS, HEADER_GAP, HEADER_PADDING, ID_DESCRIPTION, ID_FOOTER,
        ID_HEADER, ID_POPUP, ID_TITLE, LINE_SIZED, VIEWPORT_PADDING,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to — and every one of them equal to
    /// `dialog`'s, because the class lists are byte-identical on these fields.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple_and_matches_dialog() {
        const STEP: f32 = 4.0;

        assert_eq!(HEADER_PADDING, px(STEP * 6.0)); // p-6
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(FOOTER_PADDING_X, px(STEP * 6.0)); // px-6
        assert_eq!(FOOTER_PADDING_Y, px(STEP * 4.0)); // py-4
        assert_eq!(FOOTER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // p-4
        assert_eq!(BORDER_WIDTH, px(1.0));

        assert_eq!(HEADER_PADDING, crate::primitives::dialog::HEADER_PADDING);
        assert_eq!(HEADER_GAP, crate::primitives::dialog::HEADER_GAP);
        assert_eq!(
            FOOTER_PADDING_X,
            crate::primitives::dialog::FOOTER_PADDING_X
        );
        assert_eq!(
            FOOTER_PADDING_Y,
            crate::primitives::dialog::FOOTER_PADDING_Y
        );
        assert_eq!(FOOTER_GAP, crate::primitives::dialog::FOOTER_GAP);
        assert_eq!(
            VIEWPORT_PADDING,
            crate::primitives::dialog::VIEWPORT_PADDING
        );
        assert_eq!(BORDER_WIDTH, crate::primitives::dialog::BORDER_WIDTH);
    }

    /// The fixture is the one reachable call site's real source shape: a real
    /// title, a real description (unlike `dialog`'s own `None`), a real zero
    /// body, and a `size="sm"`-derived footer height distinct from `dialog`'s
    /// default-sized one.
    #[test]
    fn the_fixture_is_the_one_reachable_review_thread_delete_confirmation() {
        let fixture = AlertDialog::fixture();

        assert_eq!(fixture.max_width, px(512.0));
        assert_eq!(fixture.body_height, px(0.0));
        assert_eq!(fixture.title.as_deref(), Some("Delete this thread?"));
        assert_eq!(
            fixture.description.as_deref(),
            Some("This permanently deletes the comment and its thread."),
        );
        assert_eq!(fixture.footer_content_height, Some(px(28.0)));

        for theme in [Theme::LIGHT, Theme::DARK] {
            let height = fixture.popup_height(&theme);
            // 2 borders + header(48 pad + 20 title + 8 gap + 20 description) + 0
            // body + footer(1 border + 32 pad + 28 content).
            let expected = 2.0 + (48.0 + 20.0 + 8.0 + 20.0) + 0.0 + (1.0 + 32.0 + 28.0);
            assert!(
                (f32::from(height) - expected).abs() < f32::EPSILON,
                "{height:?}"
            );
        }
    }

    /// **The title is the one line-sized anchor**, identically to `dialog`'s.
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    /// `bg-muted/72`, the same tint `dialog`'s footer takes (72.0 on both —
    /// `dialog::FOOTER_BG_TINT` is a private constant there, so this asserts
    /// the value directly rather than importing it).
    #[test]
    fn the_footer_background_is_muted_at_seventy_two_percent_like_dialogs() {
        assert!((FOOTER_BG_TINT - 72.0).abs() < f32::EPSILON);
        for theme in [Theme::LIGHT, Theme::DARK] {
            let expected = theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT);
            assert_eq!(
                theme.muted.mix(FOOTER_BG_TINT, Color::TRANSPARENT),
                expected
            );
        }
    }

    /// `sm:rounded-b-[calc(var(--radius-2xl)-1px)]`, the same corner `dialog`'s
    /// footer rounds.
    #[test]
    fn the_footer_radius_is_radius_2xl_less_one_pixel() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_2xl.value(), px(18.0));
        }
        assert_eq!(FOOTER_RADIUS, px(17.0));
    }

    /// Every anchor id is distinct, namespaced under `alert-dialog-` and
    /// therefore never collides with `dialog::ID_*` even though the two share
    /// every constant.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_HEADER, ID_TITLE, ID_DESCRIPTION, ID_FOOTER];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("alert-dialog-")));
        assert!(
            ids.iter()
                .all(|id| *id != crate::primitives::dialog::ID_POPUP)
        );
    }

    /// A taller footer or a non-zero body both move the popup's own height by
    /// exactly their own delta — the same arithmetic property `dialog`'s
    /// equivalent test holds.
    #[test]
    fn the_popup_height_follows_the_body_and_the_footer() {
        let theme = Theme::DARK;
        let mut fixture = AlertDialog::fixture();
        let base = fixture.popup_height(&theme);

        fixture.body_height = px(f32::from(fixture.body_height) + 50.0);
        assert_eq!(fixture.popup_height(&theme), base + px(50.0));

        fixture.footer_content_height = Some(px(80.0));
        let taller_footer = base + px(50.0) + px(80.0 - 28.0);
        assert_eq!(fixture.popup_height(&theme), taller_footer);

        fixture.footer_content_height = None;
        // Losing the footer loses its border and both paddings besides its
        // (now 80px) content: 1 + 16 + 16 + 80 = 113.
        assert_eq!(fixture.popup_height(&theme), taller_footer - px(113.0));
    }

    /// No header, no footer, no body: the popup is nothing but its own two
    /// borders — the degenerate case `dialog`'s equivalent test also holds.
    #[test]
    fn a_bare_popup_is_two_borders_around_nothing() {
        let theme = Theme::DARK;
        let bare = AlertDialog {
            max_width: px(512.0),
            body_height: px(0.0),
            title: None,
            description: None,
            footer_content_height: None,
        };
        assert_eq!(bare.popup_height(&theme), px(2.0));
    }
}
