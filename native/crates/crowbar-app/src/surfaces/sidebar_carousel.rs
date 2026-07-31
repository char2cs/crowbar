//! `--surface sidebar-carousel` — the four sidebar panels on one scroll-snap
//! track.
//!
//! The fixture is the live carousel from
//! `web/src/components/layout/sidebar-carousel.tsx`. There is only one of them
//! in the app and it is always mounted, so unlike the menu there is no choice of
//! call site to make.
//!
//! # The state axis, and the one flag that is real
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **the only real one.** The carousel is scrolled to `--active-tab`; without it the track rests at `scrollLeft = 0`, which is `getInitialState()`'s `workspaces`. Selecting a tab is what moves this component, and it is the *only* thing that does. |
//! | `hover`, `focus` | **unmodelled.** The class list carries no `hover:` and no `focus:` rule, the scroll container takes no `tabindex`, and the scrollbar it would otherwise reveal is removed by `[scrollbar-width:none]`. |
//! | `empty` | **unmodelled.** `TABS` is a module constant of four and every panel is rendered unconditionally, so there is no carousel with nothing in it. |
//! | `loading`, `error` | **unmodelled**, and worth one sentence rather than a shrug: the files panel really does carry a `<Suspense fallback={<SidebarSkeleton />}>` and an `<ErrorBoundary>`, so the *component* has both states. They swap content **inside** a panel whose box is floored at the scrollport's width by `min-w-full`, so no anchored box moves and the cells cannot fail. |
//!
//! ## `selected` is a reading of §8.3's vocabulary, and is recorded as one
//!
//! `ANCHORS.md` v1.1 fixes the flag vocabulary at six words and none of them is
//! "which panel is showing". `selected` is the closest: the panel snapped into
//! the scrollport **is** the selected tab, a reference capture reaches the cell
//! by calling `setActiveTab`, and the flag then moves every anchor's `bounds.x`
//! and flips `visible` on two of the four panels. It is a reading rather than a
//! translation, and the alternative — declaring all six flags unmodelled — is
//! both what the component honestly deserves and something
//! `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled`
//! forbids. Reported rather than resolved quietly.
//!
//! # Two options that are not matrix axes
//!
//! `--height` is `NavStack`'s column, not the carousel's own: the class list's
//! only block-axis declaration is `flex-1`. It is a runtime input in exactly the
//! sense `dropdown-menu`'s `--anchor-width` is, and it has to be drivable
//! because the reference's carousel is as tall as the sidebar it is in — a
//! native run at some fixed height would report an `h` delta on every anchor
//! that belongs to neither side.
//!
//! `--panel-content` is the claim that licenses rendering the four panels empty,
//! made drivable. See [`Params::panel_content`].

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::sidebar_carousel::{self, SidebarCarousel, SidebarTab, TABS};
use crowbar_ui::components::AnchorSink;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-carousel",
    root: sidebar_carousel::ID_SCROLLPORT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
    ],
    // The tallest surface so far by an order of magnitude: a sidebar is a
    // column, not a row. `MAX_HEIGHT` is held below this with room for the
    // caption by `the_window_holds_the_tallest_carousel_and_its_caption`.
    window_height: 700,
    options,
    params: || Box::new(Params::default()),
};

/// `--active-tab`'s default: **`files`**, the third panel.
///
/// Chosen the way `dropdown-menu`'s `--focus-row 1` was — so that a bare
/// `--flags selected` renders something that can fail. It is also the sharpest
/// of the four: at `scrollLeft = 2W` two panels sit entirely off the left edge,
/// one is in view, and the fourth begins **exactly** at the scrollport's right
/// edge. That last one is the tangent case, where a zero-width intersection has
/// to read as `visible: false` on both sides.
pub const DEFAULT_ACTIVE_TAB: SidebarTab = SidebarTab::Files;

/// `--height`'s default, in logical px.
pub const DEFAULT_HEIGHT: u16 = 600;

/// The tallest `--height` this surface's window can hold with its caption still
/// on screen.
///
/// Rejected rather than clamped, for the reason `--viewport-width` is: a
/// carousel taller than the window is one whose lower panels are cut by the
/// window's own content mask, and a `visible` that turned on the window size
/// would be an artefact rather than a fact about the port.
pub const MAX_HEIGHT: u16 = 640;

/// This surface's own options.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--active-tab`: which panel the `selected` flag snaps the track to.
    ///
    /// Two knobs for one picture, exactly as `dropdown-menu` splits
    /// `--focus-row` from `--flags focus`: this says *which*, the flag says
    /// *whether*.
    pub active_tab: SidebarTab,
    /// `--height`: the height of the column the carousel's `flex-1` resolves
    /// against. See the module docs.
    pub height: u16,
    /// `--panel-content`: the max-content width of a filler box placed inside
    /// **every** panel.
    ///
    /// Zero renders the panels empty, which is what the port draws. It is
    /// drivable because "the panels' boxes do not depend on their contents" is
    /// the claim that makes drawing them empty legitimate, and a claim that can
    /// be driven is a measurement rather than an argument — drive it to several
    /// times the surface width and every anchored box has to be unchanged.
    pub panel_content: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            active_tab: DEFAULT_ACTIVE_TAB,
            height: DEFAULT_HEIGHT,
            panel_content: 0,
        }
    }
}

impl Params {
    /// The carousel this cell describes.
    #[must_use]
    pub fn carousel(self, cell: &Cell) -> SidebarCarousel {
        SidebarCarousel {
            // Without the flag the track rests where a freshly mounted carousel
            // rests: `getInitialState().activeTab` is `workspaces`, index 0.
            active: if cell.has(StateFlag::Selected) {
                self.active_tab
            } else {
                SidebarTab::Workspaces
            },
            panel_content_width: px(f32::from(self.panel_content)),
        }
    }

    /// Which tab is actually showing in this cell, flag and option together.
    #[must_use]
    pub fn showing(self, cell: &Cell) -> SidebarTab {
        self.carousel(cell).active
    }

    /// The tab vocabulary as one line, for the usage and for a rejection.
    fn vocabulary() -> String {
        TABS.into_iter()
            .map(SidebarTab::name)
            .collect::<Vec<_>>()
            .join(", ")
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--active-tab" => {
                let raw = value(args, option)?;
                self.active_tab = SidebarTab::parse(&raw).ok_or_else(|| {
                    ParseError::Rejected(format!(
                        "--active-tab takes one of {}, not {raw}",
                        Self::vocabulary(),
                    ))
                })?;
            }
            "--height" => {
                let height = pixels(&value(args, option)?, option)?;
                if height == 0 || height > MAX_HEIGHT {
                    return Err(ParseError::Rejected(format!(
                        "--height {height} is outside 1..={MAX_HEIGHT}; this surface's window is \
                         {} logical px tall and the caption sits below the carousel, so a taller \
                         one would be cut by the window's own clip and every `visible` in the \
                         snapshot would be an artefact of the window size",
                        SURFACE.window_height,
                    )));
                }
                self.height = height;
            }
            "--panel-content" => self.panel_content = pixels(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Which tab is showing, how tall the column is — and, where it applies, the
    /// fact that a flag was asked for that this cell cannot paint.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · showing {} · {}px tall",
            self.showing(cell).name(),
            self.height,
        );
        if self.panel_content > 0 {
            let _ = write!(out, " · panel content {}px", self.panel_content);
        }
        // Said on every caption, because it is the largest single thing a reader
        // could otherwise assume this surface covers: two of §8.3's four axes
        // cannot move an anchor here at all. The component paints no colour, so
        // `--theme` selects a token table nothing reads; it paints no text, so
        // the three `--content` lengths are one picture.
        out.push_str(" · no paint: theme and content move nothing here");
        // A `selected` cell that snaps to the tab the carousel already rests on
        // compares resting against resting. `Surface::unmodelled` is per surface
        // and this is per *cell*, so it is said here.
        if cell.has(StateFlag::Selected) && self.active_tab == SidebarTab::Workspaces {
            out.push_str(" · selected: --active-tab is where the track rests, so this cell cannot fail");
        }
    }

    /// The carousel inside the column its `flex-1` needs.
    ///
    /// The column is `NavStack`'s inner layer — `flex h-full flex-col` — and it
    /// is **unanchored** and lives here rather than in the component, because
    /// `nav-stack.tsx` is a different component and a `crowbar-ui` module that
    /// painted its neighbour's box would be one this port could not port twice.
    /// The tree the differ sees is unaffected either way: `ANCHORS.md` §1 makes
    /// nesting irrelevant, and the root anchor is the scrollport.
    ///
    /// `theme` is unread. This is the one component in the port with no colour,
    /// no text and no radius; see `crowbar_ui::components::sidebar_carousel`.
    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_col()
            .h(px(f32::from(self.height)))
            .child(self.carousel(cell).render(anchors))
            .into_any_element()
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--active-tab <name>".to_owned(),
            format!(
                "which panel `selected` snaps to; {} [{}]",
                Params::vocabulary(),
                DEFAULT_ACTIVE_TAB.name(),
            ),
        ),
        (
            "--height <px>".to_owned(),
            format!("the column flex-1 resolves against, 1..={MAX_HEIGHT} [{DEFAULT_HEIGHT}]"),
        ),
        (
            "--panel-content <px>".to_owned(),
            "filler inside every panel; drives the claim that the track ignores it [0]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_ACTIVE_TAB, DEFAULT_HEIGHT, MAX_HEIGHT, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::sidebar_carousel::{SidebarTab, TABS};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-carousel"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sidebar-carousel cell carries this surface's bag")
    }

    /// The defaults are the live carousel plus the sharpest tab to land on.
    #[test]
    fn the_defaults_are_the_live_carousel() {
        let bag = Params::default();

        assert_eq!(bag.active_tab, DEFAULT_ACTIVE_TAB);
        assert_eq!(bag.active_tab, SidebarTab::Files);
        assert_eq!(bag.height, DEFAULT_HEIGHT);
        assert_eq!(bag.panel_content, 0);

        // And with no flag the track rests where the store starts, whatever
        // `--active-tab` says.
        let resting = cell(&[]);
        assert_eq!(params_of(&resting).showing(&resting), SidebarTab::Workspaces);
        assert_eq!(
            params_of(&resting).carousel(&resting).panel_content_width,
            px(0.0),
        );
    }

    /// `selected` is the flag that moves this surface, and `--active-tab` is
    /// which way — two knobs for one picture, as `dropdown-menu` splits
    /// `--focus-row` from `--flags focus`.
    #[test]
    fn selected_is_what_scrolls_the_track_and_active_tab_is_where_to() {
        for tab in TABS {
            let selected = cell(&["--flags", "selected", "--active-tab", tab.name()]);
            assert_eq!(params_of(&selected).showing(&selected), tab, "{}", tab.name());

            // Without the flag, the same `--active-tab` renders the resting
            // track — so the flag is not decoration.
            let resting = cell(&["--active-tab", tab.name()]);
            assert_eq!(
                params_of(&resting).showing(&resting),
                SidebarTab::Workspaces,
                "{}",
                tab.name(),
            );
        }

        // No other flag reaches it.
        for flag in ["empty", "loading", "error", "hover", "focus"] {
            let other = cell(&["--flags", flag, "--active-tab", "git"]);
            assert_eq!(
                params_of(&other).showing(&other),
                SidebarTab::Workspaces,
                "{flag}",
            );
        }
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--active-tab", "diff"],
            vec!["--active-tab"],
            vec!["--height", "tall"],
            vec!["--height"],
            vec!["--height", "0"],
            vec!["--panel-content", "-1"],
            vec!["--panel-content"],
        ] {
            let mut full = vec!["--surface", "sidebar-carousel"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "sidebar-carousel", "--active-tab", "diff"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("`diff` is not a tab");
        };
        for tab in TABS {
            assert!(complaint.contains(tab.name()), "{complaint}");
        }
    }

    /// A `--height` the window cannot hold is refused, and the refusal says why
    /// rather than clamping — a carousel cut by the window's own clip would make
    /// `visible` an artefact of the window size.
    #[test]
    fn a_height_the_window_cannot_hold_is_refused_with_its_reason() {
        assert!(Cell::parse(
            ["--surface", "sidebar-carousel", "--height", "640"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        )
        .is_ok());

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "sidebar-carousel", "--height", "641"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("641 is above the ceiling");
        };
        assert!(complaint.contains("visible"), "{complaint}");
        assert!(complaint.contains("641"), "{complaint}");
    }

    /// The window has to hold the tallest carousel **and** the caption below it,
    /// or `MAX_HEIGHT` is a ceiling that still produces a clipped surface.
    #[test]
    fn the_window_holds_the_tallest_carousel_and_its_caption() {
        // `RowSurface::render`: the surface starts at `INSET_Y`, and the caption
        // sits below it behind a 12px gap on a ~17px line.
        const CAPTION: u16 = 12 + 17;
        let needed = MAX_HEIGHT + CAPTION;
        assert!(
            needed <= SURFACE.window_height,
            "{needed} does not fit in {}",
            SURFACE.window_height,
        );
    }

    /// **These options belong to this surface and to no other.**
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--active-tab", "--height", "--panel-content"] {
            let line = ["--surface", "git-status-row", option, "100"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in ["--active-tab", "--height", "--panel-content"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("sidebar-carousel"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
        // The usage lists which flags each surface really has; this one has
        // exactly `selected`.
        for tab in TABS {
            assert!(usage.contains(tab.name()), "{}", tab.name());
        }
    }

    /// The caption says what is showing, and says so **per cell** where a flag
    /// was asked for that cannot paint.
    #[test]
    fn the_caption_names_the_showing_tab_and_the_cells_that_cannot_fail() {
        let moved = cell(&["--flags", "selected", "--active-tab", "git"]);
        let caption = moved.describe();
        assert!(caption.contains("showing git"), "{caption}");
        assert!(caption.contains("600px tall"), "{caption}");
        assert!(!caption.contains("cannot fail"), "{caption}");

        // `selected` onto the tab the track already rests on.
        let vacuous = cell(&["--flags", "selected", "--active-tab", "workspaces"]);
        let caption = vacuous.describe();
        assert!(caption.contains("showing workspaces"), "{caption}");
        assert!(caption.contains("cannot fail"), "{caption}");

        // And the standing note about the two axes that cannot move an anchor.
        for line in [vec![], vec!["--theme", "light"], vec!["--content", "short"]] {
            assert!(cell(&line).describe().contains("no paint"), "{line:?}");
        }

        let stuffed = cell(&["--panel-content", "1200"]);
        assert!(
            stuffed.describe().contains("panel content 1200px"),
            "{}",
            stuffed.describe(),
        );
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim, and the state axis this surface claims.
    #[test]
    fn the_surface_names_itself_its_root_and_its_one_real_flag() {
        assert_eq!(SURFACE.name, "sidebar-carousel");
        assert_eq!(SURFACE.root, "carousel-scrollport");

        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }
}
