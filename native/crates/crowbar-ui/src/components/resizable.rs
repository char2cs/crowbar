//! The second Phase 2 component: `resizable` — the split-pane group the IDE
//! shell is built out of, its panels, and the separator between them.
//!
//! The native half of `web/src/components/ui/resizable.tsx`, which is a thin set
//! of Tailwind class lists over `react-resizable-panels` 4.11.2. Unlike
//! `dropdown-menu`, **the class lists are not the whole story here**: the library
//! writes inline styles onto all three elements, `web/src/index.css` overrides two
//! of them with `!important`, and an inline style beats a class. Every value in
//! the constants below was read out of the library's own `dist` bundle or out of
//! the app's own Tailwind 4.3.0 — see `native/MAPPING.md` §`resizable`.
//!
//! # What this component is
//!
//! It is **the boxes**: the group, the panels' two nested divs, the separator and
//! its optional grip. It is not the *resize interaction* — the pointer maths, the
//! hit regions, the collapse/expand behaviour and the persisted layout all live in
//! the library and produce, once a frame has settled, exactly one thing this port
//! can see: a `flex-grow` number per panel. So [`Panel::grow`] is a parameter, for
//! the same reason [`super::DropdownMenu::anchor_width`] is one.
//!
//! # This is a geometry-only surface, and that has to be said first
//!
//! Every anchor here reports `bg: #00000000`, no text, and — except the grip —
//! `radius: 0` and `border.w: 0`. That is not an omission in the port; it is what
//! the component paints. Three consequences, all of them recorded in
//! `native/MAPPING.md` rather than discovered by a later reader:
//!
//! | Axis | Here |
//! |---|---|
//! | width | **real**, and the point of the surface. `flex-basis: 0` with fractional `flex-grow` around a fixed 1px sibling is exactly where taffy and `WebKit` can round differently |
//! | theme | vacuous **unless** the grip is rendered — `bg-border` is the only colour on the whole component, and no live call site passes `withHandle` |
//! | content | vacuous. Nothing here paints a character |
//! | `hover` | has a real original and the oracle **cannot see it**: `hover:after:bg-border/60` paints a `::after`, and `ANCHORS.md` §3 forbids the pseudo-backed shortcut for a pseudo that is not `inset: 0`. This one is `left: 50%` with a translate |
//! | `focus` | has a real original and the oracle **cannot see it either**: `focus-visible:ring-1` is a box-shadow, and §6 has no field for shadows |
//!
//! # Why the two declaration lists are empty
//!
//! [`CONTENT_SIZED`] and [`LINE_SIZED`] are both `[]`. v1.6 cannot apply to
//! anything on a surface with no text at all, and v1.5 applies to a box whose used
//! width **is** a text run's max-content width — a panel's is a fraction of the
//! group's, and the separator's and the grip's are authored lengths.

use gpui::{
    AnyElement, BoxShadow, Div, IntoElement as _, Length, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `ResizablePanelGroup`, the `[data-group]` div. Every other
/// bound on this surface is reported relative to it
/// (`native/oracle/ANCHORS.md` §4).
pub const ID_GROUP: &str = "resize-group";

/// `ResizablePanel` — the **outer** of the two divs the library renders.
///
/// A group with more than one panel names them at the call site, exactly as a
/// menu with several rows does: the React extractor walks `[data-oracle-id]`
/// under the root anchor only, so ids have to be unique within one group, and
/// `resizable.tsx` has no index to build them from. `ide-shell.tsx` names its two
/// `resize-panel-sidebar` and `resize-panel-content`.
pub const ID_PANEL: &str = "resize-panel";

/// `ResizableHandle` — the `role="separator"` div.
pub const ID_HANDLE: &str = "resize-handle";

/// The grip `withHandle` renders inside the separator.
///
/// **No live call site passes `withHandle`.** `ide-shell.tsx` writes
/// `<ResizableHandle key="handle" data-testid="sidebar-resize-handle" />` and it
/// is the only one in the app, so this anchor has no reference — the same shape
/// of finding as Phase 1's `git-row-dir`. It is rendered anyway, because it is the
/// only thing on this component that paints a colour or a radius.
pub const ID_HANDLE_GRIP: &str = "resize-handle-grip";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and for the shortest possible reason: nothing on this surface paints
/// text. v1.5 exists because gpui `ceil()`s a *text run's* max-content width where
/// `WebKit` keeps the fraction, so it applies to a box whose used width **is** that
/// max-content width. Every box here is either a fraction of the group's main axis
/// (the panels), an authored length (the separator's 1px, the grip's 4×24), or
/// `100%` of its parent.
///
/// An empty list rather than no list, because the React side's declaration set is
/// also empty and the two have to be diffable against each other.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None.** v1.6's own rule is that a declaring anchor must paint text — the
/// differ refuses, by anchor name, a snapshot carrying `line_sized` without a
/// `font` — and no anchor here does. Declaring any of them would be the badge trap
/// with the text taken away.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// Spelled once, as `dropdown_menu` spells it, so a theme that moved the step
/// would move every length below together. `theme.css` does **not** redefine it.
const SPACING: f32 = 4.0;

/// `w-px` on a vertical separator, `h-px` on a horizontal one — the whole visible
/// thickness of the divider, as an `f32` so the hit strip's offsets can be
/// arithmetic rather than more literals.
const HANDLE_THICKNESS_PX: f32 = 1.0;

/// `w-px` / `aria-[orientation=horizontal]:h-px`.
pub const HANDLE_THICKNESS: Pixels = px(HANDLE_THICKNESS_PX);

/// `after:w-1.5` / `aria-[orientation=horizontal]:after:h-1.5` — the pointer
/// target, six times the thickness of the line it grabs.
const HIT_THICKNESS_PX: f32 = SPACING * 1.5;

/// [`HIT_THICKNESS_PX`] as gpui takes it.
pub const HIT_THICKNESS: Pixels = px(HIT_THICKNESS_PX);

/// Where the hit strip's leading edge lands next to a **vertical** separator (a
/// horizontal group): `after:left-1/2` is 50% of the *containing block* — the
/// separator's 1px — and `after:-translate-x-1/2` is 50% of the *strip's own*
/// 6px. So `0.5 − 3 = −2.5`, and the strip is centred on the separator's centre.
///
/// Resolved here rather than declared, because **gpui has no `translate`**. The
/// resolution is exact rather than approximate: both percentages are of lengths
/// this component authors, so there is no engine-dependent value in it.
pub const HIT_LEFT: Pixels = px(HANDLE_THICKNESS_PX / 2.0 - HIT_THICKNESS_PX / 2.0);

/// Where the hit strip's top edge lands next to a **horizontal** separator (a
/// vertical group): `after:left-0` replaces the 50%, but `after:-translate-y-1/2`
/// has **no `top-1/2` counterpart** in the class list — `after:inset-y-0` gives
/// `top: 0`, and the translate then moves the strip up by half its own height.
///
/// So it is `−3`, and the strip is centred on the separator's **top edge** rather
/// than on its centre: half a pixel off, in the reference. Reproduced rather than
/// corrected — a port that quietly centred it would report a 0.5px delta that says
/// nothing about the port.
pub const HIT_TOP: Pixels = px(-HIT_THICKNESS_PX / 2.0);

/// `hover:after:bg-border/60`.
pub const HIT_HOVER_ALPHA: f32 = 60.0;

/// `h-6` on the grip — its length along the separator.
pub const GRIP_LENGTH: Pixels = px(SPACING * 6.0);

/// `w-1` on the grip — its thickness across the separator.
pub const GRIP_THICKNESS: Pixels = px(SPACING);

/// `focus-visible:ring-1` — a **box-shadow**, 1px of spread with no blur, not a
/// border.
///
/// The same trap `dropdown_menu` records for the popup's `ring-1`, in the one
/// place on this component where it could change a compared field: `border.w` is
/// compared *exactly* by `ANCHORS.md` v1.1 and the separator's is **zero in every
/// cell**. `ring-offset-background` is also in the class list and paints nothing —
/// it sets `--tw-ring-offset-color` while `--tw-ring-offset-width` stays at its
/// `0px` initial, so the offset layer keeps its `0 0 #0000` initial value.
pub const RING_WIDTH: Pixels = px(1.0);

/// `flex-basis: 0` on a panel's outer div, from the library's inline style.
///
/// Load-bearing, and the reason the width axis is worth measuring: with a zero
/// basis every panel's used size is purely its share of the free space, so the
/// whole layout is one division that taffy and `WebKit` each round their own way.
pub const PANEL_FLEX_BASIS: Pixels = px(0.0);

/// Which way a **group** stacks its panels.
///
/// `orientation` on `ResizablePanelGroup`, and the *only* thing that decides the
/// layout: the class list's `aria-[orientation=vertical]:flex-col` is **dead**
/// (see [`Orientation::handle_axis`]), and the library writes
/// `flexDirection: row | column` inline instead.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Orientation {
    /// Panels side by side, separated by a vertical rule. The IDE shell's, and
    /// the only one any live call site asks for.
    #[default]
    Horizontal,
    /// Panels stacked, separated by a horizontal rule. **No reference**: the app
    /// has exactly one `ResizablePanelGroup` and it is `orientation="horizontal"`.
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

    /// The value the **separator** carries in `aria-orientation`, which is the
    /// **opposite** of the group's.
    ///
    /// `Separator` computes it as
    /// `orientation === "horizontal" ? "vertical" : "horizontal"`: a group whose
    /// panels sit side by side is separated by a rule that runs *down* the screen.
    ///
    /// This matters more than it reads, because every `aria-[orientation=…]`
    /// variant in `resizable.tsx` is written on the **separator**. So
    /// `aria-[orientation=horizontal]:w-full` is the rule a **vertical group**
    /// gets, and a port that matched the words up would give the horizontal group
    /// a full-width separator and collapse the whole layout.
    ///
    /// The group's own `aria-[orientation=vertical]:flex-col` is a third thing
    /// again: `Group` renders **no `aria-orientation` at all**, so that class can
    /// never match. It is dead, and the inline `flexDirection` is what stacks the
    /// panels — see [`Orientation`].
    #[must_use]
    pub const fn handle_axis(self) -> Self {
        match self {
            Self::Horizontal => Self::Vertical,
            Self::Vertical => Self::Horizontal,
        }
    }
}

/// The visual state a separator is painted in.
///
/// **Parameters, not gpui `.hover(…)` refinements** — `native/oracle/ANCHORS.md`
/// §6 makes runtime interaction state invisible to the extractor. Both of these
/// are invisible to it for a *second* reason as well, which is the honest headline
/// of this component: see the fields.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct HandleState {
    /// `hover:after:bg-border/60` — the pointer is over the separator.
    ///
    /// **The only hover rule on the whole component**, and it paints the `::after`
    /// hit strip rather than the separator. `ANCHORS.md` §3 permits a pseudo-backed
    /// anchor *only* while the pseudo is `position: absolute; inset: 0`, because
    /// the extractor then synthesises its bounds from the host's padding box. This
    /// pseudo is `left: 50%; width: 1.5rem` with a translate, so the shortcut is
    /// explicitly forbidden — and taking it anyway would have both sides agree on
    /// a 1px-wide box that is really 6px.
    ///
    /// So the strip is painted, for fidelity, and **deliberately unanchored**.
    pub hovered: bool,
    /// `focus-visible:ring-1 ring-ring` — the separator holds keyboard focus.
    ///
    /// The separator is `tabIndex={0}`, so this is a real state a user reaches.
    /// The ring is a **box-shadow** ([`RING_WIDTH`]), which `ANCHORS.md` §6 has no
    /// field for; `focus-visible:outline-hidden` sets `outline-style: none`, which
    /// it has no field for either. Painted, and invisible to the differ.
    pub focused: bool,
}

impl HandleState {
    /// The resting separator.
    #[must_use]
    pub const fn resting() -> Self {
        Self {
            hovered: false,
            focused: false,
        }
    }
}

/// The colour the `::after` hit strip paints, or `None` for "paints nothing".
///
/// Returned rather than applied so the strip can carry no background at all: the
/// class list gives it one under `:hover` and none at rest, and not painting is the
/// truer statement of that.
#[must_use]
pub fn hit_background(theme: &Theme, state: HandleState) -> Option<Color> {
    if state.hovered {
        Some(theme.border.mix(HIT_HOVER_ALPHA, Color::TRANSPARENT))
    } else {
        None
    }
}

/// One `ResizablePanel`.
#[derive(Clone, Debug, PartialEq)]
pub struct Panel {
    /// The anchor its **outer** div carries. Authored rather than derived: a group
    /// has at least two panels and `resizable.tsx` has no index to build ids from,
    /// so the call site names them on both sides.
    pub id: SharedString,
    /// `flex-grow`, as the library's layout engine resolved it.
    ///
    /// **A parameter, not a property.** `Group` writes
    /// `style={{ flexGrow: layout[panelId] }}` onto every panel from its own
    /// measurement of the group and the panels' `minSize`/`maxSize`/`defaultSize`
    /// constraints, and the numbers it writes are **percentages summing to 100**.
    /// Reproducing that engine is reproducing the wrong thing: what a parity run
    /// compares is whether two flex implementations divide the same free space the
    /// same way, given the same growth factors.
    ///
    /// Only the ratios matter, because [`PANEL_FLEX_BASIS`] is zero.
    pub grow: f32,
}

impl Panel {
    /// A panel with a named anchor and a growth factor.
    #[must_use]
    pub fn new(id: impl Into<SharedString>, grow: f32) -> Self {
        Self {
            id: id.into(),
            grow,
        }
    }
}

/// One `ResizableHandle`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Handle {
    /// The anchor it carries.
    pub id: SharedString,
    /// `withHandle` — render the grip.
    ///
    /// Defaults to `false` because the live call site does, and the prop's own
    /// default is `undefined`. See [`ID_HANDLE_GRIP`] for why the `true` arm has no
    /// reference to be compared against.
    pub with_handle: bool,
    /// The visual state.
    pub state: HandleState,
}

impl Handle {
    /// The separator the IDE shell renders: no grip, resting.
    #[must_use]
    pub fn new(id: impl Into<SharedString>) -> Self {
        Self {
            id: id.into(),
            with_handle: false,
            state: HandleState::resting(),
        }
    }
}

/// One child of a group, in DOM order.
///
/// A group is `[panel, handle, panel]` in the live shell and could be any
/// alternation, so the order is data rather than three fields — the same shape
/// [`super::dropdown_menu::MenuEntry`] takes, and for the same reason.
#[derive(Clone, Debug, PartialEq)]
pub enum GroupEntry {
    /// `ResizablePanel`.
    Panel(Panel),
    /// `ResizableHandle`.
    Handle(Handle),
}

/// A `ResizablePanelGroup` with its panels and separators.
#[derive(Clone, Debug, PartialEq)]
pub struct ResizablePanelGroup {
    /// `orientation`.
    pub orientation: Orientation,
    /// The height of the box the group's own `height: 100%` resolves against.
    ///
    /// **A parameter, and it has to be one.** The group declares `height: 100%`
    /// (inline, from the library, and again in a dead `h-full` class); in the app
    /// its containing block is `SidebarProvider`'s `h-screen` div, so the used
    /// height is the window's. A percentage against an auto-height parent computes
    /// to `auto`, which would collapse every panel to nothing and make every bound
    /// on this surface zero — so the port renders the group inside a box of this
    /// height, **above** the root anchor and therefore outside the snapshot
    /// (`ANCHORS.md` §4).
    ///
    /// It is the same kind of quantity as [`super::DropdownMenu::anchor_width`]: a
    /// runtime measurement of an element this component does not contain.
    pub shell_height: Pixels,
    /// The children, in DOM order.
    pub entries: Vec<GroupEntry>,
}

impl ResizablePanelGroup {
    /// The IDE shell's own group, which is the **only** `ResizablePanelGroup` in
    /// the app: a sidebar panel, a separator with no grip, and a content panel.
    ///
    /// The growth factors are the shape `use-sidebar-panel.ts` produces at its
    /// remembered default — a 294px sidebar (`SIDEBAR_DEFAULT_PX`) in a 1200px
    /// window, less the separator's own pixel, expressed as the percentages the
    /// library writes. They are a *starting point* for the width axis rather than
    /// a constant of the component: `--grow` moves them, and a parity run has to
    /// drive both sides to the same pair whatever the window is.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            orientation: Orientation::Horizontal,
            shell_height: DEFAULT_SHELL_HEIGHT,
            entries: vec![
                GroupEntry::Panel(Panel::new("resize-panel-sidebar", SIDEBAR_GROW)),
                GroupEntry::Handle(Handle::new(ID_HANDLE)),
                GroupEntry::Panel(Panel::new("resize-panel-content", CONTENT_GROW)),
            ],
        }
    }

    /// Renders the group, opting every contract anchor into `anchors`.
    ///
    /// The outermost element is **not** the root anchor: it is the definite-height
    /// box [`ResizablePanelGroup::shell_height`] describes, standing in for
    /// `SidebarProvider`'s `h-screen` div. It carries no anchor, so it cannot reach
    /// a snapshot — every bound is reported relative to `resize-group`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let orientation = self.orientation;
        let children: Vec<AnyElement> = self
            .entries
            .iter()
            .map(|entry| Self::entry(theme, anchors, orientation, entry))
            .collect();

        div()
            .w(relative(1.0))
            .h(self.shell_height)
            .child(anchors.root(ID_GROUP.into(), self.group().children(children)))
            .into_any_element()
    }

    /// `Group`'s own box.
    ///
    /// Four of the five declarations come from the library's inline style, which
    /// beats the `cn('flex h-full w-full …')` class list saying the same three
    /// things. The fifth is the one the class list and the inline style
    /// **disagree** about: the library writes `overflow: hidden` and
    /// `web/src/index.css` overrides it with
    /// `[data-slot='resizable-panel-group'] { overflow: visible !important }` so a
    /// pane's shadow can bleed into the sidebar. `!important` beats an inline
    /// declaration, so the group does **not** clip — which is why an overflowing
    /// panel would be reported `visible` rather than cut, on both sides.
    fn group(&self) -> Div {
        let element = div().flex().flex_nowrap().w(relative(1.0)).h(relative(1.0));
        match self.orientation {
            Orientation::Horizontal => element.flex_row(),
            Orientation::Vertical => element.flex_col(),
        }
    }

    fn entry(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        orientation: Orientation,
        entry: &GroupEntry,
    ) -> AnyElement {
        match entry {
            GroupEntry::Panel(panel) => anchors.boxed(
                AnchorId::new(panel.id.clone()),
                Self::panel(panel).child(Self::panel_content()),
            ),
            GroupEntry::Handle(handle) => Self::handle(theme, anchors, orientation, handle),
        }
    }

    /// A panel's **outer** div — `[data-panel]`, and the one `data-slot` and any
    /// `data-oracle-id` land on.
    ///
    /// Everything here is inline style; `ResizablePanel` passes no class of its own
    /// and the library puts a call site's `className` on the *inner* div. The
    /// `min-width: 0` is the one to keep: without it a flex item takes CSS's
    /// automatic minimum size, which is content-based and would stop the panel
    /// shrinking below its children — the library writes it explicitly and taffy
    /// defaults to `auto` exactly as CSS does.
    fn panel(panel: &Panel) -> Div {
        div()
            // `display: flex`, with **no** `flex-direction`: a panel is a row
            // container whichever way its group stacks.
            .flex()
            .flex_basis(PANEL_FLEX_BASIS)
            .flex_shrink(1.0)
            .flex_grow(panel.grow)
            .min_w(px(0.0))
            .min_h(px(0.0))
            .max_w(relative(1.0))
            .max_h(relative(1.0))
    }

    /// A panel's **inner** div, which holds the panel's children.
    ///
    /// **Unanchorable, and rendered anyway.** It is the library's own element:
    /// nothing in `resizable.tsx` or at a call site can put an attribute on it, so
    /// no `data-oracle-id` can reach it and the differ will never see it. It is
    /// still what a panel's content lays out inside, so leaving it out would make
    /// the port's tree a different tree the first time a panel has content.
    ///
    /// Its inline `overflow: auto` is overridden by `index.css`'s
    /// `[data-slot='resizable-panel'] > div { overflow: visible !important }`, so
    /// this scrolls nothing.
    fn panel_content() -> Div {
        div()
            .flex_grow(1.0)
            .max_w(relative(1.0))
            .max_h(relative(1.0))
    }

    /// One separator, plus its hit strip and — where `withHandle` asks for it —
    /// its grip.
    fn handle(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        orientation: Orientation,
        handle: &Handle,
    ) -> AnyElement {
        let mut element = Self::handle_box(theme, orientation, handle)
            // The `::after`, first in paint order as a pseudo-element is.
            // Unanchored: see `HandleState::hovered`.
            .child(Self::hit_strip(theme, orientation, handle.state));
        if handle.with_handle {
            element = element.child(anchors.boxed(ID_HANDLE_GRIP.into(), grip(theme)));
        }
        anchors.boxed(AnchorId::new(handle.id.clone()), element)
    }

    /// The separator's own box: `relative flex items-center justify-center`, the
    /// library's `flex: 0 0 auto`, and whichever of `w-px` / `h-px w-full` its
    /// **`aria-orientation`** selects.
    ///
    /// The base class list carries `w-px` unconditionally and the variant adds
    /// `aria-[orientation=horizontal]:w-full`; the variant wins because a
    /// class-plus-attribute selector outranks a bare class, not because
    /// tailwind-merge dropped anything. Reproduced as one branch rather than two
    /// declarations, since gpui has no cascade to resolve them with.
    fn handle_box(theme: &Theme, orientation: Orientation, handle: &Handle) -> Div {
        let element = div()
            .relative()
            .flex()
            .items_center()
            .justify_center()
            .flex_grow(0.0)
            .flex_shrink(0.0)
            .flex_basis(Length::Auto);
        let element = match orientation {
            // `aria-orientation="vertical"`: `w-px`, stretched down the group.
            Orientation::Horizontal => element.w(HANDLE_THICKNESS),
            // `aria-orientation="horizontal"`: `h-px w-full`.
            Orientation::Vertical => element.h(HANDLE_THICKNESS).w(relative(1.0)),
        };
        if handle.state.focused {
            ring(element, theme.ring)
        } else {
            element
        }
    }

    /// The `::after` pointer target.
    ///
    /// Six pixels of hit area over a one-pixel line, absolutely positioned so it
    /// takes no room in the flex layout. Painted for fidelity and **not anchored**
    /// — `HandleState::hovered` says why, and `native/MAPPING.md` records it as the
    /// one thing on this surface the differ cannot be shown.
    ///
    /// Because it is unanchored it is also **untestable through the oracle**, so
    /// its layout was measured once by hand — a temporary anchor, read back, and
    /// removed — rather than left as an assumption:
    ///
    /// | group | separator | strip | |
    /// |---|---|---|---|
    /// | horizontal | `x 78, 1×160` | `x 75.5, 6×160` | centred on the separator's centre, 78.5 |
    /// | vertical | `y 60, 600×1` | `y 57, 600×6` | centred on the separator's **top edge**, 60 — the half-pixel [`HIT_TOP`] describes |
    ///
    /// Recorded here because there is no gate that would notice if it stopped
    /// being true.
    fn hit_strip(theme: &Theme, orientation: Orientation, state: HandleState) -> Div {
        let element = div().absolute();
        let element = match orientation {
            // `after:inset-y-0 after:left-1/2 after:w-1.5 after:-translate-x-1/2`.
            Orientation::Horizontal => element
                .top(px(0.0))
                .bottom(px(0.0))
                .left(HIT_LEFT)
                .w(HIT_THICKNESS),
            // `after:left-0 after:h-1.5 after:w-full after:translate-x-0
            // after:-translate-y-1/2`. `top`, `bottom` and `height` are all
            // definite, which CSS resolves by ignoring `bottom`.
            Orientation::Vertical => element
                .left(px(0.0))
                .top(HIT_TOP)
                .w(relative(1.0))
                .h(HIT_THICKNESS),
        };
        match hit_background(theme, state) {
            Some(background) => element.bg(background),
            None => element,
        }
    }
}

/// `--shell-height`'s default: a definite box for the group's `height: 100%` to
/// resolve against. See [`ResizablePanelGroup::shell_height`].
pub const DEFAULT_SHELL_HEIGHT: Pixels = px(160.0);

/// The sidebar panel's growth factor in [`ResizablePanelGroup::fixture`].
///
/// `294 / (1200 − 1) × 100`, rounded to the two decimals the library's own
/// rounding keeps: the remembered 294px sidebar in a 1200px window, less the
/// separator's pixel, as a percentage of what is left.
pub const SIDEBAR_GROW: f32 = 24.52;

/// The content panel's growth factor in [`ResizablePanelGroup::fixture`].
///
/// Written as the literal the library would, rather than as `100.0 - SIDEBAR_GROW`:
/// that subtraction is `75.479996` in `f32`, and the number reaches a caption and
/// a `--help` line where a reader would have to decide whether the noise meant
/// anything. `the_fixture_is_the_live_ide_shell` holds the pair at the 100 the
/// library's layout always sums to.
pub const CONTENT_GROW: f32 = 75.48;

/// `z-10 flex h-6 w-1 shrink-0 rounded-lg bg-border` — the grip.
///
/// The only box on this component that paints a colour or a radius, and the only
/// reason its theme axis is not entirely vacuous. `rounded-lg` is **10px** here —
/// `theme.css` redefines `--radius-lg` over a `0.625rem` base — which on a 4px-wide
/// box paints as a pill; both extractors report the *computed* 10 rather than the
/// clamped used value, so they agree.
///
/// Two of its five classes are unportable and neither is a loss the oracle can
/// see: `z-10` is paint order, which `ANCHORS.md` §6 excludes as a field on
/// purpose, and the separator's `[&[aria-orientation=horizontal]>div]:rotate-90`
/// has no gpui equivalent at all — gpui's `Style` carries no transform. The
/// rotation only applies to a **vertical** group, which no call site asks for,
/// with `withHandle`, which no call site passes; so the cell it would change does
/// not exist on either side.
fn grip(theme: &Theme) -> Div {
    div()
        .flex()
        .flex_shrink_0()
        .h(GRIP_LENGTH)
        .w(GRIP_THICKNESS)
        .rounded(theme.radius_lg.value())
        .bg(theme.border)
}

/// Adds `focus-visible:ring-1`'s shadow layer in front of whatever shadows
/// `element` already carries.
///
/// A local copy of the one `dropdown_menu` writes, deliberately: the two
/// components are ported independently and a shared helper would make one
/// surface's diff reach into the other's file. The composition is the same because
/// Tailwind's is — `box-shadow: <ring>, <shadow>`, the ring first — and
/// `Styled::style` is the public seam for inserting a layer a builder method
/// cannot express.
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
        CONTENT_GROW, CONTENT_SIZED, DEFAULT_SHELL_HEIGHT, GRIP_LENGTH, GRIP_THICKNESS, GroupEntry,
        HANDLE_THICKNESS, HIT_HOVER_ALPHA, HIT_LEFT, HIT_THICKNESS, HIT_TOP, Handle, HandleState,
        ID_GROUP, ID_HANDLE, ID_HANDLE_GRIP, ID_PANEL, LINE_SIZED, Orientation, PANEL_FLEX_BASIS,
        Panel, RING_WIDTH, ResizablePanelGroup, SIDEBAR_GROW, hit_background,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own Tailwind
    /// 4.3.0 compiles the class to, at `--spacing: 0.25rem` and a 16px root.
    /// Spelled as the arithmetic rather than as more literals, so a change to the
    /// step cannot pass by leaving the literals alone.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(HIT_THICKNESS, px(STEP * 1.5)); // after:w-1.5 / after:h-1.5
        assert_eq!(GRIP_LENGTH, px(STEP * 6.0)); // h-6
        assert_eq!(GRIP_THICKNESS, px(STEP)); // w-1

        // The two that are not spacing multiples at all, and must not become
        // them: `w-px` and `h-px` are a literal 1px in Tailwind's own scale, and
        // `flex-basis: 0` is the library's inline style.
        assert_eq!(HANDLE_THICKNESS, px(1.0));
        assert_eq!(PANEL_FLEX_BASIS, px(0.0));
    }

    /// **The hit strip is centred on the separator in one orientation and half a
    /// pixel out in the other**, and that asymmetry is the reference's rather than
    /// this port's.
    ///
    /// A vertical separator gets `left-1/2` (of the host) plus `-translate-x-1/2`
    /// (of itself); a horizontal one gets `left-0` and `-translate-y-1/2` with no
    /// `top-1/2` to go with it. The arithmetic is spelled out because it is the
    /// only place this component resolves a percentage by hand.
    #[test]
    fn the_hit_strips_offsets_are_the_translate_resolved_by_hand() {
        // 0.5px (half of the 1px host) − 3px (half of the 6px strip).
        assert_eq!(HIT_LEFT, px(0.5 - 3.0));
        assert_eq!(HIT_LEFT, px(-2.5));
        // Centred on the separator's own centre: −2.5 + 3 = 0.5.
        let centre = f32::from(HIT_LEFT) + f32::from(HIT_THICKNESS) / 2.0;
        assert!(
            (centre - f32::from(HANDLE_THICKNESS) / 2.0).abs() < f32::EPSILON,
            "{centre}",
        );

        // 0px (`left-0`'s block-axis counterpart is `inset-y-0`'s `top: 0`) − 3px.
        assert_eq!(HIT_TOP, px(-3.0));
        // Centred on the separator's **top edge**, not its centre — which is the
        // half-pixel the reference is out by, reproduced rather than corrected.
        assert!(
            (f32::from(HIT_TOP) + f32::from(HIT_THICKNESS) / 2.0 - 0.0).abs() < f32::EPSILON,
            "{HIT_TOP:?}",
        );
        assert_ne!(HIT_TOP + HIT_THICKNESS, HANDLE_THICKNESS);
    }

    /// **The separator's `aria-orientation` is the opposite of its group's**, which
    /// is the one thing about this component a reader is most likely to get
    /// backwards — every `aria-[orientation=…]` variant in `resizable.tsx` is
    /// written on the separator.
    #[test]
    fn the_separators_axis_is_the_opposite_of_the_groups() {
        assert_eq!(Orientation::Horizontal.handle_axis(), Orientation::Vertical);
        assert_eq!(Orientation::Vertical.handle_axis(), Orientation::Horizontal);
        for orientation in [Orientation::Horizontal, Orientation::Vertical] {
            assert_ne!(orientation.handle_axis(), orientation);
            assert_eq!(orientation.handle_axis().handle_axis(), orientation);
        }

        assert_eq!(Orientation::Horizontal.name(), "horizontal");
        assert_eq!(Orientation::Vertical.name(), "vertical");
        assert_eq!(Orientation::default(), Orientation::Horizontal);
    }

    /// Hover is the only thing that paints anything on this component, it paints
    /// the **hit strip** rather than the separator, and focus paints nothing a
    /// background test can see.
    #[test]
    fn only_hover_paints_and_it_paints_the_strip() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(hit_background(&theme, HandleState::resting()), None);

            let focused = HandleState {
                focused: true,
                ..HandleState::resting()
            };
            assert_eq!(hit_background(&theme, focused), None);

            let hovered = HandleState {
                hovered: true,
                ..HandleState::resting()
            };
            assert_eq!(
                hit_background(&theme, hovered),
                Some(theme.border.mix(HIT_HOVER_ALPHA, Color::TRANSPARENT)),
            );
            // A 60% mix is genuinely a different colour from the token, or the
            // `/60` would be decoration.
            assert_ne!(hit_background(&theme, hovered), Some(theme.border));
        }

        // And the two themes' hover colours differ, which is what would make the
        // theme axis real here if the strip could be anchored at all.
        assert_ne!(
            hit_background(
                &Theme::LIGHT,
                HandleState {
                    hovered: true,
                    ..HandleState::resting()
                }
            ),
            hit_background(
                &Theme::DARK,
                HandleState {
                    hovered: true,
                    ..HandleState::resting()
                }
            ),
        );
    }

    /// `focus-visible:ring-1` is a **shadow**, so the separator's border stays
    /// zero — the field `ANCHORS.md` v1.1 compares *exactly*.
    #[test]
    fn the_ring_is_one_pixel_of_shadow_spread_and_not_a_border() {
        assert_eq!(RING_WIDTH, px(1.0));
    }

    /// The grip's radius is **10px**, because this project redefines
    /// `--radius-lg: var(--radius)` over a `0.625rem` base — not Tailwind's stock
    /// 8 — and it is wider than the box it rounds, which is what makes it a pill.
    #[test]
    fn the_grips_radius_is_this_projects_and_not_tailwinds() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_lg.value(), px(10.0));
            assert!(theme.radius_lg.value() > GRIP_THICKNESS);
        }
    }

    /// The fixture is the IDE shell's own group: two panels around one grip-less
    /// separator, horizontal, with growth factors that sum to the 100 the library
    /// always writes.
    #[test]
    fn the_fixture_is_the_live_ide_shell() {
        let group = ResizablePanelGroup::fixture();

        assert_eq!(group.orientation, Orientation::Horizontal);
        assert_eq!(group.shell_height, DEFAULT_SHELL_HEIGHT);
        assert_eq!(group.entries.len(), 3);
        assert!(matches!(group.entries[1], GroupEntry::Handle(_)));

        let ids: Vec<&str> = group
            .entries
            .iter()
            .map(|entry| match entry {
                GroupEntry::Panel(panel) => panel.id.as_ref(),
                GroupEntry::Handle(handle) => handle.id.as_ref(),
            })
            .collect();
        assert_eq!(
            ids,
            [
                "resize-panel-sidebar",
                "resize-handle",
                "resize-panel-content"
            ],
        );

        // The percentages the library's layout engine writes always sum to 100,
        // so a fixture whose pair did not would be describing a layout the
        // reference cannot produce. Read off the fixture rather than off the
        // constants, so the assertion is about the picture and not about two
        // numbers a compiler could fold.
        let grows: Vec<f32> = group
            .entries
            .iter()
            .filter_map(|entry| match entry {
                GroupEntry::Panel(panel) => Some(panel.grow),
                GroupEntry::Handle(_) => None,
            })
            .collect();
        assert_eq!(grows.len(), 2);
        assert!(
            (grows.iter().sum::<f32>() - 100.0).abs() < 0.001,
            "{grows:?}"
        );
        // And the sidebar is the smaller of the two, which is the shape the
        // remembered 294px default has in any window the shell opens at.
        assert!(grows[0] < grows[1], "{grows:?}");
        assert!((grows[0] - SIDEBAR_GROW).abs() < f32::EPSILON);
        assert!((grows[1] - CONTENT_GROW).abs() < f32::EPSILON);

        // The live separator has no grip and is at rest.
        let GroupEntry::Handle(handle) = &group.entries[1] else {
            unreachable!("the fixture's second entry is the separator")
        };
        assert!(!handle.with_handle);
        assert_eq!(handle.state, HandleState::resting());
    }

    /// The ids `resizable.tsx` writes when a call site names nothing, all distinct
    /// — a repeat would make two anchors one record and the differ would compare
    /// whichever arrived last.
    #[test]
    fn every_primitives_default_id_is_distinct() {
        let ids = [ID_GROUP, ID_PANEL, ID_HANDLE, ID_HANDLE_GRIP];

        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("resize-")));

        // And the fixture's two panels override the primitive's default, because
        // one group cannot carry two `resize-panel`s.
        let group = ResizablePanelGroup::fixture();
        for entry in &group.entries {
            if let GroupEntry::Panel(panel) = entry {
                assert_ne!(panel.id, ID_PANEL);
            }
        }
    }

    /// **Both declaration lists are empty**, and on this surface that follows from
    /// one fact rather than from arithmetic: nothing here paints text. v1.6
    /// requires a `font` on any anchor that declares it, and v1.5 is about a text
    /// run's max-content width.
    #[test]
    fn nothing_on_this_surface_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// A panel's growth factor is a parameter and the constructor keeps it, which
    /// is the whole of what this port models about the resize interaction.
    #[test]
    fn a_panel_keeps_the_growth_factor_it_was_given() {
        let panel = Panel::new("resize-panel-sidebar", 24.52);

        assert_eq!(panel.id, "resize-panel-sidebar");
        assert!((panel.grow - 24.52).abs() < f32::EPSILON);

        // A handle defaults to the live shape.
        let handle = Handle::new(ID_HANDLE);
        assert!(!handle.with_handle);
        assert_eq!(handle.state, HandleState::resting());
        assert_eq!(HandleState::default(), HandleState::resting());
    }
}
