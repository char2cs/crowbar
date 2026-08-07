//! The sidebar's four panels and which one is showing.
//!
//! Ported from `web/src/lib/store/sidebar.ts`'s `SidebarTab` and
//! `sidebar-carousel.tsx`'s `TABS`.
//!
//! # The order is the geometry
//!
//! The carousel is one horizontally-scrolled strip of four full-width panels,
//! so a tab's position in [`Tab::ALL`] **is** its scroll offset: panel *n*
//! sits at `n * viewport_width`. Reordering this array moves the panels on
//! screen. The React file states the same thing in a comment; here the
//! index↔offset mapping is [`Tab::index`] and [`Tab::at_offset`], so nothing
//! open-codes the multiplication.

/// One of the sidebar's four panels.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum Tab {
    /// The workspace tree. The default, and the only one Slice 1a fills.
    #[default]
    Workspaces,
    /// Agent chats. Slice 3.
    Chats,
    /// The file explorer. Slice 4.
    Files,
    /// The git panel. Slice 5.
    Git,
}

impl Tab {
    /// Every panel, in the order they sit in the carousel.
    pub const ALL: [Self; 4] = [Self::Workspaces, Self::Chats, Self::Files, Self::Git];

    /// This panel's position in the strip.
    #[must_use]
    pub const fn index(self) -> usize {
        match self {
            Self::Workspaces => 0,
            Self::Chats => 1,
            Self::Files => 2,
            Self::Git => 3,
        }
    }

    /// This panel's position as a float, for offset arithmetic.
    ///
    /// A separate accessor from [`Self::index`] so neither direction ever
    /// casts: `0.0..=3.0` is exact in `f32`, and comparing a rounded offset
    /// against these values is exact too, which an `as usize` round-trip
    /// would not be.
    const fn slot(self) -> f32 {
        match self {
            Self::Workspaces => 0.0,
            Self::Chats => 1.0,
            Self::Files => 2.0,
            Self::Git => 3.0,
        }
    }

    /// The panel a scroll offset has landed on, given the viewport width.
    ///
    /// Mirrors `Math.round(el.scrollLeft / el.clientWidth)` — nearest panel,
    /// so a half-way drag settles forward — and, like `TABS[index]`, yields
    /// nothing for an index past either end rather than clamping to a panel.
    ///
    /// Returns `None` for a zero or negative width, which is the collapsed
    /// sidebar: **no offset identifies a panel there**, and the browser has
    /// already clamped `scrollLeft` to 0. Reading it back anyway is the bug
    /// that made hiding and showing the sidebar while on Files silently land
    /// you on Chats.
    #[must_use]
    pub fn at_offset(offset: f32, viewport_width: f32) -> Option<Self> {
        if viewport_width <= 0.0 || !offset.is_finite() || !viewport_width.is_finite() {
            return None;
        }
        let index = (offset / viewport_width).round();
        // `index` is a rounded whole number and the slots are 0..=3, so a
        // half-slot margin selects exactly one panel or none — it is a
        // membership test written to satisfy `float_cmp`, not a tolerance.
        Self::ALL
            .into_iter()
            .find(|tab| (tab.slot() - index).abs() < 0.5)
    }

    /// The scroll offset that puts this panel in view.
    #[must_use]
    pub fn offset(self, viewport_width: f32) -> f32 {
        self.slot() * viewport_width
    }

    /// The panel's stable name, as the daemon's UI-preference rows spell it.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Workspaces => "workspaces",
            Self::Chats => "chats",
            Self::Files => "files",
            Self::Git => "git",
        }
    }

    /// Parse a persisted name. An unknown value falls back to the default
    /// panel rather than erroring: a preference row written by a newer build
    /// must not stop the sidebar from rendering.
    #[must_use]
    pub fn from_str_or_default(name: &str) -> Self {
        Self::ALL
            .into_iter()
            .find(|tab| tab.as_str() == name)
            .unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::Tab;
    use crate::sidebar::fixtures::assert_px;

    #[test]
    fn the_strip_order_is_the_carousel_order() {
        assert_eq!(
            Tab::ALL.map(Tab::as_str),
            ["workspaces", "chats", "files", "git"]
        );
        assert_eq!(Tab::ALL.map(Tab::index), [0, 1, 2, 3]);
    }

    #[test]
    fn workspaces_is_the_default_panel() {
        assert_eq!(Tab::default(), Tab::Workspaces);
    }

    #[test]
    fn offset_is_the_index_times_the_viewport() {
        assert_px!(Tab::Workspaces.offset(300.0), 0.0);
        assert_px!(Tab::Chats.offset(300.0), 300.0);
        assert_px!(Tab::Git.offset(300.0), 900.0);
    }

    #[test]
    fn an_offset_resolves_to_the_nearest_panel() {
        assert_eq!(Tab::at_offset(0.0, 300.0), Some(Tab::Workspaces));
        assert_eq!(Tab::at_offset(300.0, 300.0), Some(Tab::Chats));
        assert_eq!(
            Tab::at_offset(449.0, 300.0),
            Some(Tab::Chats),
            "just short of half-way stays put"
        );
        assert_eq!(
            Tab::at_offset(451.0, 300.0),
            Some(Tab::Files),
            "past half-way settles forward"
        );
    }

    /// `TABS[index]` is `undefined` past the strip, and an undefined tab does
    /// not change the active one.
    #[test]
    fn an_offset_past_the_strip_selects_nothing() {
        assert_eq!(Tab::at_offset(1500.0, 300.0), None);
        assert_eq!(Tab::at_offset(-300.0, 300.0), None);
    }

    /// A collapsed sidebar has zero width: no offset identifies a panel, and
    /// reading one back is what silently moved Files to Chats.
    #[test]
    fn a_collapsed_sidebar_identifies_no_panel() {
        assert_eq!(Tab::at_offset(0.0, 0.0), None);
        assert_eq!(Tab::at_offset(600.0, -1.0), None);
    }

    #[test]
    fn non_finite_geometry_selects_nothing() {
        assert_eq!(Tab::at_offset(f32::NAN, 300.0), None);
        assert_eq!(Tab::at_offset(0.0, f32::INFINITY), None);
    }

    #[test]
    fn a_persisted_name_round_trips() {
        for tab in Tab::ALL {
            assert_eq!(Tab::from_str_or_default(tab.as_str()), tab);
        }
    }

    /// A row written by a newer build must not stop the sidebar rendering.
    #[test]
    fn an_unknown_persisted_name_falls_back_to_the_default() {
        assert_eq!(Tab::from_str_or_default("search"), Tab::Workspaces);
        assert_eq!(Tab::from_str_or_default(""), Tab::Workspaces);
    }
}
