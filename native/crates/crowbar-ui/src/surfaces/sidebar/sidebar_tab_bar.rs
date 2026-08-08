//! `sidebar_tab_bar` — the sidebar's own padded wrapper around the already-
//! ported `Tabs` primitive, with its own four-tab set and its own container
//! query deciding which labels show.
//!
//! The native half of `web/src/components/layout/sidebar-tab-bar.tsx`, which
//! is `@container flex shrink-0 items-center px-2 py-1.5` around one
//! `<Tabs>`. Every value below came out of the app's own `tailwindcss` 4.3.0
//! with the utility as a candidate — the method `native/MAPPING.md` fixes.
//! See `native/mapping/sidebar-tab-bar.md`.
//!
//! # This is the call site `tabs.rs` already captured a reference from —
//! ported here as its own, separate wrapper
//!
//! `crowbar_ui::components::tabs::Tabs::fixture`'s own doc comment names this
//! exact file as the reference: *"the live sidebar tab bar, as
//! `web/src/components/layout/sidebar-tab-bar.tsx` renders it on the home
//! route."* `native/mapping/tabs.md` §9 carries the captured numbers
//! (`tabs 0,0,278,36`, three tabs at `2,2,90,32`/`94,2,90,32`/`186,2,90,32`,
//! the indicator on the first). **That capture already covers the `Tabs`
//! primitive's own geometry inside this wrapper** — this file's own job is
//! the padded box *around* it (`px-2 py-1.5`, `@container`, `shrink-0`) and
//! the tab set/label logic that call site supplies, which `tabs.rs`
//! deliberately leaves to its caller. Nothing here re-measures what `tabs.rs`
//! already measured; §1 shows the arithmetic agrees.
//!
//! # 1. The wrapper's own padding is why the captured `Tabs` root is 278px
//!
//! `tabs.md`'s capture was taken at a 294px sidebar column (unstated in that
//! file, recovered here): `Tabs` is `w-full` of whatever `sidebar-tab-bar.tsx`
//! leaves it, and this wrapper's `px-2` subtracts 8px off *each* side —
//! `294 − 2 × 8 = 278`, exactly the captured width. [`SidebarTabBar::render`]
//! reproduces that arithmetic rather than asserting the 278 as a constant of
//! its own, so a caller driving a different column width gets a `Tabs` root
//! that is still correctly `column_width − 2 × PADDING_X`.
//!
//! # 2. The container query, derived rather than read off the class names
//!
//! `@[280px]:inline` / `@[420px]:inline` query the `@container`'s own
//! **content-box** width — the wrapper's outer width minus its own `px-2`
//! padding, per `container-type: inline-size`'s definition. That content-box
//! width is the *same* quantity `Tabs`' own `w-full` resolves to (§1), so
//! `SidebarTabBar` computes each tab's label visibility off the identical
//! number it hands `Tabs` as its width — not off the wrapper's raw outer
//! width, which would be 16px too generous on both breakpoints. `tabs.md`'s
//! own capture is the confirming data point: at a 278px content box (< 280),
//! every label — including the active tab's — is `display: none`, measured
//! live.
//!
//! # 3. `include_git`, a real geometry-affecting axis this file owns — and
//! three findings from the P3.52 gate that caught a fixture bug in it
//! (2026-08-03)
//!
//! `visibleTabs = isHomeRoute ? TABS.filter(t => t.tab !== 'git') : TABS` is
//! route-derived state — Phase 4's shape, out of this item's reach — but
//! *which tab set renders* is genuine geometry: three tabs against four moves
//! every tab's width and the indicator's reach. [`SidebarTabBar::include_git`]
//! takes it as a **value**, the same way `layout-denominator.md` §6 already
//! treats a prop-driven boolean as this port's business while the `useEffect`
//! that *sets* it stays someone else's — three findings sharpen that account.
//!
//! **Finding 1 — a real port defect, caught only by the integrated gate.**
//! [`SidebarTabBar::fixture`] shipped with `include_git: true` while reusing
//! `tabs.md`'s 294px/278px capture, which was taken on the home route
//! (`include_git: false`): a combination the live app never produces at
//! once. `sidebar_tab_bar`'s own tests all passed in isolation, because the
//! one assertion that catches it —
//! `the_built_tabs_value_matches_the_primitives_own_fixture` (this file's
//! own tests) — compares against `tabs::Tabs::fixture()`, a *different,
//! already-merged*
//! cluster's surface. Three isolated gates could not have found this; the
//! integrated one did. Fixed by setting the fixture to `include_git: false`
//! and adding `the_fixture_is_the_home_routes_three_tabs` as a direct
//! regression guard, rather than trusting the equality test alone to keep
//! catching it.
//!
//! **Finding 2 — the `useEffect` stays out of scope, decided rather than
//! assumed.** Landing on the home route while `git` is active reassigns
//! `activeTab` to `workspaces` — a state *transition*, not a value. This
//! port models the **result** ([`SidebarTabBar::active`] and
//! [`SidebarTabBar::include_git`] as independent fields a caller sets
//! directly) and not the imperative correction that produces it, the same
//! split every other `useEffect`-set prop in this tree already takes. One
//! consequence, left unguarded on purpose: `SidebarTabBar { active: "git",
//! include_git: false, .. }` is constructible and renders with no tab
//! selected — [`SidebarTabBar::tabs`] filters `"git"` out of the set before
//! matching `active` against it, so nothing is. That is the same picture a
//! single, corrected-next-frame React render would paint; a geometry port
//! has no more reason to forbid it than to forbid any other transient input.
//!
//! **Finding 3 — no cross-surface inconsistency with `sidebar_carousel`,
//! checked rather than assumed.** `sidebar-carousel.tsx` keeps its own
//! `TABS` constant — four entries, no `isHomeRoute` filter anywhere — and
//! [`crate::surfaces::sidebar::sidebar_carousel`]'s already-merged port matches it
//! exactly: `PANEL_COUNT = 4`, unconditional. That is not two ported
//! surfaces reading the same route condition two different ways; it is two
//! *different* React originals with a deliberately different shape (the tab
//! strip hides the affordance on `/home`, the carousel keeps every panel
//! mounted regardless), each port faithful to its own. The asymmetry a live
//! capture shows — three tabs beside four panels, both correct — is the
//! app's own design, confirmed by reading both sources rather than
//! inferred from one.
//!
//! **Why this stays a `crowbar-ui` field rather than becoming its own
//! `--surface`.** This file's own wrapper `<div>` carries no
//! `data-oracle-id` (see [`ID_SIDEBAR_TAB_BAR`]'s doc comment) — every
//! anchor a rendered `SidebarTabBar` carries is `tabs::ID_ROOT`'s subtree,
//! already registered as `--surface tabs`. `crowbar-app/src/surface.rs`'s
//! own registry test forbids two surfaces sharing a root
//! (`every_registered_surface_has_its_own_name_and_root`), and minting a
//! second, unused id for this wrapper's own root would be exactly the
//! fabricated-anchor move `ANCHORS.md` and every module in this crate
//! already refuse. It does not cost oracle coverage: `--surface tabs
//! --tabs workspaces,chats,files` and `--tabs workspaces,chats,files,git`
//! already drive the identical three-tab/four-tab geometry this axis
//! produces, generically rather than route-flavoured. What `tabs`' own
//! surface cannot reach — this wrapper's `px-2` arithmetic and
//! `@container` breakpoint ladder (§1–2) — is a pre-existing, separate scoping
//! call (`native/mapping/layout-denominator.md` §8 Cluster 3: no reference,
//! reuse `tabs.md`'s capture) that this fixture bug did not change.

use gpui::{
    AnyElement, IntoElement as _, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
};

use crate::anchor::AnchorSink;
use crate::icon::IconName;
use crate::primitives::tabs::{ListBackground, Orientation, Tab, TabSizing, Tabs, Variant};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::Theme;

/// **This anchor never renders.** `sidebar-tab-bar.tsx`'s own wrapper `<div>`
/// carries no `data-oracle-id` — only the `Tabs` root inside it does
/// (`tabs::ID_ROOT`, already captured — see the module docs). Kept as a name
/// for the caption/`--help` machinery only; `SidebarTabBar::render` returns
/// the *unwrapped* `Tabs` element directly rather than opting a second,
/// fabricated root into `anchors`.
pub const ID_SIDEBAR_TAB_BAR: &str = "sidebar-tab-bar";

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `px-2` on the wrapper.
pub const PADDING_X: Pixels = px(SPACING * 2.0);

/// `py-1.5` on the wrapper.
pub const PADDING_Y: Pixels = px(SPACING * 1.5);

/// `@[280px]:inline` — the container width at which the **active** tab's
/// label starts showing.
pub const LABEL_ACTIVE_BREAKPOINT: Pixels = px(280.0);

/// `@[420px]:inline` — the container width at which **every** tab's label
/// shows.
pub const LABEL_ALL_BREAKPOINT: Pixels = px(420.0);

/// The four tabs, in DOM order, as `(value, label)`.
pub const ALL_TABS: [(&str, &str); 4] = [
    ("workspaces", "Workspaces"),
    ("chats", "Chats"),
    ("files", "Files"),
    ("git", "Git"),
];

/// The wrapper.
#[derive(Clone, Debug, PartialEq)]
pub struct SidebarTabBar {
    /// The active tab's value — `useSidebarStore`'s `activeTab`.
    pub active: SharedString,
    /// `!isHomeRoute` — whether the `git` tab is in the set at all. A real
    /// geometry axis; see the module docs.
    pub include_git: bool,
    /// The column this wrapper is drawn in — its own outer width, before
    /// `PADDING_X` is subtracted.
    pub column_width: Pixels,
    /// Which side of the `sm` (640px) **viewport** breakpoint decides `Tabs`'
    /// own tab height/icon size. Real Crowbar windows are always ≥ 640px —
    /// the same posture `tabs.rs`'s own fixture takes.
    pub viewport_breakpoint: Breakpoint,
}

impl SidebarTabBar {
    /// The live sidebar tab bar **on the home route** — the same cell
    /// `tabs::Tabs::fixture()` documents as its own reference, so the two
    /// must agree exactly (`the_built_tabs_value_matches_the_primitives_own_fixture`
    /// is what holds them equal).
    ///
    /// `include_git: false` here is not a default guess; it is the specific
    /// condition the 294px column (`tabs.md` §1, and this fixture's own
    /// `column_width`) was captured under. `isHomeRoute` is what filters
    /// `git` out of `sidebar-tab-bar.tsx`'s own `visibleTabs` — true on the
    /// home route, false everywhere else — and setting `include_git: true`
    /// while keeping the home route's 278px-content-box geometry would
    /// assert a combination the live app never produces at once. Confirmed
    /// live (2026-08-03, an unrelated capture): the tab strip showed three
    /// tabs while a *separate* component, `sidebar-carousel.tsx`, mounted
    /// four panels including `git` alongside it — that carousel keeps its
    /// own unconditional `TABS` constant, so its panel count says nothing
    /// about this wrapper's own tab count; the two are independently
    /// defined and only usually agree. `useSidebarStore.getInitialState()`
    /// rests on `workspaces`, which is why that is the active tab here too.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            active: "workspaces".into(),
            include_git: false,
            column_width: px(294.0),
            viewport_breakpoint: Breakpoint::Sm,
        }
    }

    /// The tab set this cell renders: three without `git`, four with it.
    #[must_use]
    pub fn tab_values(&self) -> Vec<&'static str> {
        ALL_TABS
            .iter()
            .filter(|(value, _)| self.include_git || *value != "git")
            .map(|(value, _)| *value)
            .collect()
    }

    /// The `@container`'s own content-box width — the quantity both the
    /// container query and `Tabs`' `w-full` resolve against. See the module
    /// docs §1–2.
    #[must_use]
    pub fn container_width(&self) -> Pixels {
        self.column_width - PADDING_X * 2.0
    }

    /// Whether a tab shows its label at this cell's container width:
    /// `@[420px]:inline` for every tab, `@[280px]:inline` for the active one
    /// only, below 280 for none.
    #[must_use]
    pub fn shows_label(&self, value: &str) -> bool {
        let width = self.container_width();
        if width >= LABEL_ALL_BREAKPOINT {
            true
        } else if width >= LABEL_ACTIVE_BREAKPOINT {
            self.active == value
        } else {
            false
        }
    }

    /// The `Tabs` value this wrapper composes — `crowbar_ui::components::tabs`
    /// already owns every one of its own anchors; this method only decides
    /// which tabs exist and which carry a label.
    /// The glyph each tab draws — `sidebar-tab-bar.tsx:3`'s own imports.
    ///
    /// `git` falls back to its own branch mark rather than to nothing: an
    /// unknown value here would render an empty square, which is the failure
    /// `crate::icon` exists to end.
    ///
    /// `selected` picks the weight, because the call site does:
    /// `<Icon size={14} weight={isActive ? 'fill' : 'regular'} />`. Shipping
    /// the regular artwork for the active tab drew a hollow grid where the
    /// app draws a solid one — invisible to every anchor field there is.
    #[must_use]
    pub fn glyph(value: &str, selected: bool) -> IconName {
        match (value, selected) {
            ("chats", false) => IconName::ChatsCircle,
            ("chats", true) => IconName::ChatsCircleFill,
            ("files", false) => IconName::FolderOpen,
            ("files", true) => IconName::FolderOpenFill,
            ("git", false) => IconName::GitBranch,
            ("git", true) => IconName::GitBranchFill,
            (_, false) => IconName::SquaresFour,
            (_, true) => IconName::SquaresFourFill,
        }
    }

    #[must_use]
    pub fn tabs(&self) -> Tabs {
        let tabs = self
            .tab_values()
            .into_iter()
            .map(|value| {
                let mut tab = Tab::new(value);
                tab.selected = self.active == value;
                tab.label = self.shows_label(value).then(|| value.into());
                tab.icon = Some(Self::glyph(value, tab.selected));
                tab
            })
            .collect();

        Tabs {
            orientation: Orientation::Horizontal,
            variant: Variant::Default,
            list_background: ListBackground::SidebarElementIdle,
            tab_sizing: TabSizing::Fill,
            breakpoint: self.viewport_breakpoint,
            tabs,
            panel: None,
        }
    }

    /// The wrapper's own box, sized to [`Self::column_width`] rather than
    /// `w-full`'s abstract "whatever the parent leaves" — the concrete number
    /// a caller (a surface, or eventually `IDEShell`) has to supply either
    /// way.
    fn shell(&self) -> gpui::Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .w(self.column_width)
            .px(PADDING_X)
            .py(PADDING_Y)
    }

    /// Renders the wrapper. **No anchor of its own** — see
    /// [`ID_SIDEBAR_TAB_BAR`]'s doc comment — so every anchor this element
    /// carries is `tabs.rs`'s own, at the offset this wrapper's padding
    /// produces.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.shell()
            .child(self.tabs().render(theme, anchors))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        LABEL_ACTIVE_BREAKPOINT, LABEL_ALL_BREAKPOINT, PADDING_X, PADDING_Y, SidebarTabBar,
    };
    use crate::IconName;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;
        assert_eq!(PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(PADDING_Y, px(STEP * 1.5)); // py-1.5
        // The two container-query breakpoints are literal pixel values in
        // the class names themselves, not spacing multiples.
        assert_eq!(LABEL_ACTIVE_BREAKPOINT, px(280.0));
        assert_eq!(LABEL_ALL_BREAKPOINT, px(420.0));
    }

    /// **The captured reference's own arithmetic**: a 294px column minus
    /// `px-2` twice is 278px — `tabs.md`'s own captured `Tabs` width, and the
    /// live cell in which every label is `display: none`.
    #[test]
    fn the_fixtures_container_width_reproduces_the_captured_278() {
        let bar = SidebarTabBar::fixture();
        assert_eq!(bar.container_width(), px(278.0));
        assert!(bar.container_width() < super::LABEL_ACTIVE_BREAKPOINT);
        for (value, _) in super::ALL_TABS {
            assert!(!bar.shows_label(value), "{value}");
        }
    }

    /// **P3.52 gate finding (2026-08-03):** the fixture's `include_git` is
    /// `false`, not `true` — the reverse of an earlier draft. A live capture
    /// on the home route showed the tab strip rendering three tabs
    /// (`workspaces`/`chats`/`files`, no `git`) at the same time a *different*
    /// component, `sidebar-carousel.tsx`, mounted four panels including
    /// `git`; that carousel keeps its own unconditional tab list and is not
    /// evidence about this wrapper's own count. `tabs::Tabs::fixture()`'s own
    /// doc comment already said as much — "three tabs... on the home route"
    /// — before this test existed to hold the two fixtures to it.
    #[test]
    fn the_fixture_is_the_home_routes_three_tabs() {
        assert!(!SidebarTabBar::fixture().include_git);
        assert_eq!(
            SidebarTabBar::fixture().tab_values(),
            ["workspaces", "chats", "files"]
        );
    }

    /// The three-way label ladder, at its own boundaries.
    #[test]
    fn labels_appear_in_the_order_the_container_query_promises() {
        let narrow = SidebarTabBar {
            column_width: px(278.0 + f32::from(super::PADDING_X) * 2.0),
            ..SidebarTabBar::fixture()
        };
        assert!(!narrow.shows_label("workspaces"));

        let mid = SidebarTabBar {
            column_width: px(300.0),
            ..SidebarTabBar::fixture()
        };
        assert!(mid.container_width() >= LABEL_ACTIVE_BREAKPOINT);
        assert!(mid.container_width() < LABEL_ALL_BREAKPOINT);
        assert!(mid.shows_label("workspaces")); // the active tab
        assert!(!mid.shows_label("chats")); // not active

        let wide = SidebarTabBar {
            column_width: px(450.0),
            ..SidebarTabBar::fixture()
        };
        assert!(wide.container_width() >= LABEL_ALL_BREAKPOINT);
        for (value, _) in super::ALL_TABS {
            if value == "git" {
                continue; // excluded from the tab set by include_git
            }
            assert!(wide.shows_label(value), "{value}");
        }
    }

    /// `include_git` decides the anchor **set**, name for name. Both states
    /// are driven explicitly here rather than trusting the fixture's own
    /// default for either — the fixture's `include_git: false` is itself a
    /// tested fact ([`the_fixture_is_the_home_routes_three_tabs`]), not an
    /// assumption this test should also lean on.
    #[test]
    fn include_git_adds_or_removes_the_fourth_tab() {
        let with_git = SidebarTabBar {
            include_git: true,
            ..SidebarTabBar::fixture()
        };
        assert_eq!(
            with_git.tab_values(),
            ["workspaces", "chats", "files", "git"]
        );

        let without_git = SidebarTabBar {
            include_git: false,
            ..SidebarTabBar::fixture()
        };
        assert_eq!(without_git.tab_values(), ["workspaces", "chats", "files"]);

        let built = without_git.tabs();
        assert_eq!(built.tabs.len(), 3);
        assert!(built.tabs.iter().all(|tab| tab.value != "git"));
    }

    /// The **selected** tab draws the fill weight and every other tab draws
    /// the regular one — `weight={isActive ? 'fill' : 'regular'}`.
    ///
    /// Asserted on the glyphs themselves rather than on a fixture equality,
    /// because a fixture comparison passes just as happily when both sides are
    /// wrong together. That is exactly how this shipped: the port drew the
    /// regular grid for the active tab, and the fixture it was compared
    /// against drew the regular grid too.
    #[test]
    fn the_selected_tab_draws_the_fill_weight_and_the_others_do_not() {
        for (value, regular, fill) in [
            (
                "workspaces",
                IconName::SquaresFour,
                IconName::SquaresFourFill,
            ),
            ("chats", IconName::ChatsCircle, IconName::ChatsCircleFill),
            ("files", IconName::FolderOpen, IconName::FolderOpenFill),
            ("git", IconName::GitBranch, IconName::GitBranchFill),
        ] {
            assert_eq!(SidebarTabBar::glyph(value, false), regular, "{value} idle");
            assert_eq!(SidebarTabBar::glyph(value, true), fill, "{value} active");
            assert_ne!(regular, fill, "{value}: the two weights must be two files");
        }

        // And through the built value, so the wiring is covered and not just
        // the mapping: exactly the active tab gets a fill glyph.
        let built = SidebarTabBar::fixture().tabs();
        for tab in &built.tabs {
            let expected = SidebarTabBar::glyph(&tab.value, tab.selected);
            assert_eq!(tab.icon, Some(expected), "{}", tab.value);
        }
        assert_eq!(
            built
                .tabs
                .iter()
                .filter(|tab| tab.icon == Some(IconName::SquaresFourFill))
                .count(),
            1,
        );
    }

    /// The built `Tabs` value carries the sidebar's own overrides — matching
    /// `tabs::Tabs::fixture`'s own documented shape exactly, since that
    /// fixture *is* this call site.
    #[test]
    fn the_built_tabs_value_matches_the_primitives_own_fixture() {
        let built = SidebarTabBar::fixture().tabs();
        let reference = crate::primitives::tabs::Tabs::fixture();
        assert_eq!(built, reference);
    }
}
