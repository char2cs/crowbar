//! The first Phase 3 component: `tabs` — a tab list, its tabs, the indicator
//! that moves under the active one, and the panel below.
//!
//! The native half of `web/src/components/ui/tabs.tsx`, which is a set of
//! Tailwind class lists over `@base-ui/react` 1.6.0's `Tabs`. Every value below
//! came out of the app's own `index.css` through its own Tailwind 4.3.0 — see
//! `native/mapping/tabs.md`.
//!
//! # The indicator is positioned by **measurement**, and that is the whole item
//!
//! `Tabs.Indicator` does not lay itself out. On every render base-ui measures
//! the **active tab** and the **list** — `getCssDimensions` (a
//! `getComputedStyle` read that falls back to `offsetWidth`/`offsetHeight`) plus
//! `getBoundingClientRect` for the delta — and writes six custom properties as
//! an **inline style** on the indicator span:
//!
//! ```text
//! --active-tab-left --active-tab-right --active-tab-top
//! --active-tab-bottom --active-tab-width --active-tab-height
//! ```
//!
//! The class list then consumes four of them:
//! `absolute bottom-0 left-0 h-(--active-tab-height) w-(--active-tab-width)
//! translate-x-(--active-tab-left) -translate-y-(--active-tab-bottom)`.
//!
//! Resolved, with a list that has no border and does not scroll, that
//! arithmetic comes out **exactly the active tab's border box** — measured, not
//! assumed: the live sidebar tab bar reports the tab at `2,2,90,32` and the
//! indicator at `2,2,90,32`, with `--active-tab-left: 2px` and
//! `--active-tab-bottom: 2px` against a `p-0.5` list.
//!
//! **gpui has neither custom properties nor `translate`, and a `Div` cannot read
//! a sibling's laid-out box.** So the indicator is rendered as an *absolutely
//! positioned child of the active tab*, inset by the tab's own border width to
//! reach its border box. That is a **structural deviation and it is deliberate**:
//!
//! * it is the only construct in gpui that says "this box is that box" without a
//!   measurement pass, so the indicator's bounds stay an **output** of the port's
//!   own layout;
//! * `ANCHORS.md` §1 makes tree shape irrelevant to the differ — anchors are
//!   compared by id, never by position in a tree;
//! * the rejected alternative was taking `{left, bottom, width, height}` as a
//!   parameter, the way [`super::resizable::Panel::grow`] takes the library's
//!   resolved `flex-grow`. **It is not the same shape of quantity.** `grow` is an
//!   *input* both engines then divide space with; the indicator's box is the
//!   *answer*. Passing it in would make `tab-indicator`'s bounds a copy of the
//!   reference's own numbers — an anchor that cannot fail, on the one part of
//!   this component the item exists to get right.
//!
//! What the deviation assumes is written down rather than hoped for, and
//! [`INDICATOR_INSET`] carries the arithmetic: the list has **no border**
//! (`--active-tab-left` is measured from the list's *border* box while `left: 0`
//! resolves against its *padding* box, so a bordered list would offset the two by
//! the border width) and **does not scroll** (`--active-tab-bottom` is computed
//! from `scrollHeight`).
//!
//! # A snapshot of the indicator is one instant of a 200ms transition
//!
//! `transition-[width,translate] duration-200 ease-in-out`. `width` and
//! `translate` — so `bounds.x`, `bounds.y` **and** `bounds.w` of `tab-indicator`
//! are mid-flight for 200ms after the active tab changes. `height` is not in the
//! list and does not animate. `ANCHORS.md` §6 says a snapshot is one instant and
//! has no field for a curve, so a reference captured inside that window compares
//! an interpolated box against a settled one. This port draws the settled box,
//! always; a capture has to be taken at rest.
//!
//! # `hidden`, which is a `visible: false` waiting to happen
//!
//! base-ui writes `hidden` on the indicator until `width > 0 && height > 0`, and
//! renders **nothing at all** when the root has no value. `[hidden]` is
//! `display: none !important` in Tailwind's preflight, so an indicator caught
//! before layout settles reports `visible: false` on the React side. Here the
//! indicator is an `Option`: no active tab, no element.

use gpui::{
    AnyElement, Div, FontWeight, Length, ParentElement as _, Pixels, SharedString, Styled as _,
    div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::icon::IconName;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: `Tabs`, the `[data-slot=tabs]` div. Every other bound on this
/// surface is reported relative to it (`native/oracle/ANCHORS.md` §4).
pub const ID_ROOT: &str = "tabs";

/// `TabsList` — the `role="tablist"` div.
pub const ID_LIST: &str = "tabs-list";

/// `TabsPrimitive.Indicator` — the span `TabsList` renders after its children.
///
/// One per list, so unlike a tab it needs no suffix.
pub const ID_INDICATOR: &str = "tab-indicator";

/// What a tab's anchor id is built out of. See [`tab_id`].
pub const TAB_ID_PREFIX: &str = "tabs-tab-";

/// What a panel's anchor id is built out of. See [`panel_id`].
pub const PANEL_ID_PREFIX: &str = "tabs-content-";

/// A tab's anchor id: the prefix and the tab's **own `value`**.
///
/// **Not a convenience.** `ANCHORS.md` v1.8 says a snapshot carries each anchor
/// **at most once**, and a live `tabs` root holds three to six tabs. The
/// `dropdown-menu` answer — the primitive writes a fixed default and the call
/// site overrides it — is unavailable here: `sidebar-tab-bar.tsx` builds its tabs
/// from a `map` and `git-panel.tsx` writes two by hand, and neither may be
/// edited by this item.
///
/// `value` is the right key rather than the available one: it is the tab's
/// identity in base-ui, which looks the active tab up by it
/// (`getTabElementBySelectedValue`), and base-ui requires it to be unique within
/// a root for that lookup to work at all.
///
/// **Stated hole.** `value` is optional in base-ui, which falls back to the tab's
/// index; `tabs.tsx` writes `String(props.value)`, so an omitted value yields
/// `tabs-tab-undefined` and two of them collide. That is loud — the extractor
/// refuses a repeated id — rather than silent, which is the correct direction. No
/// live call site omits it.
#[must_use]
pub fn tab_id(value: &str) -> SharedString {
    format!("{TAB_ID_PREFIX}{value}").into()
}

/// A panel's anchor id: the prefix and the panel's own `value`.
///
/// Derived for the same reason [`tab_id`] is, and not because two panels are
/// mounted at once — base-ui unmounts an inactive panel unless a call site asks
/// for `keepMounted`, and none does. `keepMounted` is a prop rather than a fact,
/// so the id does not depend on nobody ever passing it.
#[must_use]
pub fn panel_id(value: &str) -> SharedString {
    format!("{PANEL_ID_PREFIX}{value}").into()
}

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None.** Nothing on this component paints its own text: a tab's label is a
/// `<span>` the *call site* puts inside it, so the tab element has no text node
/// of its own and the React extractor emits no `text` for it. Every box here is
/// either a share of the list's main axis (a tab under `flex-1`), an authored
/// length (the underline's 2px), or 100% of its parent.
///
/// An empty list rather than no list, because the React side's declaration set is
/// also empty and the two have to be diffable against each other.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None**, and for the shortest reason: v1.6 requires a declaring anchor to
/// paint text — the differ refuses, by name, a snapshot carrying `line_sized`
/// without a `font` — and no anchor here does. The tab is the trap this rule was
/// written for twice over: it *contains* the word "Workspaces" and its height is
/// an authored `h-9 sm:h-8`, so declaring it would compare 32 against a 20px line
/// box and manufacture a 12px delta on an anchor both engines agree on.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// Spelled once, as `dropdown_menu` and `resizable` spell it, so a theme that
/// moved the step would move every length below together. `theme.css` does **not**
/// redefine it.
const SPACING: f32 = 4.0;

/// `gap-2` on the root — between the list and the panel.
pub const ROOT_GAP: Pixels = px(SPACING * 2.0);

/// `p-0.5` on a `default` list.
pub const LIST_PADDING: Pixels = px(SPACING * 0.5);

/// `gap-x-0.5` on the list.
///
/// **Column gap only**, which is the trap: the class is `gap-x-0.5` and the
/// vertical variant is `data-[orientation=vertical]:flex-col`, so flipping the
/// list to a column leaves `row-gap` at its `normal` initial value and the tabs
/// touch. Reproduced rather than corrected — see [`Orientation::list_gap`].
pub const LIST_GAP: Pixels = px(SPACING * 0.5);

/// `data-[orientation=horizontal]:py-1` / `data-[orientation=vertical]:px-1` on an
/// `underline` list — its only padding, and on one axis only.
pub const UNDERLINE_LIST_PADDING: Pixels = px(SPACING);

/// `h-9` on a tab: the height a viewport **narrower** than 640px gets.
pub const TAB_HEIGHT_BASE: Pixels = px(SPACING * 9.0);

/// `sm:h-8` on a tab: the height a viewport ≥ 640px gets, and the one a real
/// Crowbar window renders.
pub const TAB_HEIGHT_SM: Pixels = px(SPACING * 8.0);

/// `border` on a tab, as an `f32` so [`TAB_PADDING_X`] and [`INDICATOR_INSET`]
/// can be arithmetic rather than more literals.
const TAB_BORDER_PX: f32 = 1.0;

/// `border border-transparent` on a tab.
///
/// **A real border, unlike `ring-1`.** `ANCHORS.md` v1.1 compares `border.w`
/// *exactly*, and the reference reports `1` on every tab; a port that skipped it
/// because the colour is transparent would be wrong on every cell, and wrong
/// about the box as well — a border takes room in the box model.
pub const TAB_BORDER: Pixels = px(TAB_BORDER_PX);

/// `px-[calc(--spacing(2.5)-1px)]` on a tab: **9px**, not 10.
///
/// The `-1px` is [`TAB_BORDER`] being subtracted back out, so the visible inset
/// stays at the 10px `px-2.5` would have given a tab with no border.
pub const TAB_PADDING_X: Pixels = px(SPACING * 2.5 - TAB_BORDER_PX);

/// `gap-1.5` between a tab's children.
pub const TAB_GAP: Pixels = px(SPACING * 1.5);

/// `[&_svg:not([class*='size-'])]:size-4.5` — a leading icon below 640px.
pub const ICON_SIZE_BASE: Pixels = px(SPACING * 4.5);

/// `sm:[&_svg:not([class*='size-'])]:size-4` — a leading icon at 640px and above.
///
/// **This wins over the SVG's own `width`/`height` attributes.** `sidebar-tab-bar`
/// asks phosphor for `size={14}`, which renders `width="14" height="14"`;
/// presentational attributes have no specificity, so the 16px from the class
/// applies and the icon measures 16 — confirmed live.
pub const ICON_SIZE_SM: Pixels = px(SPACING * 4.0);

/// `[&_svg]:-mx-0.5` — a leading icon's negative side margins, which take 4px
/// back out of its 16.
pub const ICON_MARGIN_X: Pixels = px(SPACING * -0.5);

/// `data-[orientation=horizontal]:h-0.5` / `data-[orientation=vertical]:w-0.5` —
/// the `underline` variant's thickness.
pub const UNDERLINE_THICKNESS: Pixels = px(SPACING * 0.5);

/// `data-[orientation=horizontal]:translate-y-px` /
/// `data-[orientation=vertical]:-translate-x-px` on an `underline` indicator: one
/// pixel away from the tab, on the axis the rule runs across.
///
/// It is not additive. Both are `--tw-translate-*` on the same property as the
/// base `translate-x-(--active-tab-left)` / `-translate-y-(--active-tab-bottom)`,
/// and the variant's selector outranks the bare class — so under `underline` the
/// nudge **replaces** the measured offset on that axis rather than adding to it.
pub const UNDERLINE_NUDGE: Pixels = px(1.0);

/// `focus-visible:ring-2` — **a box-shadow**, 2px of spread with no blur.
///
/// The same trap `dropdown_menu` and `resizable` record for `ring-1`, at twice
/// the width. `border.w` on a tab is [`TAB_BORDER`] in every cell and a port that
/// reached for a border here would report `3` against the reference's `1`.
pub const RING_WIDTH: Pixels = px(2.0);

/// `text-muted-foreground/72` on a `default` list.
pub const LIST_MUTED_ALPHA: f32 = 72.0;

/// `text-foreground/70` — `sidebar-tab-bar.tsx`'s override of it.
pub const SIDEBAR_LIST_FOREGROUND_ALPHA: f32 = 70.0;

/// Where the indicator's insets start from, so it covers the active tab's
/// **border** box rather than its padding box.
///
/// An absolutely positioned box resolves its insets against its containing
/// block's *padding* box, in CSS and in taffy alike; the tab's padding box is its
/// border box shrunk by [`TAB_BORDER`] on every side. So `-1px` all round is the
/// tab's border box, which is what `--active-tab-width`/`-height` measure — the
/// tab's `getComputedStyle().width` under `box-sizing: border-box`.
///
/// Measured rather than reasoned: `row_layout::tabs` lays the surface out in a
/// real window and asserts the indicator's recorded bounds equal the active tab's,
/// so a taffy that resolved insets against the border box instead would fail here
/// rather than ship a one-pixel error.
pub const INDICATOR_INSET: Pixels = px(-TAB_BORDER_PX);

/// Which way a set of tabs runs.
///
/// `orientation` on `Tabs.Root`, which writes `data-orientation` onto the root,
/// the list and every tab — so unlike `resizable`'s, every
/// `data-[orientation=…]` variant here is selected by the **same** value on
/// whichever element carries it. No live call site sets it; both take the
/// default.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Orientation {
    /// Tabs side by side. The only one any live call site renders.
    #[default]
    Horizontal,
    /// Tabs stacked. **No reference**: `sidebar-tab-bar.tsx` and `git-panel.tsx`
    /// both leave `orientation` at its default.
    Vertical,
}

impl Orientation {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Horizontal => "horizontal",
            Self::Vertical => "vertical",
        }
    }

    /// The gap **between tabs** this orientation actually gets.
    ///
    /// `gap-x-0.5` is a `column-gap`, and nothing in the class list sets a
    /// `row-gap`. A horizontal list's main axis is the column axis, so it gets
    /// the 2px; a vertical list stacks along the row axis and gets **nothing**,
    /// leaving its tabs touching. That is the reference's behaviour and it is
    /// reproduced, not corrected — a port that quietly spaced a vertical list
    /// would report a 2px delta per tab that says nothing about the port.
    #[must_use]
    pub const fn list_gap(self) -> Pixels {
        match self {
            Self::Horizontal => LIST_GAP,
            Self::Vertical => px(0.0),
        }
    }
}

/// `TabsList`'s `variant` prop.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Variant {
    /// `rounded-lg bg-muted p-0.5 text-muted-foreground/72` on the list, and a
    /// filled, rounded, bordered indicator behind the active tab.
    ///
    /// The default, and what **both** live call sites pass explicitly.
    #[default]
    Default,
    /// No list background at all, one axis of padding, and a 2px rule in
    /// `bg-primary` at the edge of the list.
    ///
    /// **No reference.** `variant="underline"` appears nowhere in `web/src`
    /// outside the type that declares it.
    Underline,
}

impl Variant {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Default => "default",
            Self::Underline => "underline",
        }
    }
}

/// The list's background, which is the one colour on this component a call site
/// overrides.
///
/// A parameter for the reason [`super::resizable::Panel::grow`] is one: it is not
/// a property of the primitive, and the differ compares `bg` on `tabs-list`
/// exactly, so a port that only knew `bg-muted` would be wrong on every cell of
/// the surface that has a reference.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ListBackground {
    /// `bg-muted` — the primitive's own, and what `git-panel.tsx` gets by passing
    /// no `className` background.
    Muted,
    /// `bg-sidebar-element-idle` — `sidebar-tab-bar.tsx`'s override, and the one
    /// the live reference capture reports (`#f5f5f51c`).
    ///
    /// The default, because the fixture is that call site.
    #[default]
    SidebarElementIdle,
}

impl ListBackground {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Muted => "muted",
            Self::SidebarElementIdle => "sidebar-element-idle",
        }
    }

    /// The colour it paints, or `None` where the variant paints none.
    ///
    /// `underline` carries no background utility at all, and not painting is a
    /// truer statement of that than painting `transparent` — though both extractors
    /// report `#00000000` either way.
    #[must_use]
    pub fn color(self, theme: &Theme, variant: Variant) -> Option<Color> {
        match variant {
            Variant::Underline => None,
            Variant::Default => Some(match self {
                Self::Muted => theme.muted,
                Self::SidebarElementIdle => theme.color_sidebar_element_idle,
            }),
        }
    }

    /// The list's inherited text colour.
    ///
    /// **Invisible to the differ on every cell that has a reference**, and painted
    /// anyway: the tab's label is the call site's `<span>`, so no anchor here
    /// reports an `fg` at all. See `native/mapping/tabs.md`.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> Color {
        match self {
            Self::Muted => theme
                .muted_foreground
                .mix(LIST_MUTED_ALPHA, Color::TRANSPARENT),
            Self::SidebarElementIdle => theme
                .foreground
                .mix(SIDEBAR_LIST_FOREGROUND_ALPHA, Color::TRANSPARENT),
        }
    }
}

/// How a tab claims its share of the list's main axis.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum TabSizing {
    /// `flex-1` — `flex: 1 1 0%`, so every tab is an equal share whatever its
    /// content says.
    ///
    /// **What both live call sites produce**, and therefore the default:
    /// `sidebar-tab-bar.tsx` writes `flex flex-1 items-center justify-center
    /// gap-1` and `git-panel.tsx` writes `flex-1 justify-center`, and
    /// `tailwind-merge` drops the primitive's `shrink-0 grow` against either.
    #[default]
    Fill,
    /// `shrink-0 grow` — the primitive's own: `flex-basis: auto`, so a tab is at
    /// least its content wide and then grows.
    ///
    /// **No reference.** Every call site overrides it.
    Intrinsic,
}

impl TabSizing {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Fill => "fill",
            Self::Intrinsic => "intrinsic",
        }
    }
}

/// One `TabsTab`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Tab {
    /// The tab's `value` — its identity in base-ui, and what [`tab_id`] builds
    /// its anchor id out of.
    pub value: SharedString,
    /// Whether this tab carries `data-active`.
    ///
    /// A **parameter, not a gpui refinement**: `ANCHORS.md` §6 makes runtime
    /// interaction state invisible to the extractor, and this is the one state on
    /// this component that moves an anchored box — it decides where the indicator
    /// is.
    pub selected: bool,
    /// The leading glyph this tab draws, when the call site gives it one.
    ///
    /// An **empty box**, as on every surface so far: the icon is an SVG a call
    /// site chooses, there is no native equivalent, and drawing a substitute
    /// would put a shape on screen for the oracle to converge on. It carries no
    /// anchor, and under [`TabSizing::Fill`] it cannot move one either.
    pub icon: Option<IconName>,
    /// The call site's label span, where it renders one.
    ///
    /// `None` in the fixture, and that is the *live* picture rather than a
    /// simplification: `sidebar-tab-bar.tsx` puts the label behind
    /// `@[280px]:inline` / `@[420px]:inline` container queries, and the sidebar's
    /// query container measures **278px**, so all three labels are `display: none`
    /// — including the active one. Confirmed live.
    pub label: Option<SharedString>,
}

impl Tab {
    /// An unselected tab with a leading icon and no visible label — the live
    /// sidebar's shape.
    #[must_use]
    pub fn new(value: impl Into<SharedString>) -> Self {
        Self {
            value: value.into(),
            selected: false,
            // No glyph by default. A tab that should draw one says which; the
            // old `bool` could only say "reserve a box", which is how three
            // sidebar tabs came to be empty squares.
            icon: None,
            label: None,
        }
    }

    /// The same tab, drawing `icon` as its leading glyph.
    #[must_use]
    pub fn with_icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// The same tab, selected.
    #[must_use]
    pub fn selected(mut self) -> Self {
        self.selected = true;
        self
    }

    /// Its anchor id.
    #[must_use]
    pub fn anchor(&self) -> SharedString {
        tab_id(&self.value)
    }
}

/// One `TabsPanel`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Panel {
    /// The panel's `value`, which [`panel_id`] builds its anchor id out of.
    pub value: SharedString,
}

impl Panel {
    /// A panel for a tab's value.
    #[must_use]
    pub fn new(value: impl Into<SharedString>) -> Self {
        Self {
            value: value.into(),
        }
    }

    /// Its anchor id.
    #[must_use]
    pub fn anchor(&self) -> SharedString {
        panel_id(&self.value)
    }
}

/// A `Tabs` root with its list, its tabs and — where a call site renders one —
/// its panel.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Tabs {
    /// `orientation` on the root.
    pub orientation: Orientation,
    /// `variant` on the list.
    pub variant: Variant,
    /// The list's background and inherited text colour.
    pub list_background: ListBackground,
    /// How the tabs divide the list.
    pub tab_sizing: TabSizing,
    /// Which side of `sm:` (640px) the **viewport** is on, which decides the
    /// tab's height and an icon's size.
    pub breakpoint: Breakpoint,
    /// The tabs, in DOM order. At most one is [`Tab::selected`].
    pub tabs: Vec<Tab>,
    /// The mounted panel, where the call site has one.
    ///
    /// `None` in the fixture: `sidebar-tab-bar.tsx` renders a bare `Tabs` with a
    /// list and no `TabsPanel` at all. See [`Tabs::fixture`] for why the one call
    /// site that *does* have a panel is not the fixture.
    pub panel: Option<Panel>,
}

impl Tabs {
    /// The live sidebar tab bar, as `web/src/components/layout/sidebar-tab-bar.tsx`
    /// renders it on the home route: three tabs, `workspaces` active, no panel.
    ///
    /// **This call site rather than `git-panel.tsx`'s**, and the reason is
    /// `ANCHORS.md` v1.8. A snapshot carries exactly the surface's own anchors, and
    /// the React extractor collects every `data-oracle-id` under the root. This
    /// root contains its list, its tabs and its indicator and **nothing else** — a
    /// `TabsTab`'s children are one phosphor SVG and one `<span>`, neither of which
    /// can carry an anchor. The git panel's root contains `ChangedFilesTree`, whose
    /// rows are `git-status-file-item`s carrying `git-row-icon`, `git-row-name`,
    /// `git-row-badge` and the rest — so a capture there swallows another surface
    /// whenever the fixture workspace has a changed file, which is exactly the
    /// defect that produced the rule.
    ///
    /// It is a property of the *cell* rather than of the surface, so `tabs`
    /// deliberately declares no scope in `oracleSurfaceScope`: the anchor set moves
    /// with the tab count, the tab values and whether a panel is mounted, and a
    /// fixed list would be wrong in most cells.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            orientation: Orientation::Horizontal,
            variant: Variant::Default,
            list_background: ListBackground::SidebarElementIdle,
            tab_sizing: TabSizing::Fill,
            breakpoint: Breakpoint::Sm,
            // The live sidebar tab bar, glyphs included — every one of these
            // draws an icon in the reference, which is what
            // `the_fixture_is_the_live_sidebar_tab_bar` asserts.
            tabs: vec![
                // `SquaresFourFill`, not `SquaresFour`: this tab is the
                // selected one, and `sidebar-tab-bar.tsx` draws the selected
                // tab at `weight="fill"`. The two are different artwork, not
                // one shape styled two ways — see `IconName`'s own docs.
                Tab::new("workspaces")
                    .with_icon(IconName::SquaresFourFill)
                    .selected(),
                Tab::new("chats").with_icon(IconName::ChatsCircle),
                Tab::new("files").with_icon(IconName::FolderOpen),
            ],
            panel: None,
        }
    }

    /// A tab's height: `h-9`, or `sm:h-8` at 640px and above.
    #[must_use]
    pub const fn tab_height(&self) -> Pixels {
        match self.breakpoint {
            Breakpoint::Base => TAB_HEIGHT_BASE,
            Breakpoint::Sm => TAB_HEIGHT_SM,
        }
    }

    /// A leading icon's box: `size-4.5`, or `sm:size-4` at 640px and above.
    #[must_use]
    pub const fn icon_size(&self) -> Pixels {
        match self.breakpoint {
            Breakpoint::Base => ICON_SIZE_BASE,
            Breakpoint::Sm => ICON_SIZE_SM,
        }
    }

    /// Which tab is active, if any.
    ///
    /// The **first** selected one. A `Tabs.Root` has a single `value`, so more
    /// than one active tab is not a picture the reference can produce;
    /// [`Tabs::render`] renders one indicator regardless, which keeps the anchor
    /// set from silently doubling.
    #[must_use]
    pub fn active(&self) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.selected)
    }

    /// Renders the tabs, opting every contract anchor into `anchors`.
    ///
    /// The root **is** the outermost element here, unlike `resizable`'s: this
    /// component needs no definite-size box above it, because `RowSurface` already
    /// draws the surface inside a `--width` box and every height on this component
    /// is authored.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut children: Vec<AnyElement> = vec![self.list(theme, anchors)];
        if let Some(panel) = &self.panel {
            children.push(anchors.boxed(AnchorId::new(panel.anchor()), Self::panel()));
        }
        anchors.root(ID_ROOT.into(), self.root().children(children))
    }

    /// `Tabs.Root`'s own box: `flex flex-col gap-2`, or `flex-row` when the
    /// orientation is vertical.
    ///
    /// `w-full` is the call site's — both live ones stretch the root — and it is
    /// what makes the list's own width the surface's. The primitive itself
    /// declares no width.
    fn root(&self) -> Div {
        let element = div().flex().gap(ROOT_GAP).w(relative(1.0));
        match self.orientation {
            Orientation::Horizontal => element.flex_col(),
            Orientation::Vertical => element.flex_row(),
        }
    }

    /// `TabsList`, its tabs, and the indicator under the active one.
    fn list(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let children: Vec<AnyElement> = self
            .tabs
            .iter()
            .map(|tab| self.tab(theme, anchors, tab))
            .collect();
        anchors.boxed(ID_LIST.into(), self.list_box(theme).children(children))
    }

    /// The list's own box.
    ///
    /// `w-fit` is **absent**, deliberately: taffy has no `fit-content`, expressing
    /// it would mean giving the list an `align-self` the class list does not have,
    /// and `tailwind-merge` replaces it with the `w-full` **both** live call sites
    /// write — so the arm has no reference and reproducing it would be inventing a
    /// picture. Recorded in `native/mapping/tabs.md` rather than approximated.
    fn list_box(&self, theme: &Theme) -> Div {
        let element = div()
            .relative()
            .flex()
            .items_center()
            .justify_center()
            .w(relative(1.0))
            .gap_x(self.orientation.list_gap())
            .text_color(self.list_background.foreground(theme));
        let element = match self.orientation {
            Orientation::Horizontal => element.flex_row(),
            Orientation::Vertical => element.flex_col(),
        };
        let element = match self.variant {
            Variant::Default => element.p(LIST_PADDING).rounded(theme.radius_lg.value()),
            Variant::Underline => match self.orientation {
                Orientation::Horizontal => element.py(UNDERLINE_LIST_PADDING),
                Orientation::Vertical => element.px(UNDERLINE_LIST_PADDING),
            },
        };
        match self.list_background.color(theme, self.variant) {
            Some(background) => element.bg(background),
            None => element,
        }
    }

    /// One `TabsTab`, plus the indicator when this is the active one.
    fn tab(&self, theme: &Theme, anchors: &dyn AnchorSink, tab: &Tab) -> AnyElement {
        let mut element = self.tab_box(theme, tab);
        // **The indicator goes in FIRST.** It is absolutely positioned and
        // opaque, so a later sibling paints over it — but appended last it
        // painted over the tab's own glyph instead, and the active tab was the
        // one tab in the bar with no icon. Nothing in the anchor contract
        // could see that: the indicator's own record was correct, the glyph's
        // was correct, and neither carries paint order.
        if tab.selected && self.active().is_some_and(|first| first.value == tab.value) {
            element = element.child(anchors.boxed(ID_INDICATOR.into(), self.indicator(theme)));
        }
        if let Some(glyph) = tab.icon {
            // **The glyph follows the tab's own colour, not the list's.**
            //
            // In the source the svg has no colour of its own: it inherits
            // `currentColor`, so `data-active:text-foreground` moves the glyph
            // and the label together. Here the icon takes an explicit colour,
            // and this passed `theme.muted_foreground` unconditionally — so
            // the active tab's glyph painted `rgb(164, 164, 164)` where the
            // live window paints `rgb(245, 245, 245)`, a third of the way to
            // the background on the one tab that is supposed to be brightest.
            //
            // The text branch below already had the rule right; only the glyph
            // was missing it. Both now read the same source of truth.
            element = element.child(
                self.icon()
                    .child(glyph.render(self.icon_size(), self.glyph_color(theme, tab))),
            );
        }
        if let Some(label) = &tab.label {
            // **Deliberately unanchored, and not through `AnchorSink` at all.**
            // The label is a `<span>` the call site puts inside the tab, so the
            // tab element has no text node of its own and the React extractor
            // emits no `text`/`fg`/`font`/`text_width`/`clipped` for it. Routing
            // this through `AnchorSink::text_half` would record the run under the
            // tab's id and fabricate the whole text group on one side — five
            // `FieldPresence` deltas on every tab, in the shape `ANCHORS.md` §3
            // records for `git-row-badge` and with the sides swapped.
            element = element.child(div().child(label.clone()));
        }
        anchors.boxed(AnchorId::new(tab.anchor()), element)
    }

    /// A tab's own box.
    fn tab_box(&self, theme: &Theme, tab: &Tab) -> Div {
        let element = div()
            .relative()
            .flex()
            .flex_row()
            .items_center()
            .h(self.tab_height())
            .gap(TAB_GAP)
            .rounded(theme.radius_md.value())
            .border(TAB_BORDER)
            .border_color(Color::TRANSPARENT.value())
            .px(TAB_PADDING_X)
            .font_weight(FontWeight::MEDIUM)
            .text_size(self.text_size(theme));
        let element = match self.tab_sizing {
            // `flex: 1` — `flex-grow: 1; flex-shrink: 1; flex-basis: 0%`.
            TabSizing::Fill => element
                .flex_grow(1.0)
                .flex_shrink(1.0)
                .flex_basis(relative(0.0)),
            // `shrink-0 grow`, over Tailwind's `flex-basis: auto` initial.
            TabSizing::Intrinsic => element
                .flex_grow(1.0)
                .flex_shrink(0.0)
                .flex_basis(Length::Auto),
        };
        let element = match self.orientation {
            // `data-[orientation=vertical]:w-full
            // data-[orientation=vertical]:justify-start`.
            Orientation::Vertical => element.w(relative(1.0)).justify_start(),
            Orientation::Horizontal => element.justify_center(),
        };
        if tab.selected {
            // `data-active:text-foreground`, which beats the list's inherited
            // muted colour.
            element.text_color(theme.foreground)
        } else {
            element
        }
    }

    /// What a tab paints its glyph and its label in.
    ///
    /// One function because the source has one rule: the svg inherits
    /// `currentColor`, so whatever `data-active:text-foreground` does to the
    /// label it does to the glyph. Spelling it twice is how they drifted.
    #[must_use]
    fn glyph_color(&self, theme: &Theme, tab: &Tab) -> Color {
        if tab.selected {
            theme.foreground
        } else {
            self.list_background.foreground(theme)
        }
    }

    /// A tab's font size.
    ///
    /// `text-base` is Tailwind's 1rem and `--ui-text-lg` is the same 1rem;
    /// `sm:text-sm` is 0.875rem and `--ui-text-base` is the same 0.875rem. The
    /// same trade `dropdown_menu` records for the whole port: there is no token
    /// for Tailwind's own scale, and minting one would put a value in the design
    /// system the design system does not have.
    fn text_size(&self, theme: &Theme) -> gpui::Rems {
        match self.breakpoint {
            Breakpoint::Base => theme.ui_text_lg.value(),
            Breakpoint::Sm => theme.ui_text_base.value(),
        }
    }

    /// A leading icon's empty box.
    fn icon(&self) -> Div {
        div()
            .flex_shrink_0()
            .w(self.icon_size())
            .h(self.icon_size())
            .mx(ICON_MARGIN_X)
    }

    /// `Tabs.Indicator`, resolved against the active tab it is a child of.
    ///
    /// Every number here is an inset from the *tab's* padding box; the module
    /// docs carry the arithmetic and what it assumes about the list.
    fn indicator(&self, theme: &Theme) -> Div {
        let element = div().absolute();
        match self.variant {
            // The base geometry: the active tab's border box, exactly.
            Variant::Default => element
                .top(INDICATOR_INSET)
                .bottom(INDICATOR_INSET)
                .left(INDICATOR_INSET)
                .right(INDICATOR_INSET)
                .rounded(theme.radius_lg.value())
                .border(TAB_BORDER)
                .border_color(theme.background.value())
                .bg(theme.background)
                // The **same** raised stack `ROW_ACTIVE` carries, and for the
                // same reason: the live app computes an identical
                // `box-shadow` on `tab-indicator` and on `project-home-row`.
                //
                // This used to be a bare `shadow_xs()`, on the reasoning that
                // `black/10` had "no black token to mint from" and that
                // `ANCHORS.md` §6 "has no field for a shadow, so the differ
                // sees neither the layer nor the difference". `Color::BLACK`
                // exists, and the second half is the argument that also
                // shipped 23 empty icon boxes — the differ not seeing a thing
                // is not the thing being absent.
                .shadow(crate::elevation::raised(TAB_BORDER)),
            Variant::Underline => {
                let element = element.bg(theme.primary);
                match self.orientation {
                    // A rule across the bottom of the list, one pixel below it.
                    Orientation::Horizontal => element
                        .left(INDICATOR_INSET)
                        .right(INDICATOR_INSET)
                        .bottom(px(-(TAB_BORDER_PX
                            + f32::from(UNDERLINE_LIST_PADDING)
                            + 1.0)))
                        .h(UNDERLINE_THICKNESS),
                    // A rule down the left of the list, one pixel outside it.
                    Orientation::Vertical => element
                        .top(INDICATOR_INSET)
                        .bottom(INDICATOR_INSET)
                        .left(px(-(TAB_BORDER_PX + 1.0)))
                        .w(UNDERLINE_THICKNESS),
                }
            }
        }
    }

    /// `TabsPanel`: `flex-1 outline-none`, and nothing else at all.
    fn panel() -> Div {
        div().flex_grow(1.0).flex_shrink(1.0).flex_basis(px(0.0))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ICON_MARGIN_X, ICON_SIZE_BASE, ICON_SIZE_SM, ID_INDICATOR, ID_LIST, ID_ROOT,
        INDICATOR_INSET, LINE_SIZED, LIST_GAP, LIST_MUTED_ALPHA, LIST_PADDING, ListBackground,
        Orientation, PANEL_ID_PREFIX, Panel, RING_WIDTH, ROOT_GAP, SIDEBAR_LIST_FOREGROUND_ALPHA,
        TAB_BORDER, TAB_GAP, TAB_HEIGHT_BASE, TAB_HEIGHT_SM, TAB_ID_PREFIX, TAB_PADDING_X, Tab,
        TabSizing, Tabs, UNDERLINE_LIST_PADDING, UNDERLINE_NUDGE, UNDERLINE_THICKNESS, Variant,
        panel_id, tab_id,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind 4.3.0 compiles the class to, at `--spacing: 0.25rem` and a 16px
    /// root. Spelled as the arithmetic rather than as more literals, so a change
    /// to the step cannot pass by leaving the literals alone.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(ROOT_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(LIST_PADDING, px(STEP * 0.5)); // p-0.5
        assert_eq!(LIST_GAP, px(STEP * 0.5)); // gap-x-0.5
        assert_eq!(UNDERLINE_LIST_PADDING, px(STEP)); // py-1 / px-1
        assert_eq!(TAB_HEIGHT_BASE, px(STEP * 9.0)); // h-9
        assert_eq!(TAB_HEIGHT_SM, px(STEP * 8.0)); // sm:h-8
        assert_eq!(TAB_GAP, px(STEP * 1.5)); // gap-1.5
        assert_eq!(ICON_SIZE_BASE, px(STEP * 4.5)); // size-4.5
        assert_eq!(ICON_SIZE_SM, px(STEP * 4.0)); // sm:size-4
        assert_eq!(ICON_MARGIN_X, px(STEP * -0.5)); // -mx-0.5
        assert_eq!(UNDERLINE_THICKNESS, px(STEP * 0.5)); // h-0.5 / w-0.5

        // The three that are not spacing multiples and must not become them:
        // `border` and the two `px` translates are literal pixels in Tailwind's
        // own scale.
        assert_eq!(TAB_BORDER, px(1.0));
        assert_eq!(UNDERLINE_NUDGE, px(1.0));
        assert_eq!(INDICATOR_INSET, px(-1.0));
    }

    /// **A tab's horizontal padding is 9, not 10**, and the missing pixel is the
    /// border it sits inside — which is the whole point of writing the class as
    /// `px-[calc(--spacing(2.5)-1px)]` rather than as `px-2.5`.
    #[test]
    fn the_tabs_padding_has_its_own_border_subtracted_out() {
        assert_eq!(TAB_PADDING_X, px(9.0));
        assert_eq!(TAB_PADDING_X + TAB_BORDER, px(4.0 * 2.5));
    }

    /// `focus-visible:ring-2` is a **shadow**, so a tab's border stays at the
    /// 1px `border border-transparent` gives it — the field `ANCHORS.md` v1.1
    /// compares *exactly*.
    #[test]
    fn the_ring_is_shadow_spread_and_not_more_border() {
        assert_eq!(RING_WIDTH, px(2.0));
        assert_ne!(RING_WIDTH, TAB_BORDER);
    }

    /// **`gap-x-0.5` is a column gap**, so flipping the list to a column loses
    /// it entirely. Reproduced rather than corrected.
    #[test]
    fn a_vertical_list_gets_no_gap_at_all() {
        assert_eq!(Orientation::Horizontal.list_gap(), LIST_GAP);
        assert_eq!(Orientation::Vertical.list_gap(), px(0.0));
        assert_eq!(Orientation::default(), Orientation::Horizontal);
        assert_eq!(Orientation::Horizontal.name(), "horizontal");
        assert_eq!(Orientation::Vertical.name(), "vertical");
    }

    /// The fixture is the live sidebar tab bar on the home route: three tabs,
    /// the first active, no panel, and the call site's background override.
    #[test]
    fn the_fixture_is_the_live_sidebar_tab_bar() {
        let tabs = Tabs::fixture();

        assert_eq!(tabs.orientation, Orientation::Horizontal);
        assert_eq!(tabs.variant, Variant::Default);
        assert_eq!(tabs.list_background, ListBackground::SidebarElementIdle);
        assert_eq!(tabs.tab_sizing, TabSizing::Fill);
        assert_eq!(tabs.breakpoint, Breakpoint::Sm);
        assert!(tabs.panel.is_none());

        let values: Vec<&str> = tabs.tabs.iter().map(|tab| tab.value.as_ref()).collect();
        assert_eq!(values, ["workspaces", "chats", "files"]);

        // Exactly one active tab, and it is the first — which is what
        // `getInitialState()` leaves `useSidebarStore` on.
        let selected: Vec<&str> = tabs
            .tabs
            .iter()
            .filter(|tab| tab.selected)
            .map(|tab| tab.value.as_ref())
            .collect();
        assert_eq!(selected, ["workspaces"]);
        assert_eq!(
            tabs.active().map(|tab| tab.value.as_ref()),
            Some("workspaces")
        );

        // The live cell shows a leading icon and no label on every tab — the
        // sidebar's query container is 278px, under `@[280px]`.
        assert!(tabs.tabs.iter().all(|tab| tab.icon.is_some()));
        assert!(tabs.tabs.iter().all(|tab| tab.label.is_none()));

        // At `sm:`, which is the only side of 640px a real window is on.
        assert_eq!(tabs.tab_height(), TAB_HEIGHT_SM);
        assert_eq!(tabs.icon_size(), ICON_SIZE_SM);
    }

    /// **A tab's anchor id is its own `value`**, which is what keeps three tabs
    /// under one root from being three copies of one anchor — the thing
    /// `ANCHORS.md` v1.8 forbids.
    #[test]
    fn every_tabs_anchor_id_is_distinct_because_it_is_its_value() {
        let tabs = Tabs::fixture();

        let ids: Vec<String> = tabs
            .tabs
            .iter()
            .map(|tab| tab.anchor().to_string())
            .collect();
        assert_eq!(
            ids,
            ["tabs-tab-workspaces", "tabs-tab-chats", "tabs-tab-files"],
        );

        let mut sorted = ids.clone();
        sorted.sort();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");

        // And the fixed ids do not collide with a derived one or with each other.
        let fixed = [ID_ROOT, ID_LIST, ID_INDICATOR];
        let mut all: Vec<String> = fixed.iter().map(|id| (*id).to_owned()).collect();
        all.extend(ids);
        all.push(panel_id("changes").to_string());
        let before = all.len();
        all.sort();
        all.dedup();
        assert_eq!(all.len(), before, "{all:?}");

        assert_eq!(tab_id("history"), "tabs-tab-history");
        assert_eq!(panel_id("history"), "tabs-content-history");
        assert!(tab_id("x").starts_with(TAB_ID_PREFIX));
        assert!(panel_id("x").starts_with(PANEL_ID_PREFIX));
        assert_eq!(Panel::new("changes").anchor(), "tabs-content-changes");
    }

    /// The list's background is the one colour a call site overrides, and the
    /// two overrides are genuinely different colours in both themes — or the
    /// parameter would be decoration.
    #[test]
    fn the_lists_background_is_the_call_sites_and_the_two_differ() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let muted = ListBackground::Muted.color(&theme, Variant::Default);
            let sidebar = ListBackground::SidebarElementIdle.color(&theme, Variant::Default);

            assert_eq!(muted, Some(theme.muted));
            assert_eq!(sidebar, Some(theme.color_sidebar_element_idle));
            assert_ne!(muted, sidebar);

            // `underline` carries no background utility at all.
            assert_eq!(
                ListBackground::Muted.color(&theme, Variant::Underline),
                None
            );
            assert_eq!(
                ListBackground::SidebarElementIdle.color(&theme, Variant::Underline),
                None,
            );

            // And the inherited text colours are two different mixes.
            assert_eq!(
                ListBackground::Muted.foreground(&theme),
                theme
                    .muted_foreground
                    .mix(LIST_MUTED_ALPHA, Color::TRANSPARENT),
            );
            assert_eq!(
                ListBackground::SidebarElementIdle.foreground(&theme),
                theme
                    .foreground
                    .mix(SIDEBAR_LIST_FOREGROUND_ALPHA, Color::TRANSPARENT),
            );
            assert_ne!(
                ListBackground::Muted.foreground(&theme),
                ListBackground::SidebarElementIdle.foreground(&theme),
            );
            // A 70% mix is a different colour from the token, or the `/70` would
            // be decoration too.
            assert_ne!(
                ListBackground::SidebarElementIdle.foreground(&theme),
                theme.foreground,
            );
        }

        assert_eq!(
            ListBackground::default(),
            ListBackground::SidebarElementIdle
        );
        assert_eq!(ListBackground::Muted.name(), "muted");
        assert_eq!(
            ListBackground::SidebarElementIdle.name(),
            "sidebar-element-idle",
        );
    }

    /// **The indicator's radius is the list's, not the tab's.** `rounded-lg` on
    /// the indicator against `rounded-md` on the tab it covers, and this project
    /// redefines both away from Tailwind's stock 8 and 6.
    #[test]
    fn the_indicator_is_rounder_than_the_tab_it_covers() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_lg.value(), px(10.0));
            assert_eq!(theme.radius_md.value(), px(8.0));
            assert!(theme.radius_lg.value() > theme.radius_md.value());
        }
    }

    /// At most one indicator, however many tabs claim to be selected — a
    /// `Tabs.Root` has one `value`, and a doubled anchor id is a refusal rather
    /// than a delta.
    #[test]
    fn a_second_selected_tab_does_not_get_a_second_indicator() {
        let mut tabs = Tabs::fixture();
        tabs.tabs[2].selected = true;

        assert_eq!(
            tabs.active().map(|tab| tab.value.as_ref()),
            Some("workspaces")
        );
        assert_eq!(tabs.tabs.iter().filter(|tab| tab.selected).count(), 2);

        // And with nothing selected there is no active tab at all, which is what
        // makes the indicator an `Option` rather than always-rendered.
        let none = Tabs {
            tabs: vec![Tab::new("workspaces"), Tab::new("chats")],
            ..Tabs::fixture()
        };
        assert!(none.active().is_none());
    }

    /// The breakpoint decides two lengths and nothing else, and the `sm:` side is
    /// the one a real window renders.
    #[test]
    fn the_breakpoint_moves_the_tab_height_and_the_icon() {
        let base = Tabs {
            breakpoint: Breakpoint::Base,
            ..Tabs::fixture()
        };
        assert_eq!(base.tab_height(), TAB_HEIGHT_BASE);
        assert_eq!(base.icon_size(), ICON_SIZE_BASE);
        assert_ne!(base.tab_height(), TAB_HEIGHT_SM);
        assert_ne!(base.icon_size(), ICON_SIZE_SM);
        assert_eq!(Breakpoint::default(), Breakpoint::Sm);
    }

    /// Both declaration lists are empty, and on this surface that follows from
    /// one fact: **no anchor here paints its own text.** A tab's label is a
    /// `<span>` the call site puts inside it.
    #[test]
    fn nothing_on_this_surface_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The two vocabularies a caption and a command line spell.
    #[test]
    fn every_option_has_a_word() {
        assert_eq!(Variant::default(), Variant::Default);
        assert_eq!(Variant::Default.name(), "default");
        assert_eq!(Variant::Underline.name(), "underline");
        assert_eq!(TabSizing::default(), TabSizing::Fill);
        assert_eq!(TabSizing::Fill.name(), "fill");
        assert_eq!(TabSizing::Intrinsic.name(), "intrinsic");
    }
}
