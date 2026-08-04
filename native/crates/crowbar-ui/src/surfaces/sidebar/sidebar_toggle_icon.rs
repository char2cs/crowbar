//! `sidebar_toggle_icon` — the panel glyph, and the one component in this group
//! that **authors its own box on purpose**.
//!
//! The native half of `web/src/components/ui/sidebar-toggle-icon.tsx`: one
//! `<svg>` with a `24 × 24` viewBox, a rounded `<rect>`, a divider and two rail
//! lines, all stroked at 2px in `currentColor` with `fill="none"`. Its class
//! list is `cn('size-4', className)` and nothing else. See
//! `native/mapping/sidebar-toggle-icon.md`.
//!
//! # What the oracle can and cannot see
//!
//! `stroke`, `stroke-width`, `stroke-linecap` and the path data have **no field
//! in `native/oracle/ANCHORS.md`**, and `fg` is emitted only for an element with
//! its own text nodes — an `<svg>` has element children and none. So the whole
//! glyph is invisible and the anchor pins `bounds`, `bg`, `visible`, `radius`
//! and `border.w`.
//!
//! In particular the `rx="2.5"` on the panel rect is **not** the anchor's
//! `radius`: that field reads the element's CSS `border-radius`, which is 0
//! here. A port that translated the rect's corner into `.rounded(px(2.5))`
//! would put a real 2.5px corner on the box and the differ would call it — the
//! one way this component's invisibility can bite in the *other* direction.
//!
//! # `size-4` is an opt-out, and that is the whole design of the file
//!
//! `native/MAPPING.md` records that `button`'s base class list carries
//! `[&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4`,
//! which beats a presentational `width`/`height` attribute outright — P3.2
//! measured a phosphor `size={14}` rendering at 16. **This icon escapes that
//! rule by carrying a `size-` class**, which is what the primitive's own source
//! comment says it is for: without it the glyph took whichever size its button
//! variant dictated, and the same component rendered at four different sizes in
//! four places.
//!
//! The escape is total rather than conditional. `cn` is tailwind-merge, so a
//! call site's own `size-*` would *replace* `size-4` and still match
//! `[class*='size-']`; a call site that names no size leaves `size-4` in place.
//! **There is no class list a call site can pass that lets the button's rule
//! apply**, which is why [`CallSite::override_extent`] is `None` at every one of
//! them and the extent does not move across the `sm` breakpoint.

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};

/// The single anchor this surface carries.
pub const ID_SIDEBAR_TOGGLE_ICON: &str = "sidebar-toggle-icon";

/// **Nothing.** `size-4` authors the width; there is no text and no content
/// measure — `ANCHORS.md` v1.5 does not apply.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** `size-4` authors the height too, and the element paints no
/// text, so there is no line box for it to be derived from.
pub const LINE_SIZED: [&str; 0] = [];

/// `size-4` — `calc(var(--spacing) * 4)` at the stock `0.25rem`, measured live
/// as `16 × 16`, and the same number at every viewport width. See the module
/// docs for why the breakpoint cannot move it.
pub const EXTENT: Pixels = px(16.0);

/// The viewBox's extent, `24` on both axes.
///
/// The glyph's own geometry is authored in these units — the panel rect is
/// `x=3 y=4 w=18 h=16 rx=2.5`, the divider runs at `x=9`, and the rail lines sit
/// at `y=9` and `y=13`. **None of it is in the contract**; it is recorded in
/// `native/mapping/sidebar-toggle-icon.md` rather than as constants here,
/// because a constant nothing draws from is a value that can drift without
/// anything noticing.
pub const VIEW_BOX: f32 = 24.0;

/// The `className` bundle a call site merges over the primitive's `size-4`.
///
/// Both live importers are the same `Button` recipe —
/// `variant="ghost" size="icon-sm"` with a `rounded-sm text-muted-foreground`
/// className on the **button**, not on the icon — so neither passes anything
/// here. The vocabulary exists because "does the call site name a size" is a
/// real branch of the button's `[&_svg:not([class*='size-'])]` rule, and one a
/// port could easily get wrong; that no live call site takes it is the
/// measurement.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className: the primitive's `size-4` alone.
    None,
    /// `tabs/components/tab-navigation-buttons.tsx` — the toggle in the tab bar,
    /// shown when the sidebar is hidden. Passes no className to the icon.
    TabNavigation,
    /// `layout/sidebar-project-header.tsx` — the toggle on the sidebar's leading
    /// edge. **The captured cell.** Passes no className to the icon.
    SidebarProjectHeader,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 3] = [
    CallSite::None,
    CallSite::TabNavigation,
    CallSite::SidebarProjectHeader,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::TabNavigation => "tab-navigation",
            Self::SidebarProjectHeader => "sidebar-project-header",
        }
    }

    /// The extent this call site names on the icon itself, if any.
    ///
    /// **All three are `None`**, and that is the measurement rather than an
    /// oversight — `separator::CallSite::height`'s situation exactly. Neither
    /// live importer passes a className to the glyph; both style the *button*.
    #[must_use]
    pub fn override_extent(self) -> Option<Pixels> {
        match self {
            Self::None | Self::TabNavigation | Self::SidebarProjectHeader => None,
        }
    }
}

/// One `<SidebarToggleIcon>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct SidebarToggleIcon {
    /// The call site whose className is merged over the primitive's `size-4`.
    pub call_site: CallSite,
    /// §8.3's `empty`: the glyph with its extent merged away.
    ///
    /// Expressible from a call site — `cn` is tailwind-merge, so
    /// `className="size-0"` **replaces** `size-4` — and taken by none. Both
    /// engines give a zero-extent box zero area and report `visible: false`, so
    /// the cell is comparable. It is a separate field rather than a
    /// [`CallSite`] variant because the call sites are named importers and this
    /// is a hypothetical class list none of them passes.
    pub empty: bool,
}

impl SidebarToggleIcon {
    /// The captured cell: the sidebar header's toggle at a 1714px viewport —
    /// `16 × 16`, `bg #00000000`, `radius 0`, `border.w 0`, `visible: true`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::SidebarProjectHeader,
            empty: false,
        }
    }

    /// The extent this cell renders at.
    ///
    /// The primitive's [`EXTENT`] unless a call site names its own — and none
    /// does. **No breakpoint parameter**, deliberately: see the module docs.
    #[must_use]
    pub fn extent(self) -> Pixels {
        if self.empty {
            return px(0.0);
        }
        self.call_site.override_extent().unwrap_or(EXTENT)
    }

    /// The glyph's box.
    ///
    /// No background, no radius, no border. `sidebar-toggle-icon.tsx` names
    /// none, preflight zeroes `border` on every element, and the rect's
    /// `rx="2.5"` is viewBox geometry rather than a CSS corner — see the module
    /// docs. The reference agrees on all three.
    fn shell(self) -> Div {
        let extent = self.extent();
        div().flex_shrink_0().w(extent).h(extent)
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_SIDEBAR_TOGGLE_ICON), self.shell())
    }
}

#[cfg(test)]
mod tests {
    use super::{ALL_CALL_SITES, CallSite, EXTENT, SidebarToggleIcon, VIEW_BOX};
    use crate::primitives::button;
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use gpui::px;

    /// Neither declaration is made — `size-4` authors both axes and the glyph
    /// paints no text.
    #[test]
    fn the_glyph_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// **The `size-4` opt-out is load-bearing.** The glyph is 16px at both
    /// breakpoints, where the button rule it escapes is 18 below `sm` and 16 at
    /// or above it. The second half is the control: without it this test would
    /// pass even if the rule never moved, and would be proving nothing.
    #[test]
    fn the_primitives_extent_does_not_follow_the_buttons_icon_rule() {
        for call_site in ALL_CALL_SITES {
            assert_eq!(
                SidebarToggleIcon {
                    call_site,
                    empty: false,
                }
                .extent(),
                px(16.0),
            );
        }

        let size = button::Size::IconSm;
        assert_eq!(size.icon(Breakpoint::Sm), EXTENT, "the two agree above sm");
        assert_ne!(
            size.icon(Breakpoint::Base),
            EXTENT,
            "below sm the button rule is 18 and the opt-out is what keeps the glyph at 16",
        );
        assert_eq!(size.icon(Breakpoint::Base), px(18.0));
    }

    /// **No live call site names a size on the icon** — the record of a
    /// measurement, in `separator`'s shape: the branch exists in the button's
    /// CSS and nothing in the app takes it.
    #[test]
    fn no_modelled_call_site_overrides_the_extent() {
        for call_site in ALL_CALL_SITES {
            assert_eq!(call_site.override_extent(), None, "{}", call_site.name());
        }
    }

    /// The viewBox is 24, so the glyph is drawn at two-thirds scale in its 16px
    /// box. Not a compared quantity — recorded because the ratio is what makes
    /// the 2px stroke read as 1.33px on screen.
    #[test]
    fn the_view_box_is_the_lucide_twenty_four() {
        assert!((VIEW_BOX - 24.0).abs() < f32::EPSILON);
        assert!(f32::from(EXTENT) < VIEW_BOX);
    }

    /// §8.3's `empty` pins **zero** at every call site — the one input that
    /// moves a field the contract records on this surface, and a rendering both
    /// engines agree on.
    #[test]
    fn the_empty_cell_pins_a_zero_extent() {
        for call_site in ALL_CALL_SITES {
            let empty = SidebarToggleIcon {
                call_site,
                empty: true,
            };
            assert_eq!(empty.extent(), px(0.0), "{}", call_site.name());
        }
    }

    /// The vocabulary is closed and its words are unique, and the fixture is the
    /// captured cell.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["none", "sidebar-project-header", "tab-navigation"]);
        assert_eq!(
            SidebarToggleIcon::fixture().call_site,
            CallSite::SidebarProjectHeader,
        );
    }
}
