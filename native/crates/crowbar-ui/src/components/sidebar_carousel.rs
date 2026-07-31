//! `sidebar-carousel` — the four sidebar panels on one horizontal scroll-snap
//! track, only one of which is ever in view.
//!
//! The native half of `web/src/components/layout/sidebar-carousel.tsx`. Nothing
//! in this app overrides its classes from a stylesheet — `[data-sidebar-carousel]`
//! is a query hook for the component's own tests and carries no rule — so the
//! class lists are what renders.
//!
//! # What makes this component different from the three before it
//!
//! It paints **nothing**. No background, no border, no radius, no text, no
//! font, no colour of any kind: every class on it is layout. So there is no
//! [`Theme`](crate::Theme) argument here, and that is a statement rather than an
//! omission — a component that took one and read no token would suggest a
//! palette it does not have.
//!
//! What it does instead is **clip**. The scrollport is
//! `overflow-x-scroll overflow-y-hidden`; the four panels are each `min-w-full`,
//! so the track is four scrollports wide and three of the four panels sit
//! entirely outside the visible box at any instant. Measured live at a 600px
//! window: `scrollWidth` **1176** against `clientWidth` **294**.
//!
//! That clipping is the whole contract surface, and it lands on exactly one
//! field.
//!
//! # `visible` on a snapped-out panel is `false`. The reasoning, in full
//!
//! 1. `native/oracle/ANCHORS.md` §3 defines the field as *"Actually painted:
//!    not `display:none`, not `visibility:hidden`, non-zero area, **not fully
//!    clipped by an ancestor**."* A panel scrolled out is fully clipped by its
//!    ancestor — the scrollport — and none of it reaches the screen. There is no
//!    second reading of §3 that makes it `true`.
//! 2. **Both extractors already answer that way, unmodified.** The React side's
//!    `oracleIsVisible` walks `parentElement` upwards and intersects the
//!    anchor's box against every ancestor whose `overflow` is not `visible`; the
//!    scrollport is one. `crowbar-driver`'s `is_visible` intersects `bounds`
//!    against `window.content_mask()`, which gpui pushes in
//!    `Interactivity::prepaint` from `Style::overflow_mask` — i.e. from this
//!    component's own overflow. Neither needed a line changed to agree.
//! 3. The alternative reading — *"the panel is part of the scroll content, so it
//!    is visible"* — makes the field a claim about **reachability** rather than
//!    about paint. It would also report `true` for all four panels in every cell
//!    of the matrix, on both sides, forever: the cheapest possible fake
//!    convergence on the one field that tells a snapped carousel apart from four
//!    stacked panels.
//! 4. It gives up nothing, because `bounds` still carries the geometry. A
//!    snapped-out panel reports `x = -294` or `+294` relative to the root and is
//!    compared at ±0.5px like any other. A port with the wrong scroll offset
//!    cannot hide behind `visible: false`.
//! 5. **The tangent case is exact, and the two implementations already agree on
//!    it.** At `scrollLeft = k · W` the panel at index `k + 1` starts exactly at
//!    the scrollport's right edge: the intersection is zero-width. The DOM side
//!    requires `r - l > 0`; the driver requires `!intersect().is_empty()`. Both
//!    say `false`. So "the next panel along" is not a grey area — it is the
//!    default cell here precisely because it is the sharpest one.
//!
//! # Two things this component cannot show the differ, said plainly
//!
//! * **Scroll snapping itself.** gpui has no `scroll-snap-type` and no
//!   `scroll-snap-align`. What the port reproduces is not the snap engine but
//!   the *snapped position*, and it is entitled to: `sidebar-carousel.tsx`
//!   computes those itself, in both of its effects —
//!   `el.scrollLeft = index * el.clientWidth` in the `ResizeObserver` and
//!   `el.scrollTo({ left: index * el.clientWidth })` on an `activeTab` change.
//!   The browser's snapping only catches a swipe that ended *between* panels,
//!   and `ANCHORS.md` §6 already puts "a snapshot is one instant" beyond the
//!   contract.
//! * **What is inside a panel.** The four panels hold `WorkspaceTree`,
//!   `AgentChatsPanel`, `FileExplorerTree` and `GitPanel`; this port renders
//!   them **empty**, the same call the three components before it made about
//!   icons. That is only legitimate because every panel is `min-w-full` and the
//!   track always overflows, so flexbox freezes all four at exactly the
//!   scrollport's width whatever their content wants — see
//!   [`SidebarCarousel::panel_content_width`], which exists so a reader can
//!   *drive* that rather than take it on trust.

use gpui::{AnyElement, Div, Overflow, ParentElement as _, Pixels, Styled as _, div, px, relative};

use super::anchor::{AnchorId, AnchorSink};

/// The root anchor: the scroll container itself, whose visible box is the
/// scrollport every panel is clipped against. Every other bound on this surface
/// is reported relative to it (`native/oracle/ANCHORS.md` §4).
pub const ID_SCROLLPORT: &str = "carousel-scrollport";

/// The `workspaces` panel — `<WorkspaceTree />`.
pub const ID_PANEL_WORKSPACES: &str = "carousel-panel-workspaces";
/// The `chats` panel — `<AgentChatsPanel />`.
pub const ID_PANEL_CHATS: &str = "carousel-panel-chats";
/// The `files` panel — `<FileExplorerTree />` under an `ErrorBoundary` and a
/// `Suspense`.
pub const ID_PANEL_FILES: &str = "carousel-panel-files";
/// The `git` panel — `<GitPanel />`.
pub const ID_PANEL_GIT: &str = "carousel-panel-git";

/// The anchors whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and here that is not even a close call: nothing on this surface
/// paints text at all, so there is no max-content width for gpui to `ceil()`.
/// Written down as an empty list rather than left out, because the React side's
/// (also empty) declaration set has to be diffable against it — a
/// `data-oracle-content-sized` on one side only is a `FieldPresence` delta that
/// forgives nothing.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose box height **is** their own line box
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None**, for the same reason. v1.6's own warning is that "has text" does not
/// mean "is line-sized"; the weaker statement holds here — nothing has text, and
/// every box on this surface takes its height from the flex container rather
/// than from a line box.
pub const LINE_SIZED: [&str; 0] = [];

/// How many panels the track carries.
///
/// Four, and fixed: `TABS` in `sidebar-carousel.tsx` is a module constant and
/// every panel is rendered unconditionally. That is why `empty` is not a state
/// this surface has.
pub const PANEL_COUNT: usize = 4;

/// One of the four sidebar tabs, in the order `TABS` lists them.
///
/// The order is load-bearing twice over: it is the DOM order of the panels, and
/// it is the multiplier in `scrollLeft = index * clientWidth`. The React
/// component's own test calls an off-by-one here out by name.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum SidebarTab {
    /// `<WorkspaceTree />`. Index 0, and `getInitialState()`'s `activeTab` — so
    /// this is where a freshly mounted carousel rests.
    #[default]
    Workspaces,
    /// `<AgentChatsPanel />`.
    Chats,
    /// `<FileExplorerTree />`.
    Files,
    /// `<GitPanel />`.
    Git,
}

/// The four tabs in DOM order — `TABS` in `sidebar-carousel.tsx`.
pub const TABS: [SidebarTab; PANEL_COUNT] = [
    SidebarTab::Workspaces,
    SidebarTab::Chats,
    SidebarTab::Files,
    SidebarTab::Git,
];

impl SidebarTab {
    /// The word `SidebarTab` uses in the store, and the one the command line
    /// takes.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Workspaces => "workspaces",
            Self::Chats => "chats",
            Self::Files => "files",
            Self::Git => "git",
        }
    }

    /// The anchor this panel carries, on both sides.
    #[must_use]
    pub const fn anchor(self) -> &'static str {
        match self {
            Self::Workspaces => ID_PANEL_WORKSPACES,
            Self::Chats => ID_PANEL_CHATS,
            Self::Files => ID_PANEL_FILES,
            Self::Git => ID_PANEL_GIT,
        }
    }

    /// `TABS.indexOf(tab)` — the panel's place on the track, which is also the
    /// multiplier in `scrollLeft = index * clientWidth`.
    ///
    /// A `u8` rather than a `usize` so the arithmetic below can reach `f32`
    /// through `From` instead of through a cast.
    #[must_use]
    pub const fn index(self) -> u8 {
        match self {
            Self::Workspaces => 0,
            Self::Chats => 1,
            Self::Files => 2,
            Self::Git => 3,
        }
    }

    /// Whether this panel's `<div>` carries `h-full`.
    ///
    /// **The first two do and the last two do not**, and this is copied out of
    /// the JSX rather than tidied: it looks like an inconsistency and it is one,
    /// but it is *inert* — a flex item in a row with `align-items: stretch`
    /// already fills the container's height, and `height: 100%` resolves against
    /// the same box. Reproducing it and pinning the equality with a layout test
    /// is how the port shows that, instead of asserting it.
    #[must_use]
    pub const fn full_height(self) -> bool {
        matches!(self, Self::Workspaces | Self::Chats)
    }

    /// The tab a word names.
    #[must_use]
    pub fn parse(raw: &str) -> Option<Self> {
        TABS.into_iter().find(|tab| tab.name() == raw)
    }

    /// The scroll offset this tab snaps to, **as a fraction of the scrollport's
    /// width**, negated so it can be spent as a leading margin.
    ///
    /// A fraction rather than a number of pixels because that is what the
    /// quantity *is*: `el.scrollLeft = index * el.clientWidth`. Expressed as a
    /// percentage margin it needs no width to be passed in, resolves against the
    /// same box CSS resolves it against, and cannot drift out of step with a
    /// `--width` the surface was drawn at.
    #[must_use]
    pub fn scroll_fraction(self) -> f32 {
        -f32::from(self.index())
    }
}

/// The sidebar carousel, at one scroll position.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SidebarCarousel {
    /// Which panel is snapped into the scrollport.
    ///
    /// **Visual state as a parameter, never a gpui scroll handle.**
    /// `ANCHORS.md` §6 makes runtime interaction state invisible to the
    /// extractor, and gpui's own horizontal scrolling lives on
    /// `StatefulInteractiveElement` — it needs an element id and a
    /// `ScrollHandle` that a wheel event can move and that gpui re-clamps every
    /// frame. A surface whose defining quantity lived there would be one a
    /// snapshot could not pin.
    pub active: SidebarTab,
    /// The max-content width of whatever each panel holds.
    ///
    /// Zero — the default — renders the panels **empty**, which is what this
    /// port does: the four React panels are whole feature subtrees with no
    /// native equivalent, and drawing substitutes would put shapes on screen for
    /// the oracle to converge on.
    ///
    /// It is drivable rather than assumed because "the panels' boxes do not
    /// depend on their contents" is the claim that licenses rendering them
    /// empty, and it is a claim about *flexbox* rather than about this
    /// component: all four panels are `min-w-full`, so `Σ min-width` is four
    /// scrollports against one scrollport of space, every item is frozen at its
    /// minimum whatever its flex base size was, and the result is the same track
    /// for an empty panel and a 4000px-wide one. Driving it is how that stops
    /// being an argument and becomes a measurement.
    pub panel_content_width: Pixels,
}

impl Default for SidebarCarousel {
    fn default() -> Self {
        Self {
            // `getInitialState()` in `web/src/lib/store/sidebar.ts`.
            active: SidebarTab::Workspaces,
            panel_content_width: px(0.0),
        }
    }
}

impl SidebarCarousel {
    /// The carousel showing one tab, with empty panels — the shape the port
    /// draws.
    #[must_use]
    pub fn showing(active: SidebarTab) -> Self {
        Self {
            active,
            ..Self::default()
        }
    }

    /// Renders the scrollport and its four panels, opting every contract anchor
    /// into `anchors`.
    ///
    /// **The element this returns has no height of its own.** `flex-1` is the
    /// only block-axis declaration the class list carries, so the carousel takes
    /// its height from the column it is in — `NavStack`'s inner layer in the app,
    /// and the surface's own host in a parity run. Reproducing the declaration
    /// rather than resolving it here is the same call `dropdown-menu` made about
    /// `w-(--anchor-width)`: a taffy that resolved `flex-1` differently from
    /// `WebKit` should show up, not be pre-empted.
    #[must_use]
    pub fn render(&self, anchors: &dyn AnchorSink) -> AnyElement {
        let panels: Vec<AnyElement> = TABS
            .into_iter()
            .map(|tab| anchors.boxed(AnchorId::from(tab.anchor()), self.panel(tab)))
            .collect();
        anchors.root(AnchorId::from(ID_SCROLLPORT), scrollport().children(panels))
    }

    /// One panel: `min-w-full [scroll-snap-align:start] flex flex-col
    /// overflow-hidden`, plus `h-full` on the two that carry it.
    ///
    /// The **first** panel additionally carries the scroll offset as a negative
    /// leading margin. See [`SidebarCarousel::active`] and the mapping table:
    /// `scrollLeft` has no element to live on in a declarative tree, and a
    /// negative margin on the first flex item shifts it and every sibling after
    /// it by exactly the same amount, which is what a scroll offset does.
    fn panel(self, tab: SidebarTab) -> Div {
        let mut element = div()
            // `min-w-full`. The floor that freezes every panel at the
            // scrollport's width, whatever its content wants.
            .min_w_full()
            .flex()
            .flex_col()
            .overflow_hidden();
        if tab.full_height() {
            element = element.h_full();
        }
        if tab.index() == 0 {
            element = element.ml(relative(self.active.scroll_fraction()));
        }
        if self.panel_content_width > px(0.0) {
            element = element.child(div().w(self.panel_content_width));
        }
        element
    }
}

/// The scroll container: `flex flex-1 overflow-x-scroll overflow-y-hidden
/// [scroll-snap-type:x_mandatory] [scrollbar-width:none]
/// [&::-webkit-scrollbar]:hidden`.
///
/// The two overflow axes are set through [`Styled::style`] rather than through
/// `overflow_hidden()`, because they are genuinely two different values and this
/// is the one component where saying so matters. gpui's `overflow_x_scroll()` is
/// not reachable: it lives on `StatefulInteractiveElement` and would drag in an
/// element id and a `ScrollHandle`. The *style* is what both consumers read —
/// taffy zeroes the automatic minimum size for `Scroll` and `Hidden` alike, and
/// `Style::overflow_mask` produces the same mask for either — so the seam gives
/// the declaration without the machinery.
///
/// `scrollbar-width: none` needs nothing: gpui's `Style::scrollbar_width`
/// defaults to `0px`, so taffy reserves no gutter, which is exactly what the
/// class asks `WebKit` for.
fn scrollport() -> Div {
    let mut element = div().flex().flex_1();
    let style = element.style();
    style.overflow.x = Some(Overflow::Scroll);
    style.overflow.y = Some(Overflow::Hidden);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ID_PANEL_CHATS, ID_PANEL_FILES, ID_PANEL_GIT, ID_PANEL_WORKSPACES,
        ID_SCROLLPORT, LINE_SIZED, PANEL_COUNT, SidebarCarousel, SidebarTab, TABS,
    };
    use gpui::px;

    /// The DOM order of the panels, which is also the multiplier in
    /// `scrollLeft = index * clientWidth`. The React component's own test names
    /// an off-by-one here as the defect it guards; this is the same guard on
    /// this side.
    #[test]
    fn the_track_is_the_four_tabs_in_dom_order() {
        assert_eq!(TABS.len(), PANEL_COUNT);
        assert_eq!(
            TABS.map(SidebarTab::name),
            ["workspaces", "chats", "files", "git"],
        );

        for (position, tab) in TABS.into_iter().enumerate() {
            let expected = u8::try_from(position).expect("four tabs fit in a u8");
            assert_eq!(tab.index(), expected, "{}", tab.name());
            // The offset is the index, negated — spelled as the identity rather
            // than as a second table, so the two cannot drift.
            assert!(
                (tab.scroll_fraction() + f32::from(expected)).abs() < f32::EPSILON,
                "{}",
                tab.name(),
            );
        }
    }

    /// Every word round-trips, and nothing else is a tab.
    #[test]
    fn the_tab_vocabulary_is_closed() {
        for tab in TABS {
            assert_eq!(SidebarTab::parse(tab.name()), Some(tab));
        }
        for word in ["", "workspace", "chat", "file", "diff", "Files"] {
            assert_eq!(SidebarTab::parse(word), None, "{word}");
        }
    }

    /// One anchor per panel, all distinct, all distinct from the root — two
    /// panels sharing an id would make one record and the differ would compare
    /// whichever arrived last.
    #[test]
    fn every_panel_carries_its_own_anchor() {
        let mut ids: Vec<&str> = TABS.into_iter().map(SidebarTab::anchor).collect();
        ids.push(ID_SCROLLPORT);

        let before = ids.len();
        ids.sort_unstable();
        ids.dedup();
        assert_eq!(ids.len(), before, "{ids:?}");
        assert_eq!(before, PANEL_COUNT + 1);

        assert_eq!(SidebarTab::Workspaces.anchor(), ID_PANEL_WORKSPACES);
        assert_eq!(SidebarTab::Chats.anchor(), ID_PANEL_CHATS);
        assert_eq!(SidebarTab::Files.anchor(), ID_PANEL_FILES);
        assert_eq!(SidebarTab::Git.anchor(), ID_PANEL_GIT);
        assert!(ids.iter().all(|id| id.starts_with("carousel-")));
    }

    /// `h-full` is on the first two panels and not on the last two — a fact
    /// about the JSX, reproduced rather than tidied. That it changes nothing is
    /// a *layout* claim and is measured in `row_layout.rs`; what this pins is
    /// that the port did not quietly normalise the four.
    #[test]
    fn h_full_is_on_the_first_two_panels_only() {
        assert_eq!(
            TABS.map(SidebarTab::full_height),
            [true, true, false, false],
        );
    }

    /// A fresh carousel rests on `workspaces` with empty panels — the store's
    /// `getInitialState()`, and this port's rendering of the four subtrees it
    /// has no native equivalent for.
    #[test]
    fn a_fresh_carousel_rests_where_the_store_starts() {
        let carousel = SidebarCarousel::default();

        assert_eq!(carousel.active, SidebarTab::Workspaces);
        assert_eq!(carousel.panel_content_width, px(0.0));
        assert!((carousel.active.scroll_fraction()).abs() < f32::EPSILON);
        assert_eq!(SidebarTab::default(), SidebarTab::Workspaces);

        // And `showing` moves only the tab.
        let files = SidebarCarousel::showing(SidebarTab::Files);
        assert_eq!(files.active, SidebarTab::Files);
        assert_eq!(files.panel_content_width, px(0.0));
        assert!((files.active.scroll_fraction() + 2.0).abs() < f32::EPSILON);
    }

    /// **Both declaration lists are empty, and on this surface that is not even
    /// a judgement call**: v1.5 is about a text run's max-content width and v1.6
    /// about a line box, and nothing here paints text.
    #[test]
    fn nothing_on_this_surface_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }
}
