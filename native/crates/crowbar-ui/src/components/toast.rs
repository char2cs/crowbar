//! `toast` — P3.28's second item, and the one that fails the wrap test *and*
//! has no live producer at all.
//!
//! The native half of `web/src/components/ui/toast.tsx`'s `AnchoredToasts` —
//! the file's only rendering export, reached through `AnchoredToastProvider`
//! (mounted once, at `routes/__root.tsx`). Every value below is derived from
//! the app's own compiled Tailwind class list, the method `native/MAPPING.md`
//! fixes — **not measured live**, because there is nothing live to measure; see
//! §2.
//!
//! # §1. Wrap or build: the seam test, applied to `gpui_component::Notification`
//!
//! `native/vendor/gpui-component/src/notification.rs` has a `Notification` and
//! a `NotificationList` — real candidates under §10.1. Applying `popover`'s own
//! test (**"a widget is wrappable-and-measurable exactly when it lets the
//! caller supply an *element*, not merely a style"**), read against the vendor
//! source rather than a member-name grep (a fixed-list grep is exactly what
//! under-counted `popover`'s own seam by missing `focus_trap` and
//! `v_virtual_list` — this item's brief names both):
//!
//! * `Notification::content` **does** take a builder —
//!   `Fn(&mut Self, &mut Window, &mut Context<Self>) -> AnyElement + 'static`
//!   — which is the exact shape `popover.rs`'s own module docs already ruled
//!   out: `'static`, and a component here is handed `&dyn AnchorSink` with an
//!   anonymous lifetime, which cannot be captured by a `'static` closure. Not a
//!   coincidence of naming — `gpui_component::popover::Popover::content` has
//!   the identical signature, for the identical reason.
//! * **Unlike `Popover`, there is no fallback.** `Popover` also has
//!   `ParentElement`/`.child()`, which takes an *already-built* `AnyElement` —
//!   no closure, no `'static` bound — and that is the seam `dialog.rs`,
//!   `sheet.rs` and `popover.rs` all actually use. `Notification` implements
//!   neither `ParentElement` nor anything with an equivalent shape: its whole
//!   painted box (`h_flex().id("notification")…bg(cx.theme().tokens.popover)…
//!   rounded(cx.theme().radius_lg)…`) is built inside its own private
//!   `Render::render`, and the only other seam is `Styled::style()` — a
//!   `StyleRefinement` on that same private `h_flex`, the "nothing to anchor"
//!   shape `tooltip.rs`'s module docs and this item's brief both name as the
//!   fake convergence `ANCHORS.md` exists to refuse: `AnchorSink::root`/
//!   `boxed` take a `Div` this crate constructs, and no call into
//!   `Notification` ever produces one.
//! * `gpui_component::alert::Alert` (also named in this item's brief, and
//!   sharing a name with `alert-dialog` and nothing else — see
//!   `alert_dialog.rs`'s module docs for that unrelated finding) fails the same
//!   way, one door further shut: `title`/`message` are `SharedString`/`Text`,
//!   not elements, `icon` is a fixed `Icon` type, and there is no `content`
//!   closure at all, `'static`-bound or otherwise.
//!
//! **Verdict: built, not wrapped** — the third component in this tree to fail
//! the test outright (`dropdown_menu`, `checkbox`, `tooltip` are the others;
//! see `tooltip.rs`'s module docs for the same test applied to a different
//! vendor type), and, like `tooltip`, for a *structural* reason rather than a
//! missing method.
//!
//! # §2. Reachability: zero, and it is provable rather than merely unobserved
//!
//! `toast.tsx` exports two things: `toastManager` (a bare
//! `Toast.createToastManager()` singleton, re-exported through
//! `lib/toast-manager.ts` for stores to import without crossing the
//! stores-must-not-import-from-components rule) and `anchoredToastManager` +
//! `AnchoredToastProvider`/`AnchoredToasts` — a **second**, independent
//! manager and the only component that actually renders this file's own JSX
//! (`upsertReplayClassName`, the icon set, the `tooltipStyle` branch — all of
//! it lives inside `AnchoredToasts`, and nowhere else in `web/src`).
//!
//! `AnchoredToasts` calls `Toast.useToastManager()` bound to
//! `anchoredToastManager` and, per toast, `if (!positionerProps?.anchor) return
//! null` — so it paints something only when some caller has already called
//! `anchoredToastManager.add(…)` with a `positionerProps.anchor` set. **No
//! caller ever does.** `grep -rn anchoredToastManager web/src` finds exactly
//! three lines: the singleton's own declaration, `AnchoredToastProvider`'s own
//! `<Toast.Provider toastManager={anchoredToastManager}>`, and
//! `lib/toast-manager.ts`'s re-export — no `.add(` anywhere. Every real toast
//! the app shows a user goes through the **other** manager
//! (`features/window/stores/toast-store.ts`'s `toast.show`/`.info`/`.success`/
//! `.warning`/`.error`, all `toastManager.add(…)`), rendered by a **third,
//! unrelated file** — `components/layout/sidebar-toast-overlay.tsx`'s own
//! hand-rolled `SidebarToastItem`, which imports none of `toast.tsx`'s JSX,
//! only the `toastManager` object. `AppDialog`'s relationship to `DialogPopup`
//! one door over is the same shape: a second, independent renderer that
//! bypasses the primitive entirely — except `AppDialog` is at least *reachable
//! through some path*, and `AnchoredToasts` has none.
//!
//! This is unlike `alert-dialog`'s finding (see `alert_dialog.rs`'s module
//! docs): that component has real, driveable code blocked by an environmental
//! defect this session did not introduce. `AnchoredToasts` has **no code path,
//! in any environment**, that ever calls `anchoredToastManager.add` with an
//! anchor — the brief's "if you cannot reach the real element, STOP and
//! report" applies here in its strongest form, and no reference was captured,
//! attempted or fabricated.
//!
//! # §3. `popover`'s `Variant::Tooltip` does not already cover this
//!
//! The brief asks the question directly, and `popover.rs`'s own module docs
//! already answer half of it: `Variant::Tooltip` models `PopoverPopup`'s own
//! `tooltipStyle` prop, found by `grep tooltipStyle` to be reached "on
//! `toast.tsx`'s own primitive and on no `PopoverContent` anywhere" — i.e.
//! `popover.tsx`'s `tooltipStyle` arm is *itself* unreached, and was modelled
//! by reading source, the same position this file is in. So the question is
//! not "is `Variant::Tooltip` reachable" (no) but "is it the *same shape* as
//! `toast.tsx`'s `tooltipStyle` branch, so this file could reuse it instead of
//! duplicating it" — and reading both class lists side by side, the CSS
//! **values** agree almost everywhere (`rounded-md`, `text-xs` → this crate's
//! `ui_text_sm`, and `popover`'s tooltip viewport padding `py-1`
//! `[--viewport-inline-padding:--spacing(2)]` is the same `4px`/`8px` pair as
//! `toast.tsx`'s own `Toast.Content className="… px-2 py-1"` under
//! `tooltipStyle`) while the **shapes** do not:
//!
//! | | `popover::Variant::Tooltip` | `toast`'s `tooltipStyle` |
//! |---|---|---|
//! | outer box width | a **required, caller-supplied `Pixels`** (`Popover::width` — every live call site's own `w-*` class, `repo-icon-popover`'s measured 256px) | **none** — `Toast.Root` has no width class at all; `Toast.Positioner`'s `max-w-[min(--spacing(64),var(--available-width))]` is a *cap*, not a length, and the root shrinks to its content exactly the way `tooltip.rs`'s own root does |
//! | title, when present | a *separate*, always-styled `PopoverTitle` (`font-semibold text-lg leading-none`) nested in the viewport alongside arbitrary `children` | **is** the box's only content, and carries **no className of its own** under this branch — plain, inherited 12px, not semibold, not `leading-none` |
//! | driven by | a click-to-open trigger + `gpui_component::Popover`'s deferred/anchored placement, open/closed state | a timed, queued notification list (`Toast.Root`/`Toast.Positioner` pairs, one per active toast, stacked by a manager) — no trigger, no vendor widget in the render path at all (§1) |
//!
//! A caller that reused `Popover::render(Variant::Tooltip, …)` for a toast
//! would therefore have to invent a width `toast.tsx` never authors and render
//! a title box shaped like `PopoverTitle` where the reference's is shaped like
//! nothing at all — two fabrications for one reused module. **Verdict: related
//! by coincidence of both being unreached and both being "the small padded
//! arm" of a two-arm component, not by one covering the other.** This module
//! is its own, independent port, exactly as `tooltip.rs`'s own §"`tooltip.tsx`
//! is not `popover --tooltip`" finding is for a different pair of surfaces.
//!
//! # Declarations
//!
//! `CONTENT_SIZED` and `LINE_SIZED` are both empty, and deliberately so rather
//! than by omission:
//!
//! * The **popup** is not v1.5-content-sized in the sense that declaration
//!   requires (a box whose used width *is* a text run's max-content width,
//!   `dialog.rs`'s own phrase): under the default variant it is a multi-child
//!   flex subtree (an icon column beside a title/description column), and even
//!   under `tooltipStyle` — where it *is* one run — the same anchor id has to
//!   mean one thing across both configurations, the way `dialog::ID_TITLE`
//!   means one thing whether or not a description sits beside it. It is,
//!   however, built with **no authored width** either way — `.max_w(MAX_WIDTH)`
//!   only, no `.w()` — so gpui's own flex layout produces the same
//!   shrink-to-fit box `WebKit` would, undeclared rather than mis-declared.
//! * Neither **title** nor **description** carries `leading-none` in either
//!   branch — unlike every title this tree has ported so far
//!   (`dialog`/`popover`/`sheet`'s), `toast.tsx`'s title keeps the ambient
//!   paired line height and is `break-words` prose under the default variant,
//!   exactly `dialog::ID_DESCRIPTION`'s own shape (never `dialog::ID_TITLE`'s).
//!   Declaring either would manufacture the same delta v1.6 exists to prevent.

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
    relative,
};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Theme, ui_sans_font};

/// The root anchor: `Toast.Root`, which carries `data-slot="toast-popup"` in
/// the reference. Every other bound on this surface is reported relative to
/// it (`native/oracle/ANCHORS.md` §4).
pub const ID_POPUP: &str = "toast-popup";
/// `Toast.Title`. Present under both variants.
pub const ID_TITLE: &str = "toast-title";
/// `Toast.Description` — the default variant only.
pub const ID_DESCRIPTION: &str = "toast-description";
/// The leading type icon — the default variant only, and only when the toast
/// carries a `type`.
pub const ID_ICON: &str = "toast-icon";

/// See the module docs' "Declarations" section: empty on both counts, for
/// reasons distinct from each other and stated in full there.
pub const CONTENT_SIZED: [&str; 0] = [];
/// See the module docs.
pub const LINE_SIZED: [&str; 0] = [];

/// `border` on the root — a real 1px border, the same bare class every other
/// wrap/build in this tree with a `border-popover` box carries.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `Toast.Positioner`'s `max-w-[min(--spacing(64),var(--available-width))]` —
/// the cap, not a length. `--spacing(64)` = 256px; the harness has no
/// `available-width` collision to model, so this is the binding arm.
pub const MAX_WIDTH: Pixels = px(256.0);

/// `tooltipStyle`'s `Toast.Content`: `px-2 py-1`.
pub const TOOLTIP_PADDING_X: Pixels = px(8.0);
/// `tooltipStyle`'s `Toast.Content`: `py-1`.
pub const TOOLTIP_PADDING_Y: Pixels = px(4.0);

/// The default variant's `Toast.Content`: `px-3.5`.
pub const RICH_PADDING_X: Pixels = px(14.0);
/// The default variant's `Toast.Content`: `py-3`.
pub const RICH_PADDING_Y: Pixels = px(12.0);
/// The default variant's `Toast.Content`: `flex flex-col gap-2` — between the
/// icon/title row and the optional action row.
pub const RICH_GAP: Pixels = px(8.0);
/// The icon row's own `flex gap-2` — between the icon and the title/
/// description column.
pub const ICON_ROW_GAP: Pixels = px(8.0);
/// The title/description column's `flex flex-col gap-0.5`.
pub const COLUMN_GAP: Pixels = px(2.0);
/// `[&>svg]:w-4` on the leading icon.
pub const ICON_WIDTH: Pixels = px(16.0);

/// `text-xs` on `Toast.Root` (this project's `ui_text_sm`, the Tailwind-number
/// trade `badge`/`popover`/`dropdown_menu` all make) — the size that survives
/// under `tooltipStyle`, where nothing inside overrides it.
const TOOLTIP_LINE_HEIGHT: f32 = 1.0 / 0.75;

/// `text-sm` on the default variant's `Toast.Content` (this project's
/// `ui_text_base`) — overrides the root's `text-xs` for everything inside it,
/// including the title and description.
const RICH_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// Which of `toast.tsx`'s two appearances a toast takes.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Variant {
    /// The prop's own default: an optional type icon, a title, an optional
    /// description, an optional action row.
    #[default]
    Default,
    /// `tooltipStyle`: the box's only content is its title, plain and
    /// unstyled — see the module docs §3 for why this is not `popover`'s
    /// `Variant::Tooltip` reused.
    Tooltip,
}

/// One toast popup: `Toast.Root` plus whichever of `Toast.Content`'s children
/// the variant and the call site produce.
///
/// # Why the icon and the action are **unanchored placeholders**
///
/// `dialog::Dialog::body` and `alert_dialog::AlertDialog::body` set the
/// precedent this follows for a call site's own content: the icon is a
/// `lucide-react` SVG a call site's `type` selects (no native equivalent, the
/// same call `badge::Badge::glyph_box` makes) and the action is a full
/// `<Button>` whose *label* is the call site's own string
/// (`toast.actionProps.children`) — both real, both rendered, neither
/// anchored, because there is nothing on the other side to compare either to
/// and — for the action specifically — `dialog`'s own two footer buttons are
/// this exact shape one component over: call-site content collapsed to the
/// height it occupies.
#[derive(Clone, Debug, PartialEq)]
pub struct Toast {
    /// Which appearance.
    pub variant: Variant,
    /// `Toast.Title`'s own text. Always present — `toast-store.ts`'s
    /// `ShowOptions.message` is a required field on every producer this app
    /// has, `toastManager`'s own included.
    pub title: SharedString,
    /// `Toast.Description`, when present. **The default variant only** —
    /// `tooltipStyle`'s `Toast.Content` nests nothing but the title (module
    /// docs §"Declarations").
    pub description: Option<SharedString>,
    /// Whether a leading type icon renders (`toast.type` is set). **The
    /// default variant only.**
    pub icon: bool,
    /// The action row's own content height, when `toast.actionProps` is set.
    /// `None` omits the row entirely. **The default variant only.**
    pub action_height: Option<Pixels>,
}

impl Toast {
    /// A representative default-variant toast: an icon, a title, a
    /// description, no action — `toast-store.ts`'s `toast.success(message,
    /// description)` shape, the two-argument form every live call site that
    /// passes a description uses.
    ///
    /// **No live pixels — see the module docs §2.** `title`/`description` are
    /// real strings a real call site could pass (not measured, because there
    /// is nothing running to measure); every length below is a Tailwind class
    /// compiled by hand, the same method every unreached component in this
    /// tree uses (`sheet::Sheet::fixture`, `radio_group`).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            variant: Variant::Default,
            title: SharedString::new_static("Saved"),
            description: Some(SharedString::new_static("Your changes have been saved.")),
            icon: true,
            action_height: None,
        }
    }

    /// The `tooltipStyle` fixture: a bare title, nothing else — the one
    /// `toast.tsx` itself renders under that branch regardless of what a
    /// caller passes.
    #[must_use]
    pub fn fixture_tooltip() -> Self {
        Self {
            variant: Variant::Tooltip,
            title: SharedString::new_static("Copied"),
            description: None,
            icon: false,
            action_height: None,
        }
    }

    /// The popup's own height: two borders, the variant's own padding, and its
    /// content.
    ///
    /// Every term here is the same kind of estimate `dialog::Dialog::popup_height`
    /// documents for its description: real arithmetic, used only to size the
    /// row-layout harness's window, never compared against a reference because
    /// none exists. The one-line assumption for the title/description is the
    /// same one; unlike `dialog`'s title (`leading-none`, genuinely always one
    /// line) `toast`'s can wrap, so a long title makes this an
    /// **under-estimate** — stated rather than hidden, and harmless here
    /// because nothing downstream trusts this number for more than window
    /// height.
    #[must_use]
    pub fn popup_height(&self, theme: &Theme) -> Pixels {
        match self.variant {
            Variant::Tooltip => {
                let line = theme.ui_text_sm.value().0 * 16.0 * TOOLTIP_LINE_HEIGHT;
                px(f32::from(BORDER_WIDTH) * 2.0 + f32::from(TOOLTIP_PADDING_Y) * 2.0 + line)
            }
            Variant::Default => {
                let line = theme.ui_text_base.value().0 * 16.0 * RICH_LINE_HEIGHT;
                let mut column = line;
                if self.description.is_some() {
                    column += f32::from(COLUMN_GAP) + line;
                }
                // The icon row's own height is the taller of the icon (which is
                // itself exactly one content line tall, `h-lh`) and the column
                // — never the icon, since the column is at least one line
                // itself. See `ICON_WIDTH`'s doc comment for the rest of the
                // icon's box.
                let mut content = column;
                if let Some(action) = self.action_height {
                    content += f32::from(RICH_GAP) + f32::from(action);
                }
                px(f32::from(BORDER_WIDTH) * 2.0 + f32::from(RICH_PADDING_Y) * 2.0 + content)
            }
        }
    }

    /// Renders the popup, opting every contract anchor into `anchors`.
    ///
    /// No `window`/`cx` needed — unlike `dialog`/`sheet`, nothing here wraps a
    /// `gpui-component` widget (§1), so there is no `FocusHandle` to mint.
    ///
    /// # Two divergences from the source tree shape, both deliberate
    ///
    /// * **`.flex()` on the root**, though `Toast.Root` carries no `flex`
    ///   class of its own. The reference's shrink-to-fit width comes from
    ///   `Toast.Positioner`'s absolute/fixed placement — an out-of-flow box
    ///   with no declared width sizes to its content by default in CSS — which
    ///   this crate's row-layout harness does not reproduce (every surface is
    ///   drawn inside a `--width`-sized container). `.flex()` is what makes
    ///   gpui's own layout shrink-wrap the box the same way, the identical
    ///   trade `tooltip::Tooltip::shell` makes for the same reason.
    /// * **`Toast.Content`'s padding is folded onto the root box directly**,
    ///   rather than modelled as a second, nested anchored div. `Content`
    ///   carries no border, background or radius of its own in either
    ///   variant — those are all on `Toast.Root` — so a child's padding with a
    ///   parent's zero-margin border produces an identical outer box either
    ///   way; the fold exists because `Content` earns no anchor of its own
    ///   (module docs' "Declarations" section only names four ids, and
    ///   `toast-content` — which carries no `data-slot` in the source either —
    ///   is not one of them).
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let root = div()
            .flex()
            .max_w(MAX_WIDTH)
            .rounded(self.variant.radius(theme))
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .text_color(theme.popover_foreground)
            .font(ui_sans_font(theme))
            .font_weight(FontWeight::NORMAL)
            .text_size(theme.ui_text_sm.value())
            .line_height(relative(TOOLTIP_LINE_HEIGHT));

        let inner = match self.variant {
            Variant::Tooltip => {
                root.px(TOOLTIP_PADDING_X)
                    .py(TOOLTIP_PADDING_Y)
                    .child(anchors.boxed_text(
                        AnchorId::new(ID_TITLE),
                        Self::tooltip_title_box(theme),
                        self.title.clone(),
                    ))
            }
            Variant::Default => {
                let mut content = div()
                    .flex()
                    .flex_col()
                    .gap(RICH_GAP)
                    .text_size(theme.ui_text_base.value())
                    .line_height(relative(RICH_LINE_HEIGHT))
                    .child(self.icon_row(theme, anchors));
                if let Some(action) = self.action_height {
                    content = content.child(Self::action_row(action));
                }
                root.px(RICH_PADDING_X).py(RICH_PADDING_Y).child(content)
            }
        };

        anchors.root(ID_POPUP.into(), inner)
    }

    /// The default variant's icon-and-column row: `flex gap-2`, the optional
    /// icon, then the title/description column.
    fn icon_row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut row = div().flex().gap(ICON_ROW_GAP);
        if self.icon {
            row = row.child(anchors.boxed(AnchorId::new(ID_ICON), Self::icon_box(theme)));
        }
        row.child(self.column(theme, anchors))
    }

    /// The icon's own box — an empty div, the same call `badge::Badge::glyph_box`
    /// and `tooltip`'s shortcut chip both make about a call site's own SVG:
    /// there is no native equivalent, and drawing a substitute would put a
    /// shape on screen for the oracle to converge on that this port never
    /// chose. `w-4` on the width; the height is `h-lh` — one line of the
    /// *content* column's own text, which is exactly [`RICH_LINE_HEIGHT`]'s
    /// pixel value, so it never grows the row past the column's own height
    /// (see [`Toast::popup_height`]'s doc comment).
    fn icon_box(theme: &Theme) -> Div {
        let height = px(theme.ui_text_base.value().0 * 16.0 * RICH_LINE_HEIGHT);
        div().flex_shrink_0().w(ICON_WIDTH).h(height)
    }

    /// The title/description column: `flex flex-col gap-0.5`.
    fn column(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut column = div()
            .flex()
            .flex_col()
            .gap(COLUMN_GAP)
            .child(anchors.boxed_text(
                AnchorId::new(ID_TITLE),
                Self::rich_title_box(theme),
                self.title.clone(),
            ));
        if let Some(description) = &self.description {
            column = column.child(anchors.boxed_text(
                AnchorId::new(ID_DESCRIPTION),
                Self::description_box(theme),
                description.clone(),
            ));
        }
        column
    }

    /// `tooltipStyle`'s `Toast.Title`: no className at all — plain text,
    /// inheriting the root's own size, weight and colour. Set explicitly
    /// rather than left to gpui's own inheritance, the convention every text
    /// leaf in this tree follows (`ANCHORS.md` v1.2).
    fn tooltip_title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_sm.value())
            .line_height(relative(TOOLTIP_LINE_HEIGHT))
            .font_weight(FontWeight::NORMAL)
            .text_color(theme.popover_foreground)
    }

    /// The default variant's `Toast.Title`: `min-w-0 break-words font-medium`.
    fn rich_title_box(theme: &Theme) -> Div {
        div()
            .min_w(px(0.0))
            .text_size(theme.ui_text_base.value())
            .line_height(relative(RICH_LINE_HEIGHT))
            .font_weight(FontWeight::MEDIUM)
            .text_color(theme.popover_foreground)
    }

    /// `Toast.Description`: `min-w-0 break-words text-muted-foreground`.
    fn description_box(theme: &Theme) -> Div {
        div()
            .min_w(px(0.0))
            .text_size(theme.ui_text_base.value())
            .line_height(relative(RICH_LINE_HEIGHT))
            .font_weight(FontWeight::NORMAL)
            .text_color(theme.muted_foreground)
    }

    /// The action row: `flex justify-end`, around a call site's own
    /// `<Toast.Action>` button — unanchored, see the type's own doc comment.
    fn action_row(content_height: Pixels) -> Div {
        div().flex().justify_end().child(div().h(content_height))
    }
}

impl Variant {
    /// `rounded-md` under `tooltipStyle`, `rounded-lg` otherwise.
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Pixels {
        match self {
            Self::Tooltip => theme.radius_md.value(),
            Self::Default => theme.radius_lg.value(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, COLUMN_GAP, CONTENT_SIZED, ICON_ROW_GAP, ICON_WIDTH, ID_DESCRIPTION, ID_ICON,
        ID_POPUP, ID_TITLE, LINE_SIZED, MAX_WIDTH, RICH_GAP, RICH_PADDING_X, RICH_PADDING_Y,
        TOOLTIP_PADDING_X, TOOLTIP_PADDING_Y, Toast, Variant,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(TOOLTIP_PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(TOOLTIP_PADDING_Y, px(STEP)); // py-1
        assert_eq!(RICH_PADDING_X, px(STEP * 3.5)); // px-3.5
        assert_eq!(RICH_PADDING_Y, px(STEP * 3.0)); // py-3
        assert_eq!(RICH_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(ICON_ROW_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(COLUMN_GAP, px(STEP * 0.5)); // gap-0.5
        assert_eq!(ICON_WIDTH, px(STEP * 4.0)); // w-4
        assert_eq!(MAX_WIDTH, px(STEP * 64.0)); // --spacing(64)
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    /// The two radii — `rounded-md`/`rounded-lg` — are this project's tokens,
    /// and distinct from each other, the same property `popover`'s and
    /// `tooltip`'s own two-radius tests hold.
    #[test]
    fn the_two_variants_take_distinct_radii() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(Variant::Tooltip.radius(&theme), px(8.0));
            assert_eq!(Variant::Tooltip.radius(&theme), theme.radius_md.value());
            assert_eq!(Variant::Default.radius(&theme), px(10.0));
            assert_eq!(Variant::Default.radius(&theme), theme.radius_lg.value());
            assert_ne!(
                Variant::Tooltip.radius(&theme),
                Variant::Default.radius(&theme)
            );
        }
    }

    /// Neither declaration list claims anything — see the module docs'
    /// "Declarations" section for why both are empty rather than omitted.
    #[test]
    fn nothing_is_declared_content_sized_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The default fixture is a real, plausible shape — an icon, a title, a
    /// description, no action — with no live pixels behind it (module docs
    /// §2).
    #[test]
    fn the_default_fixture_has_no_live_reference_but_is_a_real_shape() {
        let fixture = Toast::fixture();

        assert_eq!(fixture.variant, Variant::Default);
        assert_eq!(fixture.title.as_ref(), "Saved");
        assert_eq!(
            fixture.description.as_deref(),
            Some("Your changes have been saved.")
        );
        assert!(fixture.icon);
        assert_eq!(fixture.action_height, None);

        let theme = Theme::DARK;
        // 2 borders + 2×12 padding + column(20 title + 2 gap + 20 description).
        let expected = 2.0 + 24.0 + (20.0 + 2.0 + 20.0);
        assert!(
            (f32::from(fixture.popup_height(&theme)) - expected).abs() < 0.01,
            "{:?}",
            fixture.popup_height(&theme)
        );
    }

    /// The `tooltipStyle` fixture is the one shape `toast.tsx` itself renders
    /// under that branch regardless of caller — bare title, nothing else.
    #[test]
    fn the_tooltip_fixture_is_a_bare_title() {
        let fixture = Toast::fixture_tooltip();

        assert_eq!(fixture.variant, Variant::Tooltip);
        assert_eq!(fixture.title.as_ref(), "Copied");
        assert_eq!(fixture.description, None);
        assert!(!fixture.icon);

        let theme = Theme::DARK;
        // 2 borders + 2×4 padding + one 16px line (text-xs / ui_text_sm's own
        // pairing).
        let expected = 2.0 + 8.0 + 16.0;
        assert!(
            (f32::from(fixture.popup_height(&theme)) - expected).abs() < 0.01,
            "{:?}",
            fixture.popup_height(&theme)
        );
    }

    /// A description and an action row each move the popup's own height by
    /// exactly their own contribution, holding everything else fixed — the
    /// same arithmetic property `dialog`'s and `alert_dialog`'s equivalent
    /// tests hold, applied to a component with no reference to hold it against
    /// instead.
    #[test]
    fn the_popup_height_follows_the_description_and_the_action() {
        let theme = Theme::DARK;
        let mut fixture = Toast::fixture();
        fixture.description = None;
        let bare = fixture.popup_height(&theme);

        fixture.description = Some(gpui::SharedString::new_static("x"));
        let with_description = fixture.popup_height(&theme);
        assert_eq!(with_description, bare + px(2.0 + 20.0));

        fixture.action_height = Some(px(24.0));
        let with_action = fixture.popup_height(&theme);
        assert_eq!(with_action, with_description + px(8.0 + 24.0));
    }

    /// Every anchor id is distinct and namespaced.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_TITLE, ID_DESCRIPTION, ID_ICON];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("toast-")));
    }
}
