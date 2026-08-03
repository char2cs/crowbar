//! `command` — the fourth §6.2 wrap that reaches a real anchor, and a
//! composition over [`super::autocomplete`] rather than a second set of
//! boxes under new ids.
//!
//! The native half of `web/src/components/ui/command.tsx`, which is two
//! things: `CommandDialog*` — `@base-ui/react/dialog` primitives, styled
//! directly — and `Command`/`CommandInput`/`CommandList`/`CommandItem`/
//! `CommandEmpty`, each of which **renders** `autocomplete.tsx`'s own
//! `Autocomplete`/`AutocompleteInput`/`AutocompleteList`/`AutocompleteItem`/
//! `AutocompleteEmpty`, restyled through `className`, not reimplemented.
//!
//! # `command` does not compose the already-ported `dialog.rs` — it is a
//! second hand-rolled shell, the same shape `AppDialog` is
//!
//! `native/mapping/dialog.md` models `DialogPopup` (`crowbar_ui::dialog::Dialog`)
//! plus whichever of `DialogHeader`/`DialogTitle`/`DialogDescription`/
//! `DialogFooter` a call site nests, and explicitly excludes `AppDialog` —
//! `settings-dialog`, `unsaved-changes-dialog`, etc. — because that file
//! bypasses all four and hand-rolls its own chrome from raw
//! `DialogPortal`/`DialogBackdrop`/`DialogViewport`/`DialogPrimitive.Popup`.
//!
//! `command.tsx`'s `CommandDialogPopup` does exactly what `AppDialog` does:
//! it imports `@base-ui/react/dialog` **directly** (`CommandDialogPrimitive`,
//! not `dialog.tsx`'s own `Dialog`/`DialogPopup` exports) and composes
//! `CommandDialogPortal`+`CommandDialogBackdrop`+`CommandDialogViewport`+
//! `CommandDialogPrimitive.Popup` itself, with `CommandInput`/`CommandPanel`/
//! `CommandFooter` in place of `DialogHeader`/`DialogTitle`/`DialogFooter`.
//! **So `command` is distinct from `dialog.rs`, not a caller of it** — same
//! underlying base-ui `Dialog` primitive, different composition, exactly
//! `AppDialog`'s relationship to it one file over.
//!
//! What *is* shared with `dialog.rs` is the **wrap technique**, because it is
//! the same vendor widget underneath: [`GpuiDialog`] is neutralised in place
//! (`.overlay(false).close_button(false)`, every default field zeroed) and
//! this crate's own div carries the real chrome one level in — see that
//! module's docs for the field-by-field reasoning, all of which applies here
//! unchanged. The one difference: `command.tsx`'s `max-w-xl` (576px) where
//! `dialog.tsx`'s call sites cap at `max-w-md` (448px) — a call-site number,
//! not a primitive one, exactly as `dialog::Dialog::max_width` is.
//!
//! `Dialog` also needed `.min_h(px(0.0))` to beat `gpui_component::Dialog`'s
//! own unconditional `min_h_24()` (96px) floor once `alert_dialog.rs` (P3.28)
//! found a body low enough to hit it. This surface's own reachable body
//! (140px) never has either, and the same override is applied here anyway,
//! for the reason it is applied on every wrap built on `GpuiDialog` from here
//! on: the floor is the vendor's, not a property of any one call site's
//! content, and a surface with a shorter body than this one's is one
//! `--body-height 0` away from needing it silently.
//!
//! # Why the shared boxes carry `autocomplete-*` ids, not `command-*`
//!
//! See `autocomplete.rs`'s own module docs for the live confirmation:
//! `command.tsx` never overrides `data-slot` on `CommandInput`'s inner
//! `AutocompleteInput`, so the live `<input>` still reports
//! `data-slot="autocomplete-input"`. `CommandList`/`CommandItem`/
//! `CommandEmpty` *do* override it (their own `data-slot` prop wins the
//! `{...props}` spread), which is real and reproduced in the React file's
//! `data-oracle-id`s below — but the **Rust structs** stay `autocomplete`'s:
//! there is one set of boxes, `autocomplete.rs` builds all of them, and this
//! module reuses [`super::autocomplete::Input`] and [`super::autocomplete::List`]
//! directly rather than re-deriving the same geometry under a second name.
//!
//! # What is new here
//!
//! Three boxes `autocomplete.tsx` has no equivalent of: [`ID_POPUP`] (the
//! dialog shell), [`ID_PANEL`] (a bordered box with **no `data-slot` in the
//! source at all** — this port adds `data-oracle-id="command-panel"` to it,
//! since a real, painted box with its own border/radius/bg needs an anchor
//! whether or not the React file happened to name it), and [`ID_FOOTER`].
//!
//! # What was captured, and how
//!
//! `dialog.md` §6's wall again: the dev server serves the shared worktree, so
//! this branch's own `data-oracle-id`s are not live there. Captured instead
//! via `getComputedStyle`/`getBoundingClientRect` on the live, unmodified
//! app — Context Pill → click → the workspace switcher — through
//! `[data-slot="command-*"]`/`[data-slot="autocomplete-*"]`, pinned at rest
//! (`command-dialog-popup.style.transition = 'none'`; confirmed
//! `transform: none`, `opacity: 1`, no `data-starting-style` first). The live
//! DOM: `command-dialog-popup 576×142`, `command` (the wrapper) `574×140`,
//! the input row `574×48` containing `autocomplete-input-group 554×36`,
//! `command-panel 576×47` (`-mx-px`, so 2px wider than `command`), containing
//! a zero-height `command-empty` and the `574×46` scroll area this port
//! shares with [`super::scroll_area`]'s own fixture docs (the same instance,
//! already recorded there as "the command palette's"), `command-footer
//! 574×45`. One workspace exists in this dev environment
//! (`oracle-fixture`/`home`), so exactly one `command-item` (`558×30`,
//! `data-highlighted` — `autoHighlight="always"` on a one-row list) is ever
//! in the document at once, which is what keeps `ANCHORS.md` v1.8 satisfied
//! without a second declaration. `native/mapping/command.md` carries the
//! per-field table.
//!
//! # `font-editor`, and why it is not a second `font-heading`
//!
//! `dialog.md` found `font-heading` a **dead** utility — no `--font-heading`
//! at `:root`, so the class resolves to nothing and the title paints in the
//! inherited `font-sans`. `command-item`'s own `font-editor` class is
//! **not** dead: `theme.css` declares `--font-editor: var(--editor-font-family)`,
//! and `editor-theme.css` gives that a real static fallback
//! (`ui-monospace, 'JetBrains Mono Variable', monospace`), confirmed live —
//! the reachable item's own `getComputedStyle().fontFamily` leads with
//! `"JetBrains Mono Variable"`, matching this crate's own
//! [`Theme::font_mono`](crate::theme::Theme::font_mono) — `theme.css`'s
//! `--font-mono: var(--editor-font-family, 'JetBrains Mono Variable', …)`
//! is the *same* variable, read through a different custom property. So
//! unlike the title one module over, this item's own text (were it anchored
//! — it is not; see [`super::autocomplete::Item`]) would need
//! `theme.font_mono`, not an inherited `font_sans`. Recorded here because a
//! reader who only checked "is the class dead" the way `dialog.md` taught
//! would get the wrong answer applying the same test blind.

use gpui::{
    AnyElement, App, IntoElement as _, ParentElement as _, Pixels, Styled as _, Window, div, px,
};
use gpui_component::dialog::Dialog as GpuiDialog;

use super::anchor::AnchorSink;
use super::autocomplete::{self, Input as AutocompleteInput, List as AutocompleteList};
use crate::theme::{Color, Theme};

/// The root anchor: `CommandDialogPrimitive.Popup`. Every other bound on
/// this surface is reported relative to it (`native/oracle/ANCHORS.md` §4).
pub const ID_POPUP: &str = "command-dialog-popup";
/// `CommandPanel` — a real, painted box with **no `data-slot` in the source
/// at all**; see the module docs for why this port adds one.
pub const ID_PANEL: &str = "command-panel";
/// `CommandFooter`, rendered only where a call site nests one.
pub const ID_FOOTER: &str = "command-footer";

/// The anchors whose boxes size to their own text (`ANCHORS.md` v1.5).
///
/// **None.** The popup is `w-full` capped by `max-w-xl`; the panel and the
/// footer are the popup's own width less its border/margin; nothing on this
/// surface is a box whose used width is a text run's max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose **box height is their own line box**
/// (`ANCHORS.md` v1.6).
///
/// **None**, unlike `dialog`'s title: this surface has no anchor that is a
/// bare `leading-none` text run. `command-item`'s content (which would be
/// the closest candidate) is call-site-owned and unanchored — see
/// `autocomplete.rs`'s `Item`.
pub const LINE_SIZED: [&str; 0] = [];

/// `border` on the popup, on the panel (top/left/right only — `border
/// border-b-0`) and on the footer (`border-t`) — a real 1px border
/// throughout, the same value `dialog`'s and `popover`'s own root anchors
/// carry.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `CommandDialogViewport`'s `px-4`: 16px either side of the popup, the same
/// number `dialog::VIEWPORT_PADDING` carries (a coincidence of two
/// independently-authored `p-4`/`px-4` classes, not a shared constant — see
/// that module's own docs for why the components stay independent).
pub const VIEWPORT_PADDING: Pixels = px(16.0);

/// `max-h-105` on the popup: `calc(var(--spacing) * 105)`, 420px.
///
/// **Carried, not enforced.** The one reachable cell's natural content
/// height (140px) never approaches it, so there is no live reference for
/// what a clamped popup looks like — modelling the resulting flex-shrink
/// honestly would need a scrollable body this port has no reference to
/// validate against, the same call `dialog`'s `max_height`-less popup makes
/// implicitly by having no cap at all.
pub const MAX_HEIGHT: Pixels = px(420.0);

/// The unnamed `.px-2.5.py-1.5` wrapper `CommandInput` puts around
/// `AutocompleteInput` — horizontal.
pub const INPUT_ROW_PADDING_X: Pixels = px(10.0);
/// Ditto, vertical.
pub const INPUT_ROW_PADDING_Y: Pixels = px(6.0);

/// `CommandItem`'s own `py-1.5` override of `autocomplete.tsx`'s `py-1` —
/// tailwind-merge takes the later declaration, confirmed live: the reachable
/// row's own height (30px) matches `2×6 + 18`, not `2×4 + 18`.
pub const ITEM_PADDING_Y: Pixels = px(6.0);

/// `CommandList`'s own `not-empty:p-2` override of `autocomplete.tsx`'s
/// `not-empty:p-1` — same tailwind-merge reasoning, confirmed live: the
/// reachable list's own padding is 8px, not 4.
pub const LIST_PADDING: Pixels = px(8.0);

/// `CommandFooter`'s `px-5`.
pub const FOOTER_PADDING_X: Pixels = px(20.0);
/// `CommandFooter`'s `py-3`.
pub const FOOTER_PADDING_Y: Pixels = px(12.0);

/// The `command-dialog-popup`, the `command` wrapper, the input row, the
/// panel (empty state + list) and the optional footer, together.
#[derive(Clone, Debug, PartialEq)]
pub struct Command {
    /// `max-w-xl`: 576px, the class the popup carries unconditionally
    /// (unlike `dialog`'s per-call-site `max-w-md`/`max-w-lg`, every
    /// `CommandDialogPopup` in the file takes this one number).
    pub max_width: Pixels,
    /// `AutocompleteInput`'s own picture — see `autocomplete.rs`. The live
    /// reference is [`autocomplete::Input::fixture`] unmodified.
    pub input: AutocompleteInput,
    /// `AutocompleteList`'s own picture (the panel's empty state and its
    /// list) — see `autocomplete.rs`. The live reference is
    /// [`autocomplete::List::fixture`] unmodified.
    pub list: AutocompleteList,
    /// The footer's own content height, between its border and its padding.
    /// `None` omits the footer (and its anchor) entirely, and moves the
    /// panel's own bottom edge — `CommandPanel`'s `not-has-[+footer]:`
    /// variants, reproduced in [`Command::panel`].
    pub footer_content_height: Option<Pixels>,
}

impl Command {
    /// The live workspace switcher, reached from the Context Pill:
    /// `max-w-xl` (576px), `CommandInput`'s and `CommandList`'s own live
    /// fixtures unmodified, a footer whose two `<KbdGroup>`/`<Kbd>` rows come
    /// out at 20px.
    ///
    /// Measured at a 1714px viewport, dark: popup `576×142`, wrapper
    /// `574×140`, input row `574×48`, panel `576×47`, footer `574×45`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            max_width: px(576.0),
            input: AutocompleteInput::fixture(),
            list: AutocompleteList::fixture(),
            footer_content_height: Some(px(20.0)),
        }
    }

    /// The input row's own height: its two paddings around
    /// [`autocomplete::Input::control_height`]. Measured live at `48`
    /// (`2×6 + 36`).
    fn input_row_height(&self) -> f32 {
        f32::from(INPUT_ROW_PADDING_Y) * 2.0 + f32::from(self.input.control_height())
    }

    /// The panel's own height: its top border, the (always-mounted, see
    /// [`autocomplete::List::render`]) empty state and the list.
    ///
    /// The empty state's own contribution is `0` on every reachable cell
    /// ([`autocomplete::ListContent::Item`] — the empty node is mounted but
    /// carries no padding, see `autocomplete.rs`'s own `empty_box`); the
    /// `Empty` arm's `32` (`not-empty:p-2` around one text line) is modelled
    /// but has no live reference, the same call `autocomplete::List` itself
    /// makes about that branch.
    fn panel_height(&self) -> f32 {
        let empty_height = match &self.list.content {
            autocomplete::ListContent::Empty => 32.0,
            autocomplete::ListContent::Item(_) => 0.0,
        };
        f32::from(BORDER_WIDTH) + empty_height + f32::from(self.list.height)
    }

    /// The popup's own height: two borders around the input row, the panel
    /// and the footer where one is rendered.
    #[must_use]
    pub fn popup_height(&self) -> Pixels {
        let mut height = f32::from(BORDER_WIDTH) * 2.0;
        height += self.input_row_height();
        height += self.panel_height();
        if let Some(content) = self.footer_content_height {
            height +=
                f32::from(BORDER_WIDTH) + f32::from(FOOTER_PADDING_Y) * 2.0 + f32::from(content);
        }
        px(height)
    }

    /// Renders the popup through `gpui_component::dialog::Dialog`, opting
    /// every contract anchor into `anchors` — the same wrap technique
    /// `dialog::Dialog::render` uses, restated in full in the module docs.
    #[must_use]
    pub fn render(
        &self,
        window: &mut Window,
        cx: &mut App,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        // `w-full max-w-xl`, reproduced by hand against the window — exactly
        // `dialog::Dialog::render`'s own arithmetic, and the same reason: no
        // `Styled::w`/`max_w` reaches `GpuiDialog`'s fixed-pixel `w`/`max_w`
        // fields. Computed before `inner` so `panel()` can take the
        // wrapper's own content width explicitly — see that method's docs
        // for why `-mx-px` is reproduced as an explicit width rather than a
        // negative margin on an implicitly-stretched box.
        let viewport_width = f32::from(window.viewport_size().width);
        let outer_width = px((viewport_width - f32::from(VIEWPORT_PADDING) * 2.0)
            .min(f32::from(self.max_width))
            .max(0.0));
        let inner_width = px((f32::from(outer_width) - f32::from(BORDER_WIDTH) * 2.0).max(0.0));

        let mut inner = div()
            .flex()
            .flex_col()
            .size_full()
            .rounded(theme.radius_2xl.value())
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .text_color(theme.popover_foreground)
            .font_family(theme.font_sans.primary().unwrap_or("sans-serif"));

        inner = inner.child(self.input_row(theme, anchors));
        inner = inner.child(self.panel(theme, anchors, inner_width));
        if let Some(content_height) = self.footer_content_height {
            inner = inner.child(Self::footer(theme, anchors, content_height));
        }

        let popup = anchors.root(ID_POPUP.into(), inner);

        GpuiDialog::new(cx)
            .overlay(false)
            .close_button(false)
            .p_0()
            .bg(Color::TRANSPARENT)
            .border_0()
            .border_color(Color::TRANSPARENT)
            .rounded(px(0.0))
            // The vendor's own unconditional 96px floor — see the module
            // docs and `dialog.rs`'s P3.28 finding.
            .min_h(px(0.0))
            .w(outer_width)
            .children([popup])
            .into_any_element()
    }

    /// `CommandInput`'s own `.px-2.5.py-1.5` wrapper around `AutocompleteInput`.
    ///
    /// **No anchor of its own** — the source gives it neither a `data-slot`
    /// nor a class this port has any other reason to name, and its box
    /// (position/extent only) is fully accounted for by
    /// [`Command::input_row_height`]; the real anchor one level in
    /// (`autocomplete-input-group`) is [`autocomplete::Input::render`]'s.
    fn input_row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .w_full()
            .px(INPUT_ROW_PADDING_X)
            .py(INPUT_ROW_PADDING_Y)
            .child(self.input.render(theme, anchors))
            .into_any_element()
    }

    /// `CommandPanel`: `border border-b-0 bg-popover rounded-t-xl`, plus the
    /// `not-has-[+footer]:` arms that only fire when
    /// [`Command::footer_content_height`] is `None`. Its own content is
    /// [`autocomplete::List`], reused with this module's own `py-1.5`/`p-2`
    /// overrides — see that struct's own module docs for why the padding
    /// fields exist on it rather than being hardcoded there.
    ///
    /// `wrapper_width` is `command`'s own content width (`inner_width` in
    /// [`Command::render`]) — `-mx-px` is reproduced as an **explicit**
    /// width (`wrapper_width + 2×BORDER_WIDTH`) plus a matching `-1px` left
    /// margin, rather than a bare negative margin on a `w_full()` box:
    /// measured first as `.w_full().mx(px(-1.0))`, which left the panel at
    /// the wrapper's own `574px` — an explicit `w_full()` pins the computed
    /// width to the container's exactly, and does not grow it the way an
    /// implicit (auto-width, stretched) CSS block's negative margin would.
    fn panel(&self, theme: &Theme, anchors: &dyn AnchorSink, wrapper_width: Pixels) -> AnyElement {
        let has_footer = self.footer_content_height.is_some();
        let panel_width = px(f32::from(wrapper_width) + f32::from(BORDER_WIDTH) * 2.0);

        let mut panel = div()
            .relative()
            .w(panel_width)
            .ml(px(-f32::from(BORDER_WIDTH)))
            .flex()
            .flex_col()
            .min_h(px(0.0))
            .flex_1()
            .border(BORDER_WIDTH)
            .border_b(px(0.0))
            .border_color(theme.border)
            .bg(theme.popover)
            .rounded_t(theme.radius_xl.value());

        panel = if has_footer {
            panel.rounded_b(px(0.0))
        } else {
            // `not-has-[+[data-slot=command-footer]]:rounded-b-2xl` —
            // unreached by the one live call site, which always renders a
            // footer, modelled for the primitive's other picture.
            panel.rounded_b(theme.radius_2xl.value() - BORDER_WIDTH)
        };

        // `<CommandEmpty/><CommandList>…</CommandList>`, in that order —
        // siblings, both direct children of the panel. See
        // `autocomplete::empty`'s own docs for why this is not nested
        // inside `List::render`.
        let has_content = matches!(self.list.content, autocomplete::ListContent::Empty);
        panel = panel.child(autocomplete::empty(theme, anchors, has_content));

        let list = autocomplete::List {
            item_padding_y: ITEM_PADDING_Y,
            list_padding: LIST_PADDING,
            ..self.list.clone()
        };
        panel = panel.child(list.render(theme, anchors));

        anchors.boxed(ID_PANEL.into(), panel)
    }

    /// `CommandFooter`: `flex items-center justify-between gap-2 border-t
    /// px-5 py-3 rounded-b-[calc(var(--radius-2xl)-1px)]`.
    fn footer(theme: &Theme, anchors: &dyn AnchorSink, content_height: Pixels) -> AnyElement {
        let footer = div()
            .flex()
            .items_center()
            .justify_between()
            .w_full()
            .px(FOOTER_PADDING_X)
            .py(FOOTER_PADDING_Y)
            .border_t(BORDER_WIDTH)
            .border_color(theme.border)
            .rounded_b(theme.radius_2xl.value() - BORDER_WIDTH)
            .child(div().h(content_height));
        anchors.boxed(ID_FOOTER.into(), footer)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, Command, FOOTER_PADDING_X, FOOTER_PADDING_Y, ID_FOOTER,
        ID_PANEL, ID_POPUP, INPUT_ROW_PADDING_X, INPUT_ROW_PADDING_Y, ITEM_PADDING_Y, LINE_SIZED,
        LIST_PADDING, MAX_HEIGHT, VIEWPORT_PADDING,
    };
    use crate::components::autocomplete;
    use gpui::px;

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(INPUT_ROW_PADDING_X, px(STEP * 2.5)); // px-2.5
        assert_eq!(INPUT_ROW_PADDING_Y, px(STEP * 1.5)); // py-1.5
        assert_eq!(ITEM_PADDING_Y, px(STEP * 1.5)); // py-1.5
        assert_eq!(LIST_PADDING, px(STEP * 2.0)); // p-2
        assert_eq!(FOOTER_PADDING_X, px(STEP * 5.0)); // px-5
        assert_eq!(FOOTER_PADDING_Y, px(STEP * 3.0)); // py-3
        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // px-4
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(MAX_HEIGHT, px(STEP * 105.0)); // max-h-105
    }

    /// **`command.tsx`'s own overrides beat `autocomplete.tsx`'s defaults**,
    /// confirmed live and asserted here so the override cannot silently
    /// revert to the primitive's own `py-1`/`p-1`.
    #[test]
    fn command_overrides_beat_autocompletes_own_defaults() {
        assert_ne!(ITEM_PADDING_Y, autocomplete::ITEM_PADDING_Y);
        assert_ne!(LIST_PADDING, autocomplete::LIST_PADDING);
        assert!(ITEM_PADDING_Y > autocomplete::ITEM_PADDING_Y);
        assert!(LIST_PADDING > autocomplete::LIST_PADDING);
    }

    /// The fixture is the live workspace switcher, measured off the running
    /// app.
    #[test]
    fn the_fixture_is_the_live_workspace_switcher() {
        let cmd = Command::fixture();

        assert_eq!(cmd.max_width, px(576.0));
        assert_eq!(cmd.footer_content_height, Some(px(20.0)));
        assert_eq!(cmd.input, autocomplete::Input::fixture());
        assert_eq!(cmd.list.width, px(574.0));
        assert_eq!(cmd.list.height, px(46.0));

        assert_eq!(cmd.popup_height(), px(142.0));
    }

    /// Losing the footer moves the popup by exactly the footer's own
    /// contribution.
    #[test]
    fn losing_the_footer_shrinks_the_popup_by_its_own_contribution() {
        let with_footer = Command::fixture();
        let mut without = with_footer.clone();
        without.footer_content_height = None;

        let footer_contribution =
            f32::from(BORDER_WIDTH) + f32::from(FOOTER_PADDING_Y) * 2.0 + 20.0;
        assert_eq!(
            without.popup_height(),
            px(f32::from(with_footer.popup_height()) - footer_contribution),
        );
    }

    /// A taller footer content row moves the popup by exactly its own delta.
    #[test]
    fn a_taller_footer_moves_the_popup_by_its_own_delta() {
        let base = Command::fixture();
        let mut taller = base.clone();
        taller.footer_content_height = Some(px(40.0));

        assert_eq!(
            taller.popup_height(),
            px(f32::from(base.popup_height()) + 20.0),
        );
    }

    /// Neither declaration list claims anything.
    #[test]
    fn neither_declaration_list_claims_anything() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// Every anchor id this module mints is distinct, namespaced, and none
    /// of them collides with `autocomplete`'s own — the two vocabularies
    /// this module's own boxes and the shared ones between them.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_PANEL, ID_FOOTER];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("command-")));
    }

    /// The panel's own height folds the (always-mounted, zero-contribution)
    /// empty state and the list — the arithmetic the live `576×47` panel
    /// satisfies (`1 + 0 + 46`).
    #[test]
    fn the_panel_height_is_a_border_plus_the_list() {
        let cmd = Command::fixture();
        let expected = f32::from(BORDER_WIDTH) + f32::from(cmd.list.height);
        assert!((cmd.panel_height() - expected).abs() < f32::EPSILON);
        assert!((expected - 47.0).abs() < f32::EPSILON);
    }

    // `Command::render` itself is exercised in `row_layout/command.rs`,
    // which has the real window `GpuiDialog::new` needs — see `dialog.rs`'s
    // own module docs for why `crowbar-ui`'s own tests cannot call it
    // directly (`crowbar-driver`, which this crate cannot depend on, is
    // what supplies the window harness).
}
