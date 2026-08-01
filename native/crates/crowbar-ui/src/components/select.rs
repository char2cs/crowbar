//! `select` — the second §6.2 wrap, and **the one that does not converge**.
//!
//! `gpui_component::Select` owns the behaviour: the list model, the keyboard
//! cursor (`up`/`down`/`enter`/`escape`), focus, blur-to-revert, the deferred
//! anchored popup and its dismissal. This module dresses it in [`Theme`].
//!
//! The React half is `web/src/components/ui/select.tsx`, a `cva` over
//! `@base-ui/react`'s `Select`.
//!
//! # ‼️ FINDING: this wrap cannot be measured, and no anchor is possible
//!
//! Read this before adding a surface for it. There is none, and the omission is
//! the result rather than a gap.
//!
//! [`AnchorSink`](super::AnchorSink)'s methods take a [`gpui::Div`] — an element
//! **this crate holds** — and wrap it in a recording element. Every box
//! `select.tsx` styles is built *inside* `gpui-component` and never passes
//! through here:
//!
//! | React anchor | who builds the box | reachable? |
//! |---|---|---|
//! | `select-trigger` | `SelectState::render`'s `div().id("input")` | no |
//! | `select-value` | `SelectState::render`'s `div().id("title")` | no |
//! | `select-icon` | `select::Caret`, an `Icon` | no |
//! | `select-popup` / `select-panel` / `select-list` | `SelectState::render`'s `deferred(anchored(v_flex(…)))` | no |
//! | `select-item` | `SearchableListItemElement::render`'s `h_flex()` | no |
//! | `select-item-indicator` | the same, nested two `h_flex`es deeper | no |
//!
//! `Select` exposes exactly **one** styling seam — its `Styled` impl, which
//! lands in `SelectState.style` and is applied to the trigger box with
//! `refine_style` — and a `StyleRefinement` is not an element. There is no
//! trigger builder, no item builder that yields the item's *box*, and no
//! content closure. The only thing this crate could anchor is a `div()` it
//! wraps *around* the whole widget, and that box is not `select-trigger`: it is
//! an extra layer whose bounds happen to coincide. A snapshot built from it
//! would compare one box and call the surface converged, which is the fake
//! convergence `ANCHORS.md` exists to refuse.
//!
//! So: **the appearance below is real and shipping; the parity claim is not
//! made.** `native/mapping/select.md` carries the full account and the two ways
//! out.
//!
//! # What had to be overridden to reach our design
//!
//! | | |
//! |---|---|
//! | [`GpuiSelect::appearance`]`(false)` | the vendor's `true` paints `input_style(…)`, `cx.theme().input` and `cx.theme().radius` — **`gpui-component`'s theme**. Off, then repainted from [`Theme`] through `Styled`. |
//! | `.border_1()` survives it | the vendor sets a 1px border *unconditionally*, before the appearance block, and only the colour is inside it. That is the right width here — `select.tsx`'s class is a bare `border` — so only the colour is restated. |
//! | `input_size` / `input_text_size` | the vendor derives the trigger's height, padding and text size from its own `Size` enum. Every one of them is overridden with [`Size`]'s, because `select.tsx`'s three sizes are not `gpui-component`'s three. |
//!
//! # What resisted, precisely
//!
//! Four differences remain that no amount of styling reaches, because each is a
//! *structural* choice inside the vendor's `render`:
//!
//! 1. **The caret is the wrong glyph.** `select.tsx` uses lucide's
//!    `ChevronsUpDown` — two chevrons, pointing apart. `select::Caret` renders
//!    `IconName::ChevronDown`, and `Select::icon` can only replace it with
//!    another `gpui_component::Icon`, whose set has no `ChevronsUpDown`
//!    (`grep -c ChevronsUpDown vendor/gpui-component/src/icon.rs` is 0).
//! 2. **The trigger's internal gap is 4px, not 6.** The vendor's inner
//!    `h_flex()` carries `gap_1()`; `select.tsx` at `size="sm"` carries
//!    `gap-1.5`. That flex is private and `refine_style` reaches the *outer*
//!    box only.
//! 3. **The value box is `w-full`, not `flex-1`.** With a sibling caret those
//!    are different lengths, so the truncation point differs — and
//!    `text_width` + `clipped` are the pair `ANCHORS.md` exists to catch that
//!    with.
//! 4. **The item is a flex row, not a two-column grid.**
//!    `SearchableListItemElement` is `h_flex` with a trailing check;
//!    `select.tsx`'s item is `grid grid-cols-[1rem_1fr]` with a **leading**
//!    indicator. The tick is on the other side of the row.
//!
//! (1) and (2) are cosmetic-but-visible; (3) and (4) would each be a delta on
//! every cell of the matrix, if a cell could be taken at all.

use gpui::{
    App, ElementId, IntoElement, Pixels, RenderOnce, SharedString, Styled as _, Window, px,
};
use gpui_component::{
    IndexPath,
    select::{Select as GpuiSelect, SelectState},
};

use crate::theme::Theme;

/// The anchor ids `select.tsx` carries, kept here so the two sides name the
/// same strings the day the finding above is resolved.
///
/// **Nothing in this module emits them.** See the module docs: there is no box
/// this crate holds to hang one on. They are `pub` so that a future item — a
/// widened `AnchorSink`, or a forked widget — has one place to start rather
/// than re-deriving the set off the DOM.
pub mod ids {
    /// `SelectPrimitive.Trigger`.
    pub const TRIGGER: &str = "select-trigger";
    /// `SelectPrimitive.Value`.
    pub const VALUE: &str = "select-value";
    /// `SelectPrimitive.Icon`.
    pub const ICON: &str = "select-icon";
    /// `SelectPrimitive.Popup`.
    pub const POPUP: &str = "select-popup";
    /// The bordered `<div>` inside the popup that paints `bg-popover`.
    pub const PANEL: &str = "select-panel";
    /// `SelectPrimitive.List`.
    pub const LIST: &str = "select-list";
    /// `SelectPrimitive.Item`.
    pub const ITEM: &str = "select-item";
    /// `SelectPrimitive.ItemIndicator`.
    pub const ITEM_INDICATOR: &str = "select-item-indicator";
    /// `SelectPrimitive.ItemText`.
    pub const ITEM_TEXT: &str = "select-item-text";
}

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `border` on the trigger and on the popup's panel — 1px of **real** border,
/// which is what the vendor's unconditional `.border_1()` already is.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `min-w-36` — the trigger's floor when a call site sets no width.
pub const TRIGGER_MIN_WIDTH: Pixels = px(SPACING * 36.0);

/// `p-1` on the popup's list.
pub const LIST_PADDING: Pixels = px(SPACING);

/// `gap-2` between an item's indicator column and its text column.
pub const ITEM_GAP: Pixels = px(SPACING * 2.0);

/// `grid-cols-[1rem_1fr]` — the indicator column is a flat `1rem`.
pub const ITEM_INDICATOR_COLUMN: Pixels = px(16.0);

/// `py-1` on an item.
pub const ITEM_PADDING_Y: Pixels = px(SPACING);

/// `ps-2` — an item's leading padding.
pub const ITEM_PADDING_LEADING: Pixels = px(SPACING * 2.0);

/// `pe-4` — an item's trailing padding.
pub const ITEM_PADDING_TRAILING: Pixels = px(SPACING * 4.0);

/// `text-sm`'s line height — `calc(1.25 / 0.875)`, 14px text on a 20px line.
const TEXT_SM_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `text-base`'s line height — `calc(1.5 / 1)`, 16px text on a 24px line.
const TEXT_BASE_LINE_HEIGHT: f32 = 1.5;

/// Which side of `select.tsx`'s `sm:` breakpoint the **viewport** is on.
///
/// A parameter and not a media query, for the reason `input`'s is: gpui has no
/// `@media`, and `ANCHORS.md` keys its matrix on the viewport width — so the
/// breakpoint is a fact about the cell that the component is told.
///
/// **`select.tsx` crosses it four times**, and the brief's warning that the trap
/// "fires in both directions" is literal here: `min-h-9 sm:min-h-8` and
/// `min-h-8 sm:min-h-7` *shrink* above 640px, while `size-4.5 sm:size-4` and
/// `text-base sm:text-sm` shrink too. Everything gets smaller, which is the
/// opposite of the usual mobile-first direction and is easy to write backwards.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Breakpoint {
    /// Below `40rem`: the base utilities.
    Base,
    /// At or above `40rem`, which every window this app opens at satisfies.
    #[default]
    Sm,
}

/// `select.tsx`'s `selectTriggerVariants` sizes.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `size="default"` — `min-h-9 sm:min-h-8`, `px-[calc(--spacing(3)-1px)]`,
    /// `gap-2`.
    #[default]
    Default,
    /// `size="lg"` — `min-h-10 sm:min-h-9`, otherwise the default's.
    Large,
    /// `size="sm"` — `min-h-8 sm:min-h-7`, `px-[calc(--spacing(2.5)-1px)]`,
    /// `gap-1.5`. What every settings row uses.
    Small,
}

impl Size {
    /// `min-h-*`, on the given side of the breakpoint.
    ///
    /// Measured live at `size="sm"`, `Sm`: `28px`.
    ///
    /// **Spelled as a base plus a step rather than as a six-arm table**, and
    /// that is not a style preference — the six-arm version had `lg`'s two rows
    /// the wrong way round (base 8, `sm:` 9, where the class list says
    /// `min-h-10 sm:min-h-9`), and the accompanying test agreed with it because
    /// it was written from the same wrong table. `clippy::match_same_arms` is
    /// what surfaced it: the inverted pair created a duplicate body that the
    /// correct table does not have.
    ///
    /// The shape below makes the property structural instead: **every size
    /// shrinks by exactly one `--spacing` step above the breakpoint**, which is
    /// what all three of `min-h-9 sm:min-h-8`, `min-h-10 sm:min-h-9` and
    /// `min-h-8 sm:min-h-7` say. A row cannot now be inverted without inverting
    /// all three.
    #[must_use]
    pub const fn min_height(self, breakpoint: Breakpoint) -> Pixels {
        let base = match self {
            Self::Default => 9.0,
            Self::Large => 10.0,
            Self::Small => 8.0,
        };
        let steps = match breakpoint {
            Breakpoint::Base => base,
            Breakpoint::Sm => base - 1.0,
        };
        px(SPACING * steps)
    }

    /// The trigger's inline padding.
    ///
    /// **The `-1px` is the border**, written into the arithmetic rather than
    /// left implicit: `px-[calc(--spacing(3)-1px)]` is 12 − 1 = 11, and
    /// `px-[calc(--spacing(2.5)-1px)]` is 10 − 1 = 9. Measured live at `9px`.
    #[must_use]
    pub const fn padding_x(self) -> Pixels {
        let steps = match self {
            Self::Default | Self::Large => 3.0,
            Self::Small => 2.5,
        };
        px(SPACING * steps - f32::from_bits(BORDER_WIDTH_BITS))
    }

    /// The gap between the value and the caret.
    ///
    /// **Not reachable through the wrap** — see the module docs' finding 2. It
    /// is written down because it is the number the port would need, and
    /// because a mapping table that only records what worked is not a mapping
    /// table.
    #[must_use]
    pub const fn gap(self) -> Pixels {
        let steps = match self {
            Self::Default | Self::Large => 2.0,
            Self::Small => 1.5,
        };
        px(SPACING * steps)
    }
}

/// `1.0f32`'s bit pattern, so [`Size::padding_x`] can stay `const`.
///
/// `f32::from(Pixels)` is not a `const fn`, and spelling `1.0` twice is how a
/// border width comes to disagree with itself.
const BORDER_WIDTH_BITS: u32 = 1.0f32.to_bits();

/// The trigger's text size on the given side of the breakpoint: `text-base`,
/// or `sm:text-sm`.
///
/// Reached through the tokens whose *numbers* match Tailwind's, which is the
/// trade `dropdown_menu` documents — this system's `--ui-text-*` scale is not
/// Tailwind's and has no member for `text-base`'s 1rem other than
/// `--ui-text-lg`.
#[must_use]
pub fn text_size(breakpoint: Breakpoint, theme: &Theme) -> gpui::Rems {
    match breakpoint {
        Breakpoint::Base => theme.ui_text_lg.value(),
        Breakpoint::Sm => theme.ui_text_base.value(),
    }
}

/// The line box that goes with [`text_size`].
#[must_use]
pub const fn line_height(breakpoint: Breakpoint) -> f32 {
    match breakpoint {
        Breakpoint::Base => TEXT_BASE_LINE_HEIGHT,
        Breakpoint::Sm => TEXT_SM_LINE_HEIGHT,
    }
}

/// `[&_svg:not([class*='size-'])]:size-4.5 sm:…:size-4` — the caret's box.
#[must_use]
pub const fn icon_size(breakpoint: Breakpoint) -> Pixels {
    match breakpoint {
        Breakpoint::Base => px(18.0),
        Breakpoint::Sm => px(16.0),
    }
}

/// A select, wrapped.
///
/// A [`RenderOnce`] rather than a plain `render(&self, …)` method, because
/// `SelectState::new` needs a `&mut Window` and a `&mut Context` and the only
/// place this crate is given either is inside an element's own `render`.
/// `Window::use_keyed_state` then keys the state to the element id, so the list
/// model survives across frames exactly as `gpui_component`'s own call sites do.
#[derive(IntoElement)]
pub struct Select {
    id: ElementId,
    theme: Theme,
    size: Size,
    breakpoint: Breakpoint,
    width: Option<Pixels>,
    items: Vec<SharedString>,
    selected: Option<usize>,
    placeholder: Option<SharedString>,
    disabled: bool,
}

impl Select {
    /// A select over `items`, painted from `theme`.
    #[must_use]
    pub fn new(id: impl Into<ElementId>, theme: &Theme, items: Vec<SharedString>) -> Self {
        Self {
            id: id.into(),
            theme: theme.clone(),
            size: Size::default(),
            breakpoint: Breakpoint::default(),
            width: None,
            items,
            selected: None,
            placeholder: None,
            disabled: false,
        }
    }

    /// `size`.
    #[must_use]
    pub const fn size(mut self, size: Size) -> Self {
        self.size = size;
        self
    }

    /// Which side of `sm:` the viewport is on.
    #[must_use]
    pub const fn breakpoint(mut self, breakpoint: Breakpoint) -> Self {
        self.breakpoint = breakpoint;
        self
    }

    /// A call site's own width, over the primitive's `w-full min-w-36`.
    #[must_use]
    pub const fn width(mut self, width: Pixels) -> Self {
        self.width = Some(width);
        self
    }

    /// Which item is committed.
    #[must_use]
    pub const fn selected(mut self, index: usize) -> Self {
        self.selected = Some(index);
        self
    }

    /// The string shown when nothing is.
    #[must_use]
    pub fn placeholder(mut self, placeholder: impl Into<SharedString>) -> Self {
        self.placeholder = Some(placeholder.into());
        self
    }

    /// `data-disabled`.
    #[must_use]
    pub const fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }
}

impl RenderOnce for Select {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = self.theme;
        let size = self.size;
        let breakpoint = self.breakpoint;
        let items = self.items;
        let index = self.selected.map(IndexPath::new);

        let state = window.use_keyed_state(self.id, cx, move |window, cx| {
            SelectState::new(items, index, window, cx)
        });

        let mut select = GpuiSelect::new(&state)
            // The vendor's `true` would paint `gpui-component`'s theme; see the
            // module docs. Its unconditional `.border_1()` survives, which is
            // the width `select.tsx` wants anyway.
            .appearance(false)
            .disabled(self.disabled)
            // `min-w-(--anchor-width)` on the popup's panel. The vendor's
            // `Length::Auto` is the trigger's width plus 2 — the two borders —
            // which is the same quantity base-ui writes as `--anchor-width`.
            .menu_width(gpui::Length::Auto)
            .min_h(size.min_height(breakpoint))
            .px(size.padding_x())
            .rounded(theme.radius_lg.value())
            .border_color(theme.input)
            .bg(theme.background)
            .text_color(theme.foreground)
            .text_size(text_size(breakpoint, &theme))
            .line_height(gpui::relative(line_height(breakpoint)));

        select = match self.width {
            Some(width) => select.w(width),
            None => select.w_full().min_w(TRIGGER_MIN_WIDTH),
        };
        match self.placeholder {
            Some(placeholder) => select.placeholder(placeholder),
            None => select,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, Breakpoint, ITEM_GAP, ITEM_INDICATOR_COLUMN, ITEM_PADDING_LEADING,
        ITEM_PADDING_TRAILING, ITEM_PADDING_Y, LIST_PADDING, Size, TEXT_BASE_LINE_HEIGHT,
        TEXT_SM_LINE_HEIGHT, TRIGGER_MIN_WIDTH, icon_size, ids, line_height, text_size,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's Tailwind
    /// compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(TRIGGER_MIN_WIDTH, px(STEP * 36.0)); // min-w-36
        assert_eq!(LIST_PADDING, px(STEP)); // p-1
        assert_eq!(ITEM_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(ITEM_PADDING_Y, px(STEP)); // py-1
        assert_eq!(ITEM_PADDING_LEADING, px(STEP * 2.0)); // ps-2
        assert_eq!(ITEM_PADDING_TRAILING, px(STEP * 4.0)); // pe-4
        assert_eq!(ITEM_INDICATOR_COLUMN, px(16.0)); // grid-cols-[1rem_…]
        assert_eq!(BORDER_WIDTH, px(1.0)); // border
    }

    /// **The `sm:` trap, both directions.** Every one of the four utilities
    /// that crosses the breakpoint gets *smaller* above 640px, which is the
    /// opposite of the usual mobile-first direction. A port that assumed
    /// "wider viewport, bigger box" would have all four backwards.
    #[test]
    fn every_breakpoint_utility_shrinks_above_the_breakpoint() {
        for size in [Size::Default, Size::Large, Size::Small] {
            assert!(
                size.min_height(Breakpoint::Sm) < size.min_height(Breakpoint::Base),
                "{size:?}",
            );
        }
        assert!(icon_size(Breakpoint::Sm) < icon_size(Breakpoint::Base));

        for theme in [Theme::LIGHT, Theme::DARK] {
            assert!(text_size(Breakpoint::Sm, &theme).0 < text_size(Breakpoint::Base, &theme).0);
        }
    }

    /// The live settings row, measured off the running app at a 1714px
    /// viewport: `size="sm"` above the breakpoint is a **28px** box with **9px**
    /// of inline padding.
    #[test]
    fn the_live_settings_select_is_twenty_eight_by_nine() {
        assert_eq!(Size::Small.min_height(Breakpoint::Sm), px(28.0));
        assert_eq!(Size::Small.padding_x(), px(9.0));
        // And the `-1px` really is the border, not a magic number.
        assert_eq!(
            Size::Small.padding_x() + BORDER_WIDTH,
            px(10.0),
            "px-[calc(--spacing(2.5)-1px)] plus the border is a flat --spacing(2.5)",
        );
        assert_eq!(Size::Default.padding_x() + BORDER_WIDTH, px(12.0));
    }

    /// The three sizes, against the class list rather than against each other —
    /// `min-h-9 sm:min-h-8`, `min-h-10 sm:min-h-9`, `min-h-8 sm:min-h-7`.
    ///
    /// Written as the six literal boxes on purpose. The earlier version of this
    /// test compared the sizes *to one another* ("`lg` above the breakpoint is
    /// the base default's height"), which is true of the correct table **and of
    /// the inverted one** — so it passed while `lg` was 32/36 instead of 40/36.
    /// A test that only checks a relationship cannot catch a pair swapped
    /// inside it.
    #[test]
    fn each_size_is_its_own_two_utilities() {
        assert_eq!(Size::Default.min_height(Breakpoint::Base), px(36.0)); // min-h-9
        assert_eq!(Size::Default.min_height(Breakpoint::Sm), px(32.0)); // sm:min-h-8
        assert_eq!(Size::Large.min_height(Breakpoint::Base), px(40.0)); // min-h-10
        assert_eq!(Size::Large.min_height(Breakpoint::Sm), px(36.0)); // sm:min-h-9
        assert_eq!(Size::Small.min_height(Breakpoint::Base), px(32.0)); // min-h-8
        assert_eq!(Size::Small.min_height(Breakpoint::Sm), px(28.0)); // sm:min-h-7

        // And the structural property the implementation is built on: one
        // `--spacing` step across the breakpoint, the same for all three.
        for size in [Size::Default, Size::Large, Size::Small] {
            assert_eq!(
                size.min_height(Breakpoint::Base) - size.min_height(Breakpoint::Sm),
                px(4.0),
                "{size:?}",
            );
        }

        // `lg` moves the height and nothing else: it inherits the default's
        // padding and gap from the base class list.
        assert_eq!(Size::Large.padding_x(), Size::Default.padding_x());
        assert_eq!(Size::default(), Size::Default);
        assert_eq!(Breakpoint::default(), Breakpoint::Sm);
    }

    /// The two line boxes, spelled as the divisions the stylesheet writes.
    /// Measured live at `14px/20px`.
    #[test]
    fn the_line_heights_are_tailwinds_stock_pairs() {
        assert!((TEXT_SM_LINE_HEIGHT * 14.0 - 20.0).abs() < 0.001);
        assert!((TEXT_BASE_LINE_HEIGHT * 16.0 - 24.0).abs() < 0.001);
        assert!((line_height(Breakpoint::Sm) - TEXT_SM_LINE_HEIGHT).abs() < f32::EPSILON);
        assert!((line_height(Breakpoint::Base) - TEXT_BASE_LINE_HEIGHT).abs() < f32::EPSILON);
    }

    /// The gap is written down and **is not applied**, which is finding 2 in the
    /// module docs. This pins the number so that the day the wrap grows a seam
    /// for it, the target is already measured rather than re-derived.
    #[test]
    fn the_unreachable_gap_is_recorded_and_differs_from_the_vendors() {
        assert_eq!(Size::Small.gap(), px(6.0)); // gap-1.5
        assert_eq!(Size::Default.gap(), px(8.0)); // gap-2
        // `gpui_component`'s inner `h_flex().gap_x_1()` is a flat 4px, which is
        // neither of them.
        assert_ne!(Size::Small.gap(), px(4.0));
        assert_ne!(Size::Default.gap(), px(4.0));
    }

    /// The ids `select.tsx` carries: distinct, namespaced, and **nine** — one
    /// per styled box. The count is the measure of what the finding costs.
    #[test]
    fn the_recorded_ids_are_distinct_and_namespaced() {
        let all = [
            ids::TRIGGER,
            ids::VALUE,
            ids::ICON,
            ids::POPUP,
            ids::PANEL,
            ids::LIST,
            ids::ITEM,
            ids::ITEM_INDICATOR,
            ids::ITEM_TEXT,
        ];
        let mut sorted = all.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{all:?}");
        assert_eq!(all.len(), 9);
        assert!(all.iter().all(|id| id.starts_with("select-")));
    }
}
