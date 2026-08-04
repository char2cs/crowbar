//! The first Phase 2 component: `dropdown-menu` — the popup a menu opens into,
//! and the rows inside it.
//!
//! The native half of `web/src/components/ui/dropdown-menu.tsx`, which is a thin
//! set of Tailwind class lists over `@base-ui/react`'s `Menu`. Nothing in this
//! app overrides those classes from a stylesheet, so — unlike the two Phase 1
//! rows, whose class lists were half-dead under `file-explorer-tree.css` — what
//! the `cn(…)` calls say is what the popup renders. The compiled values are in
//! the constants below, each read out of the app's own Tailwind 4.3.0 rather
//! than from the class name.
//!
//! # What this component is, and what it deliberately is not
//!
//! It is **the popup and its rows**. It is not the *placement* of the popup.
//! `MenuPrimitive.Positioner` is Floating UI: it measures the trigger and the
//! viewport and writes `--anchor-width`, `--available-height` and
//! `--transform-origin` onto the popup at runtime. Two of those reach the class
//! list — `w-(--anchor-width)` and `max-h-(--available-height)` — so they are
//! *inputs* here rather than behaviour: [`DropdownMenu::anchor_width`] is a
//! parameter for the same reason [`MenuRowState`] is one, and the height cap
//! is recorded in the mapping table as positioner output this port has no way to
//! produce. Anchored positioning is its own surface and its own item.
//!
//! # The three things that are invisible to the oracle, said plainly
//!
//! `native/oracle/ANCHORS.md` §6 has no field for any of them, so the port
//! paints them for fidelity and the differ will never confirm or deny them:
//!
//! | Class | What it is | Here |
//! |---|---|---|
//! | `shadow-md` / `shadow-lg` | a two-layer drop shadow | [`popup`] paints gpui's `shadow_md()`, which carries **byte-identical** offsets, blurs and spreads to Tailwind's |
//! | `ring-1 ring-foreground/10` | a 1px **box-shadow**, not a border | a third `BoxShadow`, 0 blur and 1px spread — see [`RING_WIDTH`] |
//! | `tracking-widest` | `letter-spacing: 0.1em` | **nothing**; gpui has no letter-spacing at all, which is why the shortcut is rendered and not anchored |
//!
//! The ring matters more than it looks: it is the popup's visible *edge*, and a
//! reader who sees `border.w: 0` on both sides should not conclude the popup has
//! no outline. It has one, painted by a shadow, and neither extractor can see it.
//!
//! # Why the two declaration lists are empty
//!
//! [`CONTENT_SIZED`] and [`LINE_SIZED`] are both `[]`, which is a measurement
//! rather than an omission — see their own docs. Every anchor on this surface is
//! either a block child that stretches to the popup's content width, or a box
//! whose height is padding plus a line box. Neither v1.5 nor v1.6 applies to any
//! of them, and declaring one that does not apply is how a delta gets
//! manufactured (`ANCHORS.md` v1.6's badge trap, in a new shape).

use gpui::{
    AnyElement, BoxShadow, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _,
    div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::ContentLength;
use crate::theme::{Color, Theme, ui_sans_font};

/// The root anchor: `MenuPrimitive.Popup`, the box `bg-popover` paints. Every
/// other bound on this surface is reported relative to it
/// (`native/oracle/ANCHORS.md` §4).
pub const ID_POPUP: &str = "menu-popup";
/// `DropdownMenuLabel` — `MenuPrimitive.GroupLabel`.
pub const ID_LABEL: &str = "menu-label";
/// `DropdownMenuSeparator`.
pub const ID_SEPARATOR: &str = "menu-separator";
/// `DropdownMenuItem`, when a menu has exactly one.
///
/// A menu with several items names them at the call site — the React extractor
/// walks `[data-oracle-id]` **under the root anchor only**, so ids have to be
/// unique within one popup and not across the app, and `dropdown-menu.tsx`
/// cannot invent an index it does not have. [`DropdownMenu::fixture`] uses the
/// live review-thread menu's three names.
pub const ID_ITEM: &str = "menu-item";
/// `DropdownMenuCheckboxItem`.
pub const ID_CHECKBOX_ITEM: &str = "menu-checkbox-item";
/// The `<span>` inside a checkbox item that holds the tick.
pub const ID_CHECKBOX_INDICATOR: &str = "menu-checkbox-indicator";
/// `DropdownMenuRadioItem`.
pub const ID_RADIO_ITEM: &str = "menu-radio-item";
/// The `<span>` inside a radio item that holds the tick.
pub const ID_RADIO_INDICATOR: &str = "menu-radio-indicator";
/// `DropdownMenuSubTrigger` — the row that opens a submenu.
pub const ID_SUB_TRIGGER: &str = "menu-sub-trigger";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and that is a fact about the layout rather than a gap in the
/// declaration. v1.5 exists because gpui `ceil()`s a *text run's* max-content
/// width where `WebKit` keeps the fraction, so it applies exactly to a box whose
/// used width **is** that max-content width. Nothing here is:
///
/// * the popup's width is `max(--anchor-width, min-width)` — two definite
///   lengths, and the resolution is CSS's own clamp;
/// * every row is a **block-level** child of the popup, so its used width is the
///   popup's content width whatever its text says;
/// * the separator is a block-level 1px rule with negative inline margins;
/// * the tick indicator is `position: absolute` around a 16px icon and paints no
///   text at all.
///
/// Written down as data, and as an empty list rather than not at all, because
/// the same list has to be absent on the React side too — a
/// `data-oracle-content-sized` on one side only is a `FieldPresence` delta that
/// forgives nothing.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None**, and this is the badge trap in a second shape. Two of the anchors
/// paint text — the rows and the label — and both read like the case v1.6 was
/// written for. Neither is: `py-1` puts 4px above and below the line box, so a
/// row's border box is `4 + 20 + 4 = 28` against a 20px line, and the label's is
/// `4 + 16 + 4 = 24` against 16. Declaring either would compare 28 against 20 and
/// invent an 8px delta on an anchor both engines agree on.
///
/// The test below pins that as an arithmetic identity rather than as an opinion,
/// and [`crate::primitives::dropdown_menu`]'s consumer pins it against a live
/// layout.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root. Every `p-1`,
/// `gap-1.5`, `pr-8` below is a multiple of it, and it is spelled once so a
/// theme that moved it would move them together.
const SPACING: f32 = 4.0;

/// `p-1` on the popup — `calc(--spacing * 1)`.
pub const POPUP_PADDING: Pixels = px(SPACING);

/// `min-w-32` — `calc(--spacing * 32)`, the popup's floor when the trigger is
/// narrower than the menu.
pub const POPUP_MIN_WIDTH: Pixels = px(SPACING * 32.0);

/// `min-w-40` — the floor the live review-thread menu raises it to from its own
/// `className`. tailwind-merge keeps the later of two `min-w` utilities, so this
/// **replaces** [`POPUP_MIN_WIDTH`] rather than stacking with it.
pub const POPUP_MIN_WIDTH_LG: Pixels = px(SPACING * 40.0);

/// `ring-1` — a **box-shadow**, 1px of spread with no blur, not a border.
///
/// Worth being exact about, because `border.w` is compared *exactly* by
/// `ANCHORS.md` v1.1 and the popup's is **zero on both sides**. Tailwind 4
/// compiles `ring-1` to
/// `--tw-ring-shadow: 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color)`,
/// which is a shadow layer. A port that reached for `.border_1()` would report a
/// 1px border against the reference's 0 and be told so on every cell of the
/// matrix — while *also* being wrong about the paint, since a border takes space
/// in the box model and a ring does not.
pub const RING_WIDTH: Pixels = px(1.0);

/// `ring-foreground/10`.
const RING_ALPHA: f32 = 10.0;

/// `px-1.5` — the inline padding of every row.
pub const ROW_PADDING_X: Pixels = px(SPACING * 1.5);

/// `py-1` — the block padding of every row and of the label.
pub const ROW_PADDING_Y: Pixels = px(SPACING);

/// `gap-1.5` — between a row's icon, its label and its trailing content.
pub const ROW_GAP: Pixels = px(SPACING * 1.5);

/// `data-inset:pl-7` — the leading padding a row takes when it has to line up
/// with rows that carry a tick rather than an icon.
///
/// It **replaces** `px-1.5`'s left half rather than adding to it: `pl-7` and
/// `px-1.5` are different tailwind-merge groups, and the more specific one wins
/// the cascade at equal specificity because `data-inset:pl-7` is written later.
pub const ROW_INSET_PADDING_LEFT: Pixels = px(SPACING * 7.0);

/// `pr-8` on a checkbox or radio row — the gutter the tick sits in.
pub const TICK_ROW_PADDING_RIGHT: Pixels = px(SPACING * 8.0);

/// `right-2` on the tick's `<span>`.
pub const TICK_RIGHT: Pixels = px(SPACING * 2.0);

/// `[&_svg:not([class*='size-'])]:size-4`, and the `size-4` every call site
/// writes on its own icons: 16px either way.
pub const ICON_SIZE: Pixels = px(SPACING * 4.0);

/// `h-px` on the separator.
pub const SEPARATOR_HEIGHT: Pixels = px(1.0);

/// `my-1` on the separator.
pub const SEPARATOR_MARGIN_Y: Pixels = px(SPACING);

/// `-mx-1` on the separator — it bleeds **out** through the popup's own `p-1`,
/// so the rule runs edge to edge rather than stopping at the rows.
///
/// Negative, and taffy honours it in block layout exactly as CSS does. The
/// arithmetic to check is that the two cancel: `-mx-1` is the negation of the
/// popup's `p-1`, so the separator's border box is the popup's *padding* box.
pub const SEPARATOR_MARGIN_X: Pixels = px(-SPACING);

/// `text-sm`'s line height — Tailwind's stock `--text-sm--line-height`,
/// `calc(1.25 / 0.875)`, so 14px text sits on a 20px line box.
///
/// Spelled as the division because that is how the stylesheet writes it, and
/// because the pair it comes from is the pair the row's font size comes from.
const ROW_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `text-xs`'s line height — `--text-xs--line-height`, `calc(1 / 0.75)`, so 12px
/// text sits on a 16px line box.
const LABEL_LINE_HEIGHT: f32 = 1.0 / 0.75;

/// `data-[variant=destructive]:focus:bg-destructive/10`.
const DESTRUCTIVE_FOCUS_TINT_LIGHT: f32 = 10.0;

/// `dark:data-[variant=destructive]:focus:bg-destructive/20`.
const DESTRUCTIVE_FOCUS_TINT_DARK: f32 = 20.0;

/// `data-disabled:opacity-50`.
///
/// A real `opacity` on the row, not a colour mixed down: the React class is
/// `opacity`, it applies to the icon and the tick as well as the text, and the
/// contract has no opacity field either way. So this is painted for fidelity and
/// is **invisible to the differ** — a disabled row and an enabled one report the
/// same `fg`, which is what the DOM's own `getComputedStyle().color` reports too.
pub const DISABLED_OPACITY: f32 = 0.5;

/// Whether a `dark:` Tailwind variant is in force.
///
/// The React app switches on a `dark` class on the document element, which is
/// the same switch that selects the token table — so "the dark table is in
/// force" and "the `dark:` variants apply" are one question. A local copy of
/// `git_status_row`'s, deliberately: the two components are ported
/// independently and a shared helper would make one surface's diff reach into
/// the other's file.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// Which of the two `DropdownMenuItem` variants a row is.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum RowVariant {
    /// `variant="default"` — the prop's own default.
    #[default]
    Default,
    /// `variant="destructive"`: red text at rest, and a tinted red focus
    /// background instead of `bg-accent`.
    Destructive,
}

/// Which of `dropdown-menu.tsx`'s four row primitives a row is.
///
/// They are one shape with three differences, and keeping them one type is what
/// makes those differences legible: a checkbox and a radio row are the *same*
/// class list under two names, and a submenu trigger is an item with a chevron
/// and no disabled state.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum RowKind {
    /// `DropdownMenuItem` — `px-1.5`, no tick gutter.
    #[default]
    Item,
    /// `DropdownMenuCheckboxItem` — `pr-8 pl-1.5`, tick gutter on the right.
    CheckboxItem,
    /// `DropdownMenuRadioItem` — the identical class list under another name.
    /// Kept apart from [`RowKind::CheckboxItem`] because the two carry
    /// **different anchor ids**, which is the only thing the differ can see.
    RadioItem,
    /// `DropdownMenuSubTrigger` — `px-1.5` plus a trailing `ml-auto` chevron.
    SubTrigger,
}

impl RowKind {
    /// Whether the row reserves the `pr-8` gutter and paints a tick `<span>`.
    #[must_use]
    pub const fn has_tick(self) -> bool {
        matches!(self, Self::CheckboxItem | Self::RadioItem)
    }

    /// The anchor the tick's `<span>` carries, if there is one.
    #[must_use]
    pub const fn tick_id(self) -> Option<&'static str> {
        match self {
            Self::CheckboxItem => Some(ID_CHECKBOX_INDICATOR),
            Self::RadioItem => Some(ID_RADIO_INDICATOR),
            Self::Item | Self::SubTrigger => None,
        }
    }

    /// The id `dropdown-menu.tsx` puts on this primitive when the call site does
    /// not name it.
    #[must_use]
    pub const fn default_id(self) -> &'static str {
        match self {
            Self::Item => ID_ITEM,
            Self::CheckboxItem => ID_CHECKBOX_ITEM,
            Self::RadioItem => ID_RADIO_ITEM,
            Self::SubTrigger => ID_SUB_TRIGGER,
        }
    }
}

/// The visual state a row is painted in.
///
/// **A parameter, not a gpui `.hover(…)` refinement** — `native/oracle/ANCHORS.md`
/// §6 makes runtime interaction state invisible to the extractor, so a row that
/// expressed its focus that way would report its resting paint in every cell of
/// the state axis and converge while proving nothing.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct MenuRowState {
    /// `:focus` — the row `base-ui` has highlighted.
    ///
    /// **There is no `hover:` rule anywhere in `dropdown-menu.tsx`.** The only
    /// interaction style a row carries is `focus:`, and `base-ui`'s menu reaches
    /// it by moving DOM focus to the row under the pointer. So hovering and
    /// arrowing to a row paint the same thing, and the matrix's `hover` and
    /// `focus` flags both land here — which is a *reading* of `base-ui`, not
    /// something the class list alone says, and is recorded as such.
    pub focused: bool,
    /// `data-checked` — the tick is mounted.
    ///
    /// Only [`RowKind::has_tick`] rows can carry it. `base-ui`'s
    /// `CheckboxItemIndicator` defaults to `keepMounted: false`, so an unchecked
    /// row renders the gutter `<span>` and nothing inside it — the anchor stays
    /// present and its box collapses, which is what makes `selected` a
    /// comparable cell here rather than a vacuous one.
    pub checked: bool,
    /// `data-disabled` — `opacity-50` and no pointer events.
    ///
    /// Not a §8.3 flag: the matrix vocabulary is fixed at six words and this is
    /// not one of them. It is a row parameter, beside `--git-status` on the file
    /// tree row.
    pub disabled: bool,
}

impl MenuRowState {
    /// The resting row.
    #[must_use]
    pub const fn resting() -> Self {
        Self {
            focused: false,
            checked: false,
            disabled: false,
        }
    }
}

/// The background a row paints, or `None` for "paints nothing".
///
/// Returned rather than applied so the element can carry no background at all:
/// the extractor reports an unpainted box and a `transparent` one identically,
/// and not painting is the truer statement.
#[must_use]
pub fn row_background(theme: &Theme, variant: RowVariant, state: MenuRowState) -> Option<Color> {
    if !state.focused {
        return None;
    }
    match variant {
        // `focus:bg-accent`.
        RowVariant::Default => Some(theme.accent),
        // `data-[variant=destructive]:focus:bg-destructive/10`, and `/20` under
        // the `dark:` variant.
        RowVariant::Destructive => {
            let tint = if is_dark(theme) {
                DESTRUCTIVE_FOCUS_TINT_DARK
            } else {
                DESTRUCTIVE_FOCUS_TINT_LIGHT
            };
            Some(theme.destructive.mix(tint, Color::TRANSPARENT))
        }
    }
}

/// The colour a row's text takes.
///
/// Three rules, and the order they resolve in is the whole of it:
///
/// * `data-[variant=destructive]:text-destructive` is unconditional, so a
///   destructive row is red focused **and** at rest, and `focus:text-…` never
///   reaches it — `data-[variant=destructive]:focus:text-destructive` restates
///   the same colour precisely so that it does not.
/// * `focus:text-accent-foreground` on a default row.
/// * otherwise nothing at all, and the row inherits the popup's
///   `text-popover-foreground`. Spelled explicitly here rather than left to
///   inherit, so the function is total and the value is testable without a
///   layout.
#[must_use]
pub const fn row_foreground(theme: &Theme, variant: RowVariant, state: MenuRowState) -> Color {
    match variant {
        RowVariant::Destructive => theme.destructive,
        RowVariant::Default if state.focused => theme.accent_foreground,
        RowVariant::Default => theme.popover_foreground,
    }
}

/// The label one of the §8.3 matrix's three content lengths stands for, as the
/// **live** review-thread menu spells them.
///
/// The enum is [`ContentLength`] — the matrix's own vocabulary, shared
/// with both Phase 1 surfaces — rather than a third spelling of `short | normal
/// | overflow`. Only the strings are this surface's.
///
/// `short` and `normal` are that menu's own two labels. **`overflow` is not**:
/// no menu anywhere in the app carries a label long enough to exceed a 160px
/// popup, so the third cell has **no reference to compare against** and the
/// string here is this port's own invention. It is kept anyway, because the
/// native side has to be able to render the cell the day a menu does — and it is
/// named as unreachable rather than quietly rendered, which is the call Phase 1
/// made about `git-row-dir`.
///
/// The long string is one unbroken token on purpose. A label with spaces
/// **wraps** rather than clipping — a menu row carries no `truncate` and no
/// `whitespace-nowrap` — and a wrapped run is outside what the contract can
/// compare: the DOM extractor sums a `Range`'s client rects while the gpui side
/// shapes the whole string on one line, so every wrap costs the two sides about
/// a space's advance against a ±1.0px tolerance. A single token cannot wrap, so
/// it overflows the row and is clipped by the popup's `overflow-x: hidden`,
/// which both extractors *do* agree on.
#[must_use]
pub const fn label(content: ContentLength) -> &'static str {
    match content {
        ContentLength::Short => "Edit",
        ContentLength::Normal => "Copy as Markdown",
        ContentLength::Overflow => {
            "CopyEveryOutstandingReviewCommentAsMarkdownIncludingResolvedOnes"
        }
    }
}

/// One row of a menu popup.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct MenuRow {
    /// The anchor this row carries. Authored rather than derived: a menu with
    /// three `DropdownMenuItem`s needs three ids and the React primitive has no
    /// index to build them from, so the call site names them on both sides.
    pub id: SharedString,
    /// The row's own text — its only text node in the DOM, which is what the
    /// React extractor's `oracleOwnTextNodes` reads.
    pub label: SharedString,
    /// Which primitive it is.
    pub kind: RowKind,
    /// `variant`.
    pub variant: RowVariant,
    /// `inset` — the `pl-7` gutter.
    pub inset: bool,
    /// Whether a leading icon is rendered. The call sites all pass one; the prop
    /// is optional and a row without one closes up by its `gap-1.5`.
    pub leading_icon: bool,
    /// `DropdownMenuShortcut`, rendered `ml-auto` after the label.
    ///
    /// **Rendered and not anchored.** `tracking-widest` is
    /// `letter-spacing: 0.1em` and gpui has no letter-spacing at all, so the
    /// shaped advance would be short by a tenth of an em per character —
    /// several pixels against a ±1.0px `text_width` tolerance. Anchoring it
    /// would report a delta that says nothing about this port. Leaving it out
    /// would be worse: it is a flex sibling that takes room from the label.
    pub shortcut: Option<SharedString>,
    /// The visual state.
    pub state: MenuRowState,
}

impl MenuRow {
    /// A plain `DropdownMenuItem` with a leading icon — the shape all five live
    /// call sites use.
    #[must_use]
    pub fn item(id: impl Into<SharedString>, label: impl Into<SharedString>) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            kind: RowKind::Item,
            variant: RowVariant::Default,
            inset: false,
            leading_icon: true,
            shortcut: None,
            state: MenuRowState::resting(),
        }
    }

    /// The row's leading padding: `pl-7` when inset, `px-1.5`'s left otherwise.
    #[must_use]
    pub const fn padding_left(&self) -> Pixels {
        if self.inset {
            ROW_INSET_PADDING_LEFT
        } else {
            ROW_PADDING_X
        }
    }

    /// The row's trailing padding: `pr-8` where a tick has to fit, `px-1.5`'s
    /// right otherwise.
    #[must_use]
    pub const fn padding_right(&self) -> Pixels {
        if self.kind.has_tick() {
            TICK_ROW_PADDING_RIGHT
        } else {
            ROW_PADDING_X
        }
    }
}

/// One entry in a popup, in DOM order.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum MenuEntry {
    /// `DropdownMenuLabel` — a group heading.
    Label {
        /// The anchor it carries.
        id: SharedString,
        /// Its text.
        text: SharedString,
        /// `inset`.
        inset: bool,
    },
    /// Any of the four row primitives.
    Row(MenuRow),
    /// `DropdownMenuSeparator`.
    Separator {
        /// The anchor it carries.
        id: SharedString,
    },
}

/// A dropdown menu popup, open, with its entries.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct DropdownMenu {
    /// `--anchor-width`: the width the positioner measured off the trigger.
    ///
    /// A parameter because it is a runtime measurement of an element this
    /// component does not contain. The popup declares **both** this and
    /// [`DropdownMenu::min_width`], exactly as the class list does, and lets the
    /// layout clamp — reproducing the two declarations rather than their
    /// resolution, so that a taffy that clamped differently from `WebKit` would
    /// show up instead of being pre-empted here.
    pub anchor_width: Pixels,
    /// `min-w-32`, or whatever a call site's `className` raises it to.
    pub min_width: Pixels,
    /// The entries, in DOM order.
    pub entries: Vec<MenuEntry>,
}

impl DropdownMenu {
    /// The live comment menu from
    /// `web/src/features/git/components/review-thread-item.tsx`, which is the
    /// **only** `dropdown-menu` in the app that is neither inside the Plate
    /// editor nor hidden above a 720px viewport — so it is the one a parity run
    /// can drive at the matrix's widths.
    ///
    /// Its shape is why it was chosen rather than because it was convenient: two
    /// default rows, a separator and a `destructive` row is one of everything the
    /// state axis can move.
    ///
    /// `content` selects the **second** row's label, which is the one whose
    /// width the popup's 160px floor is closest to.
    #[must_use]
    pub fn fixture(content: ContentLength) -> Self {
        Self {
            // `size-6` on the trigger — the `DotsThree` button.
            anchor_width: px(24.0),
            // `className="min-w-40"`.
            min_width: POPUP_MIN_WIDTH_LG,
            entries: vec![
                MenuEntry::Row(MenuRow::item(
                    "menu-item-edit",
                    label(ContentLength::Short),
                )),
                MenuEntry::Row(MenuRow::item("menu-item-copy", label(content))),
                MenuEntry::Separator {
                    id: SharedString::new_static(ID_SEPARATOR),
                },
                MenuEntry::Row(MenuRow {
                    variant: RowVariant::Destructive,
                    ..MenuRow::item("menu-item-delete", "Delete")
                }),
            ],
        }
    }

    /// Renders the popup, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let children: Vec<AnyElement> = self
            .entries
            .iter()
            .map(|entry| Self::entry(theme, anchors, entry))
            .collect();
        anchors.root(ID_POPUP.into(), self.popup(theme).children(children))
    }

    /// `MenuPrimitive.Popup`'s own box.
    ///
    /// The font family is named explicitly rather than left to inherit: gpui can
    /// only report the *declared* family, and an inherited `.SystemUIFont` is a
    /// string the DOM will never produce. The reference's is `CalSansUI`, which
    /// the popup inherits from `html { @apply font-sans }` — the popup is
    /// **portalled to `document.body`**, so it inherits from the document root
    /// and not from whatever opened it. [`ui_sans_font`] also carries the
    /// fallback chain that lets a glyph missing from it (a menu accelerator's
    /// modifier symbol, say) resolve the way `WebKit` resolves it (P3.24).
    fn popup(&self, theme: &Theme) -> Div {
        let popup = div()
            .w(self.anchor_width)
            .min_w(self.min_width)
            .p(POPUP_PADDING)
            .rounded(theme.radius_lg.value())
            .bg(theme.popover)
            // `overflow-x-hidden overflow-y-auto`, and the horizontal half is
            // load-bearing: clipping at the popup's padding box is what makes a
            // too-wide row report `clipped` at all, on both sides.
            //
            // The vertical half is `hidden` here rather than `scroll`, and the
            // difference is nothing:
            //
            // * gpui's `overflow_y_scroll` lives on `StatefulInteractiveElement`,
            //   so reaching it means giving the popup an element id and a scroll
            //   handle — runtime interaction state the extractor reads *around*
            //   (`ANCHORS.md` §6), which is exactly what this surface is built
            //   not to depend on;
            // * taffy zeroes the automatic minimum size for **any** non-visible
            //   overflow, `Hidden` and `Scroll` alike, so the rule that lets a
            //   row overflow is the same one either way;
            // * the only other difference is the reserved scrollbar gutter, and
            //   macOS overlay scrollbars reserve none in `WebKit` either.
            //
            // And nothing scrolls in this port regardless: `max-h-(--available-height)`
            // is positioner output, which the mapping table records as absent.
            .overflow_hidden()
            .font(ui_sans_font(theme))
            .font_weight(FontWeight::NORMAL)
            .text_size(theme.ui_text_base.value())
            .line_height(relative(ROW_LINE_HEIGHT))
            .text_color(theme.popover_foreground);
        ring(
            popup.shadow_md(),
            theme.foreground.mix(RING_ALPHA, Color::TRANSPARENT),
        )
    }

    fn entry(theme: &Theme, anchors: &dyn AnchorSink, entry: &MenuEntry) -> AnyElement {
        match entry {
            MenuEntry::Label { id, text, inset } => anchors.boxed_text(
                AnchorId::new(id.clone()),
                Self::label(theme, *inset),
                text.clone(),
            ),
            MenuEntry::Row(row) => Self::row(theme, anchors, row),
            MenuEntry::Separator { id } => {
                anchors.boxed(AnchorId::new(id.clone()), Self::separator(theme))
            }
        }
    }

    /// `px-1.5 py-1 text-xs font-medium text-muted-foreground data-inset:pl-7`.
    ///
    /// `text-xs` is Tailwind's stock `--text-xs`, `0.75rem`. `--ui-text-sm` is
    /// the same 0.75rem and is the token this design system carries; there is no
    /// token for Tailwind's own scale, and inventing one would put a value in
    /// the system the system does not have. The same trade the file tree row
    /// makes for `text-sm`.
    fn label(theme: &Theme, inset: bool) -> Div {
        let leading = if inset {
            ROW_INSET_PADDING_LEFT
        } else {
            ROW_PADDING_X
        };
        div()
            .pl(leading)
            .pr(ROW_PADDING_X)
            .py(ROW_PADDING_Y)
            .text_size(theme.ui_text_sm.value())
            .line_height(relative(LABEL_LINE_HEIGHT))
            .font_weight(FontWeight::MEDIUM)
            .text_color(theme.muted_foreground)
    }

    /// `-mx-1 my-1 h-px bg-border`.
    fn separator(theme: &Theme) -> Div {
        div()
            .h(SEPARATOR_HEIGHT)
            .mx(SEPARATOR_MARGIN_X)
            .my(SEPARATOR_MARGIN_Y)
            .bg(theme.border)
    }

    /// One row, whichever of the four primitives it is.
    ///
    /// The children are placed in **DOM order** — icon, label, trailing — which
    /// is why the run comes from [`AnchorSink::text_half`] rather than from
    /// `boxed_text`: the chevron and the shortcut sit *after* the label, and a
    /// run appended last would reverse them.
    fn row(theme: &Theme, anchors: &dyn AnchorSink, row: &MenuRow) -> AnyElement {
        let id = AnchorId::new(row.id.clone());
        let foreground = row_foreground(theme, row.variant, row.state);
        let mut element = Self::row_box(theme, row);

        if row.leading_icon {
            element = element.child(icon(foreground));
        }
        element = element.child(anchors.text_half(&id, row.label.clone()));
        if let Some(shortcut) = &row.shortcut {
            element = element.child(Self::shortcut(theme, row, shortcut.clone()));
        }
        if row.kind == RowKind::SubTrigger {
            // `<ChevronRightIcon className="ml-auto" />` — no `size-` class, so
            // the item's own `[&_svg:not([class*='size-'])]:size-4` sizes it.
            element = element.child(icon(foreground).ml_auto());
        }
        if let Some(tick) = row.kind.tick_id() {
            element = element.child(anchors.boxed(tick.into(), Self::tick(foreground, row.state)));
        }

        anchors.boxed(id, element)
    }

    /// The row's own box: `relative flex items-center gap-1.5 rounded-md
    /// px-1.5 py-1 text-sm`, plus whichever of the paddings and paints its kind,
    /// variant and state select.
    fn row_box(theme: &Theme, row: &MenuRow) -> Div {
        let element = div()
            .relative()
            .flex()
            .items_center()
            .gap(ROW_GAP)
            .rounded(theme.radius_md.value())
            .pl(row.padding_left())
            .pr(row.padding_right())
            .py(ROW_PADDING_Y)
            .text_size(theme.ui_text_base.value())
            .line_height(relative(ROW_LINE_HEIGHT))
            .text_color(row_foreground(theme, row.variant, row.state));
        let element = match row_background(theme, row.variant, row.state) {
            Some(background) => element.bg(background),
            None => element,
        };
        if row.state.disabled {
            element.opacity(DISABLED_OPACITY)
        } else {
            element
        }
    }

    /// `<span class="pointer-events-none absolute right-2 flex items-center
    /// justify-center">`, holding a `CheckIcon` **only when checked**.
    ///
    /// `base-ui`'s `CheckboxItemIndicator` is `keepMounted: false`, so the tick
    /// itself unmounts and the `<span>` stays. That is what the `selected` cell
    /// of the matrix measures: the same anchor, 16px wide checked and zero wide
    /// unchecked.
    fn tick(foreground: Color, state: MenuRowState) -> Div {
        let element = div()
            .absolute()
            .right(TICK_RIGHT)
            .flex()
            .items_center()
            .justify_center()
            .text_color(foreground);
        if state.checked {
            element.child(icon(foreground))
        } else {
            element
        }
    }

    /// `<span class="ml-auto text-xs tracking-widest text-muted-foreground
    /// group-focus/dropdown-menu-item:text-accent-foreground">`.
    ///
    /// Unanchored — see [`MenuRow::shortcut`]. The focus colour is the row's own,
    /// because `**:text-accent-foreground` reaches every descendant of a focused
    /// non-destructive row and the group variant here says the same thing twice.
    fn shortcut(theme: &Theme, row: &MenuRow, text: SharedString) -> Div {
        let color = if row.state.focused && row.variant == RowVariant::Default {
            theme.accent_foreground
        } else {
            theme.muted_foreground
        };
        div()
            .ml_auto()
            .flex_shrink_0()
            .text_size(theme.ui_text_sm.value())
            .line_height(relative(LABEL_LINE_HEIGHT))
            .text_color(color)
            .child(text)
    }
}

/// A 16px icon box.
///
/// **Empty**, for the reason [`super::GitStatusRow`]'s and
/// [`super::FileTreeRow`]'s are: the React icons are SVGs a call site chooses,
/// there is no native equivalent, and drawing a substitute would put a shape on
/// screen for the oracle to converge on. The box is what the contract measures,
/// and the colour is set so that a future glyph inherits the right one.
fn icon(foreground: Color) -> Div {
    div()
        .flex()
        .items_center()
        .justify_center()
        .flex_shrink_0()
        .w(ICON_SIZE)
        .h(ICON_SIZE)
        .text_color(foreground)
}

/// Adds `ring-1`'s shadow layer in front of whatever shadows `element` already
/// carries.
///
/// Tailwind's composite is `box-shadow: <ring>, <shadow>` — the ring first — and
/// gpui's `shadow_md()` sets the whole list, so the ring has to be inserted
/// rather than declared. `Styled::style` is the public seam for exactly this.
///
/// The colour is a token; the two `shadow_md` layers' is not, and cannot be:
/// Tailwind's default `--tw-shadow-color` is the literal `rgb(0 0 0 / 0.1)`,
/// which no token in `theme.css` carries and which
/// `scripts/check-invariants.sh` rule 4 will not let this crate mint. gpui's own
/// `shadow_md()` carries byte-identical offsets, blurs, spreads **and** that
/// colour, inside gpui, so the port reaches for it rather than for a literal.
fn ring(mut element: Div, color: Color) -> Div {
    let layer = BoxShadow::new(px(0.0), px(0.0), color.value()).spread_radius(RING_WIDTH);
    element
        .style()
        .box_shadow
        .get_or_insert_with(Vec::new)
        .insert(0, layer);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, DISABLED_OPACITY, DropdownMenu, ICON_SIZE, ID_CHECKBOX_ITEM, ID_ITEM,
        ID_LABEL, ID_POPUP, ID_RADIO_ITEM, ID_SEPARATOR, ID_SUB_TRIGGER, LABEL_LINE_HEIGHT,
        LINE_SIZED, MenuEntry, MenuRow, MenuRowState, POPUP_MIN_WIDTH, POPUP_MIN_WIDTH_LG,
        POPUP_PADDING, RING_WIDTH, ROW_GAP, ROW_INSET_PADDING_LEFT, ROW_LINE_HEIGHT, ROW_PADDING_X,
        ROW_PADDING_Y, RowKind, RowVariant, SEPARATOR_HEIGHT, SEPARATOR_MARGIN_X,
        SEPARATOR_MARGIN_Y, TICK_RIGHT, TICK_ROW_PADDING_RIGHT, label, row_background,
        row_foreground,
    };
    use crate::surfaces::rows::ContentLength;
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind 4.3.0 compiles the class to, at `--spacing: 0.25rem` and a 16px
    /// root. Spelled as the arithmetic rather than as more literals, so a change
    /// to the step cannot pass by leaving the literals alone.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(POPUP_PADDING, px(STEP)); // p-1
        assert_eq!(POPUP_MIN_WIDTH, px(STEP * 32.0)); // min-w-32
        assert_eq!(POPUP_MIN_WIDTH_LG, px(STEP * 40.0)); // min-w-40
        assert_eq!(ROW_PADDING_X, px(STEP * 1.5)); // px-1.5
        assert_eq!(ROW_PADDING_Y, px(STEP)); // py-1
        assert_eq!(ROW_GAP, px(STEP * 1.5)); // gap-1.5
        assert_eq!(ROW_INSET_PADDING_LEFT, px(STEP * 7.0)); // pl-7
        assert_eq!(TICK_ROW_PADDING_RIGHT, px(STEP * 8.0)); // pr-8
        assert_eq!(TICK_RIGHT, px(STEP * 2.0)); // right-2
        assert_eq!(ICON_SIZE, px(STEP * 4.0)); // size-4
        assert_eq!(SEPARATOR_HEIGHT, px(1.0)); // h-px
        assert_eq!(SEPARATOR_MARGIN_Y, px(STEP)); // my-1

        // The one that has to be negative, and the identity that makes it
        // meaningful: `-mx-1` exactly undoes the popup's `p-1`, so the rule's
        // border box is the popup's padding box.
        assert_eq!(SEPARATOR_MARGIN_X, px(-STEP));
        assert_eq!(SEPARATOR_MARGIN_X + POPUP_PADDING, px(0.0));
    }

    /// The popup's radius is **10px**, because this project redefines
    /// `--radius-lg: var(--radius)` over a `0.625rem` base — not Tailwind's
    /// stock 8. The rows' `rounded-md` is the same 8px the git status row's
    /// button uses.
    #[test]
    fn the_radii_are_this_projects_and_not_tailwinds() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_lg.value(), px(10.0));
            assert_eq!(theme.radius_md.value(), px(8.0));
        }
    }

    /// The two line heights, spelled as the divisions the stylesheet writes:
    /// 14px text on a 20px line box for a row, 12px on 16px for a label.
    #[test]
    fn the_line_heights_are_tailwinds_stock_pairs() {
        assert!(
            (ROW_LINE_HEIGHT * 14.0 - 20.0).abs() < 0.001,
            "{ROW_LINE_HEIGHT}"
        );
        assert!(
            (LABEL_LINE_HEIGHT * 12.0 - 16.0).abs() < 0.001,
            "{LABEL_LINE_HEIGHT}"
        );
        // And they are different numbers, which is the reason a label is not a
        // row with a smaller font.
        assert!((ROW_LINE_HEIGHT - LABEL_LINE_HEIGHT).abs() > 0.05);

        for theme in [Theme::LIGHT, Theme::DARK] {
            // `text-sm` = 0.875rem, `text-xs` = 0.75rem, reached through the two
            // ui tokens that carry the same numbers.
            assert_eq!(theme.ui_text_base.value(), gpui::rems(0.875));
            assert_eq!(theme.ui_text_sm.value(), gpui::rems(0.75));
        }
    }

    /// `ring-1` is a **shadow**, so the popup's border stays zero — the field
    /// `ANCHORS.md` v1.1 compares *exactly*. This pins the width the ring is
    /// painted at, and the fact that it is not a border, as one assertion.
    #[test]
    fn the_ring_is_one_pixel_of_shadow_spread_and_not_a_border() {
        assert_eq!(RING_WIDTH, px(1.0));
    }

    /// Focus is the only thing that paints a row background, and the two
    /// variants paint **different** ones — which is what makes the state axis
    /// real on this surface rather than one flag with one paint.
    #[test]
    fn only_focus_paints_a_row_background_and_the_variants_differ() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            for variant in [RowVariant::Default, RowVariant::Destructive] {
                assert_eq!(
                    row_background(&theme, variant, MenuRowState::resting()),
                    None
                );
                // Neither of the two non-§8.3 row parameters paints one either.
                for state in [
                    MenuRowState {
                        checked: true,
                        ..MenuRowState::resting()
                    },
                    MenuRowState {
                        disabled: true,
                        ..MenuRowState::resting()
                    },
                ] {
                    assert_eq!(row_background(&theme, variant, state), None);
                }
            }

            let focused = MenuRowState {
                focused: true,
                ..MenuRowState::resting()
            };
            assert_eq!(
                row_background(&theme, RowVariant::Default, focused),
                Some(theme.accent),
            );
            let destructive = row_background(&theme, RowVariant::Destructive, focused);
            assert_ne!(destructive, Some(theme.accent));
            assert_ne!(destructive, None);
        }
    }

    /// The destructive focus tint is `/10` in light and `/20` in dark — the
    /// `dark:` variant doubles it, and the two are genuinely different colours.
    #[test]
    fn the_destructive_focus_tint_doubles_under_the_dark_variant() {
        let focused = MenuRowState {
            focused: true,
            ..MenuRowState::resting()
        };

        assert_eq!(
            row_background(&Theme::LIGHT, RowVariant::Destructive, focused),
            Some(Theme::LIGHT.destructive.mix(10.0, Color::TRANSPARENT)),
        );
        assert_eq!(
            row_background(&Theme::DARK, RowVariant::Destructive, focused),
            Some(Theme::DARK.destructive.mix(20.0, Color::TRANSPARENT)),
        );
    }

    /// **A destructive row is red focused and at rest**, which is the arm a
    /// "focus changes the colour" shortcut gets wrong: the unconditional
    /// `data-[variant=destructive]:text-destructive` beats
    /// `focus:text-accent-foreground`, and the class list restates the same red
    /// under `:focus` precisely so that it does.
    #[test]
    fn the_destructive_variant_keeps_its_colour_through_focus() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let focused = MenuRowState {
                focused: true,
                ..MenuRowState::resting()
            };

            assert_eq!(
                row_foreground(&theme, RowVariant::Destructive, MenuRowState::resting()),
                theme.destructive,
            );
            assert_eq!(
                row_foreground(&theme, RowVariant::Destructive, focused),
                theme.destructive,
            );

            // Where a default row does move, and starts on the popup's own
            // inherited colour rather than on `--foreground`.
            assert_eq!(
                row_foreground(&theme, RowVariant::Default, MenuRowState::resting()),
                theme.popover_foreground,
            );
            assert_eq!(
                row_foreground(&theme, RowVariant::Default, focused),
                theme.accent_foreground,
            );
            assert_ne!(theme.accent_foreground, theme.destructive);
        }
    }

    /// The paddings follow the **kind** and the **inset prop**, not the content.
    #[test]
    fn the_paddings_follow_the_kind_and_the_inset_prop() {
        let plain = MenuRow::item(ID_ITEM, "Edit");
        assert_eq!(plain.padding_left(), ROW_PADDING_X);
        assert_eq!(plain.padding_right(), ROW_PADDING_X);

        let inset = MenuRow {
            inset: true,
            ..MenuRow::item(ID_ITEM, "Edit")
        };
        assert_eq!(inset.padding_left(), ROW_INSET_PADDING_LEFT);
        assert_eq!(inset.padding_right(), ROW_PADDING_X);

        for kind in [RowKind::CheckboxItem, RowKind::RadioItem] {
            let tick = MenuRow {
                kind,
                ..MenuRow::item(ID_CHECKBOX_ITEM, "Wrap lines")
            };
            assert_eq!(tick.padding_left(), ROW_PADDING_X);
            assert_eq!(tick.padding_right(), TICK_ROW_PADDING_RIGHT);
        }

        // A submenu trigger is `px-1.5` like a plain item: the chevron is
        // `ml-auto` inside the padding, not a gutter outside it.
        let sub = MenuRow {
            kind: RowKind::SubTrigger,
            ..MenuRow::item(ID_SUB_TRIGGER, "Copy as")
        };
        assert_eq!(sub.padding_right(), ROW_PADDING_X);
    }

    /// Only the two tick kinds reserve a gutter and carry a tick anchor, and the
    /// checkbox and the radio carry **different** ids off an identical class
    /// list — which is the only thing about them the differ can tell apart.
    #[test]
    fn the_tick_kinds_are_the_two_that_carry_a_tick_anchor() {
        assert!(RowKind::CheckboxItem.has_tick());
        assert!(RowKind::RadioItem.has_tick());
        assert!(!RowKind::Item.has_tick());
        assert!(!RowKind::SubTrigger.has_tick());

        assert_eq!(
            RowKind::CheckboxItem.tick_id(),
            Some("menu-checkbox-indicator")
        );
        assert_eq!(RowKind::RadioItem.tick_id(), Some("menu-radio-indicator"));
        assert_eq!(RowKind::Item.tick_id(), None);
        assert_eq!(RowKind::SubTrigger.tick_id(), None);
        assert_ne!(
            RowKind::CheckboxItem.tick_id(),
            RowKind::RadioItem.tick_id()
        );
    }

    /// The ids `dropdown-menu.tsx` writes when a call site names nothing, one
    /// per primitive and all distinct — a repeat would make two anchors one
    /// record and the differ would compare whichever arrived last.
    #[test]
    fn every_primitives_default_id_is_distinct() {
        let ids = [
            ID_POPUP,
            ID_LABEL,
            ID_SEPARATOR,
            ID_ITEM,
            ID_CHECKBOX_ITEM,
            ID_RADIO_ITEM,
            ID_SUB_TRIGGER,
            "menu-checkbox-indicator",
            "menu-radio-indicator",
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");

        for kind in [
            RowKind::Item,
            RowKind::CheckboxItem,
            RowKind::RadioItem,
            RowKind::SubTrigger,
        ] {
            assert!(ids.contains(&kind.default_id()), "{kind:?}");
        }
        assert!(ids.iter().all(|id| id.starts_with("menu-")));
    }

    /// **Both declaration lists are empty, and that is arithmetic rather than an
    /// opinion.** A row's box is `py-1` above and below a 20px line box, a
    /// label's is `py-1` around a 16px one; declaring either `line_sized` would
    /// compare 28 against 20, or 24 against 16, on anchors both engines agree on.
    #[test]
    fn nothing_on_this_surface_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);

        let row_box = f32::from(ROW_PADDING_Y) * 2.0 + ROW_LINE_HEIGHT * 14.0;
        let label_box = f32::from(ROW_PADDING_Y) * 2.0 + LABEL_LINE_HEIGHT * 12.0;
        assert!((row_box - 28.0).abs() < 0.001, "{row_box}");
        assert!((label_box - 24.0).abs() < 0.001, "{label_box}");
        assert!(row_box - ROW_LINE_HEIGHT * 14.0 > 0.5);
        assert!(label_box - LABEL_LINE_HEIGHT * 12.0 > 0.5);
    }

    /// The fixture is the live review-thread menu: two default rows, a
    /// separator, one destructive row — and `--content` moves the second label
    /// only, so the cells differ in the one string whose width the 160px floor
    /// is closest to.
    #[test]
    fn the_fixture_is_the_live_comment_menu() {
        let menu = DropdownMenu::fixture(ContentLength::Normal);

        assert_eq!(menu.anchor_width, px(24.0));
        assert_eq!(menu.min_width, POPUP_MIN_WIDTH_LG);
        assert_eq!(menu.entries.len(), 4);
        assert!(matches!(menu.entries[2], MenuEntry::Separator { .. }));

        let labels: Vec<&str> = menu
            .entries
            .iter()
            .filter_map(|entry| match entry {
                MenuEntry::Row(row) => Some(row.label.as_ref()),
                _ => None,
            })
            .collect();
        assert_eq!(labels, ["Edit", "Copy as Markdown", "Delete"]);

        let variants: Vec<RowVariant> = menu
            .entries
            .iter()
            .filter_map(|entry| match entry {
                MenuEntry::Row(row) => Some(row.variant),
                _ => None,
            })
            .collect();
        assert_eq!(
            variants,
            [
                RowVariant::Default,
                RowVariant::Default,
                RowVariant::Destructive
            ],
        );

        // Every row carries its own id, and the ids are the ones the call site
        // has to write on the React side too.
        let ids: Vec<&str> = menu
            .entries
            .iter()
            .map(|entry| match entry {
                MenuEntry::Row(row) => row.id.as_ref(),
                MenuEntry::Separator { id } | MenuEntry::Label { id, .. } => id.as_ref(),
            })
            .collect();
        assert_eq!(
            ids,
            [
                "menu-item-edit",
                "menu-item-copy",
                "menu-separator",
                "menu-item-delete"
            ],
        );
    }

    /// `--content` moves the second row's label and nothing else, and the
    /// `overflow` string is **one unbreakable token**: a label with spaces wraps
    /// rather than clipping, and a wrapped run is outside what the contract can
    /// compare.
    #[test]
    fn the_content_axis_moves_one_label_and_the_long_one_cannot_wrap() {
        for content in [
            ContentLength::Short,
            ContentLength::Normal,
            ContentLength::Overflow,
        ] {
            let menu = DropdownMenu::fixture(content);
            let MenuEntry::Row(second) = &menu.entries[1] else {
                unreachable!("the fixture's second entry is a row")
            };
            assert_eq!(second.label, label(content));

            let MenuEntry::Row(first) = &menu.entries[0] else {
                unreachable!("the fixture's first entry is a row")
            };
            assert_eq!(first.label, "Edit", "{content:?}");
        }

        let long = label(ContentLength::Overflow);
        assert!(long.len() > label(ContentLength::Normal).len() * 3);
        for breakable in [' ', '-', '/', '\u{2010}'] {
            assert!(
                !long.contains(breakable),
                "{long:?} has a break opportunity at {breakable:?}, so it would wrap",
            );
        }
    }

    /// The row defaults are the shape all five live call sites use: a plain
    /// item, default variant, not inset, with a leading icon and no shortcut.
    #[test]
    fn a_plain_item_is_the_shape_every_call_site_writes() {
        let row = MenuRow::item(ID_ITEM, "Edit");

        assert_eq!(row.kind, RowKind::Item);
        assert_eq!(row.variant, RowVariant::Default);
        assert!(!row.inset);
        assert!(row.leading_icon);
        assert_eq!(row.shortcut, None);
        assert_eq!(row.state, MenuRowState::resting());
        assert_eq!(RowKind::default(), RowKind::Item);
        assert_eq!(RowVariant::default(), RowVariant::Default);
        assert_eq!(ContentLength::default(), ContentLength::Normal);
    }

    /// `opacity-50`, and the fact that it is invisible to the differ: the row's
    /// declared colour is unchanged by it, which is what both extractors read.
    #[test]
    fn disabling_a_row_dims_it_without_moving_a_compared_field() {
        let theme = Theme::DARK;
        let disabled = MenuRowState {
            disabled: true,
            ..MenuRowState::resting()
        };

        assert!((DISABLED_OPACITY - 0.5).abs() < f32::EPSILON);
        assert_eq!(
            row_foreground(&theme, RowVariant::Default, disabled),
            row_foreground(&theme, RowVariant::Default, MenuRowState::resting()),
        );
        assert_eq!(
            row_background(&theme, RowVariant::Default, disabled),
            row_background(&theme, RowVariant::Default, MenuRowState::resting()),
        );
    }
}
