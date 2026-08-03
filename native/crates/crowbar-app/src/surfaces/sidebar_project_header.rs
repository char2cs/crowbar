//! `--surface sidebar-project-header` — the sidebar's own top bar: a toggle
//! on the leading edge, a back/forward/settings cluster on the trailing edge,
//! and a macOS traffic-light spacer that only exists on one side of one
//! platform.
//!
//! `crowbar_ui::components::sidebar_project_header` carries the full account
//! of this composition's own arithmetic and of why it does not compose
//! `button::Button::render` or `SidebarToggleIcon` directly. This file is the
//! cell.
//!
//! # A live reference exists, and its own numbers confirm this port's arithmetic
//!
//! A live capture at a 344px column, dark, macOS, reads:
//!
//! ```text
//! root sidebar-project-header  344x44
//!   -toggle    x304 y8 28x28      -back      x8  y8 28x28
//!   -forward   x38  y8 28x28      -settings  x68 y8 28x28
//! ```
//!
//! That is not this surface's own default cell — the toggle sitting at the
//! *trailing* edge with the cluster at the *leading* one only happens when
//! `--right` is given — but the arithmetic checks out exactly against
//! [`SidebarProjectHeader`]'s own constants: `back`/`forward`/`settings` are
//! three `28×28` boxes (`button::Size::IconSm.extent(Breakpoint::Sm)`) at
//! `sidebar_project_header::CLUSTER_GAP` (2px) apart, starting at
//! `sidebar_project_header::PADDING_INNER` (8px) off the left edge; the
//! toggle sits `sidebar_project_header::PADDING_OUTER` (12px) off the right
//! edge (`344 − 28 − 12 = 304`); and the root's own height, 44px, is
//! [`sidebar_project_header::HEIGHT_MAC`] — the only arm a running webview
//! ever produces. `the_right_docked_cell_reproduces_the_live_reference`
//! below drives exactly this cell through a real taffy layout and asserts
//! every one of these seven numbers. Neither `--platform` nor the two
//! `can_go_*` booleans are recoverable from the capture (opacity is not a
//! field this contract carries — see
//! `row_layout::button::a_disabled_button_is_still_visible_to_both_extractors`
//! for the identical limitation on `button`'s own surface), so this file's
//! own default cell stays [`SidebarProjectHeader::fixture`]'s left-docked,
//! both-enabled resting state rather than guessing at those two axes.
//!
//! # `sidebar-toggle-icon` must never appear on this surface
//!
//! `web/src/lib/oracle/extract.ts`'s own `oracleSurfaceScope` entry for
//! `sidebar-project-header` deliberately excludes it — the toggle's glyph
//! belongs to a different, already-registered surface
//! (`sidebar_toggle_icon.rs`), and this composition's own port does not
//! paint it (see the component's module docs). `the_toggle_never_carries_its_icons_id`
//! below is what holds this surface to that.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `sidebar-project-header.tsx`'s own `<div>` carries no `hover:`/`focus:`/`data-active` rule at all — every interactive class list lives on the four `<Button>`s, which is `button`'s own surface's business, not this composition's (the identical call `sidebar_toggle_icon.md` §5 makes about the button around *that* glyph). This is not a row, so `Empty` does not apply either. `--right`, `--platform`, `--no-back` and `--no-forward` are this surface's own axis instead — the same shape `fps-overlay`'s `--fps`/`--max-dt`/`--drops` are. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::keybinding::Platform;
use crowbar_ui::components::sidebar_project_header::{self, SidebarProjectHeader};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-project-header",
    root: sidebar_project_header::ID_SIDEBAR_PROJECT_HEADER,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `HEIGHT_MAC` (44) is the tallest arm, plus `CAPTION_HEIGHT`'s 29 — 90
    // holds it with room and is a floor rather than a ceiling: `driven_height`
    // below tracks the platform exactly, so a shorter `--platform other` cell
    // still reports its own real 34+29 through the `max` in
    // `Cell::window_extent`, not this floor.
    min_window_height: 90,
    // The bar fills its own sidebar *column*, not the window — the same
    // ordinary inset `git-status-row`'s own surface takes.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--right`: `sidebarPosition === 'right'`.
    pub right: bool,
    /// `--platform`: `IS_MAC`'s own branch. Only `mac` has a reference —
    /// `platform.ts` returns `'macos'` on every path that has a `window`, the
    /// identical posture `keybinding`'s own `--platform` takes.
    pub platform: Platform,
    /// `--no-back` clears it: `canGoBack`.
    pub can_go_back: bool,
    /// `--no-forward` clears it: `canGoForward`.
    pub can_go_forward: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = SidebarProjectHeader::fixture();
        Self {
            right: fixture.is_right,
            platform: fixture.platform,
            can_go_back: fixture.can_go_back,
            can_go_forward: fixture.can_go_forward,
        }
    }
}

impl Params {
    /// The header this cell describes, at [`SidebarProjectHeader::fixture`]'s
    /// own four anchor ids — this surface drives no option that renames any
    /// of them, the real React source having no prop for it either.
    #[must_use]
    pub fn header(&self) -> SidebarProjectHeader {
        SidebarProjectHeader {
            is_right: self.right,
            platform: self.platform,
            can_go_back: self.can_go_back,
            can_go_forward: self.can_go_forward,
            ..SidebarProjectHeader::fixture()
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--right" => self.right = true,
            "--platform" => self.platform = parse_platform(&value(args, option)?)?,
            "--no-back" => self.can_go_back = false,
            "--no-forward" => self.can_go_forward = false,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// The bar's own height follows [`Platform`] exactly —
    /// [`sidebar_project_header::HEIGHT_MAC`] or
    /// [`sidebar_project_header::HEIGHT_OTHER`] — and nothing else on this
    /// command line moves it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        Some(match self.platform {
            Platform::Mac => 44,
            Platform::Other => 34,
        })
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · {} · platform {} · back {} · forward {}",
            if self.right { "right-docked" } else { "left-docked" },
            if matches!(self.platform, Platform::Mac) {
                "mac"
            } else {
                "other"
            },
            if self.can_go_back { "enabled" } else { "disabled" },
            if self.can_go_forward {
                "enabled"
            } else {
                "disabled"
            },
        );
        if !matches!(self.platform, Platform::Mac) {
            out.push_str(
                " · platform other: platform.ts always reports macos in a webview, so \
                 this cell has no reference",
            );
        }
    }

    /// **`true`, unconditionally.** See the module docs and this surface's
    /// own `unmodelled` list: every §8.3 flag is checked and none applies —
    /// `sidebar-project-header.tsx`'s own wrapper carries no interaction rule
    /// of any kind, and this is not a row, so `Empty` does not apply either.
    /// `--right`/`--platform`/`--no-back`/`--no-forward` are this surface's
    /// own axis instead.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.header().render(theme, anchors)
    }
}

/// `--platform`'s closed vocabulary — the identical spelling
/// `keybinding`'s own surface takes.
fn parse_platform(raw: &str) -> Result<Platform, ParseError> {
    match raw {
        "mac" => Ok(Platform::Mac),
        "other" => Ok(Platform::Other),
        other => Err(ParseError::Rejected(format!(
            "--platform takes mac or other, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--right".to_owned(),
            "sidebarPosition === 'right': mirrors the bar and swaps which edge gets \
             the 12px outer padding [left-docked]"
                .to_owned(),
        ),
        (
            "--platform <mac|other>".to_owned(),
            "IS_MAC's own branch; only mac is reachable in a running webview [mac]".to_owned(),
        ),
        (
            "--no-back".to_owned(),
            "canGoBack = false: dims the back button [enabled]".to_owned(),
        ),
        (
            "--no-forward".to_owned(),
            "canGoForward = false: dims the forward button [enabled]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options, parse_platform};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::keybinding::Platform;
    use crowbar_ui::components::sidebar_project_header::SidebarProjectHeader;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-project-header"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sidebar-project-header cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_components_own_fixture() {
        let bag = Params::default();
        assert!(!bag.right);
        assert_eq!(bag.platform, Platform::Mac);
        assert!(bag.can_go_back);
        assert!(bag.can_go_forward);

        let header = params_of(&cell(&[])).header();
        assert_eq!(header, SidebarProjectHeader::fixture());
    }

    #[test]
    fn every_option_reaches_the_header_independently() {
        let driven = params_of(&cell(&[
            "--right",
            "--platform",
            "other",
            "--no-back",
            "--no-forward",
        ]))
        .header();
        assert!(driven.is_right);
        assert_eq!(driven.platform, Platform::Other);
        assert!(!driven.can_go_back);
        assert!(!driven.can_go_forward);

        let right_only = params_of(&cell(&["--right"])).header();
        assert!(right_only.is_right);
        assert_eq!(right_only.platform, Platform::Mac);
        assert!(right_only.can_go_back);
        assert!(right_only.can_go_forward);
    }

    #[test]
    fn platform_parses_its_closed_vocabulary() {
        assert_eq!(parse_platform("mac"), Ok(Platform::Mac));
        assert_eq!(parse_platform("other"), Ok(Platform::Other));
        assert!(matches!(
            parse_platform("windows"),
            Err(ParseError::Rejected(_))
        ));
    }

    #[test]
    fn driven_height_follows_the_platform_and_nothing_else() {
        assert_eq!(params_of(&cell(&[])).driven_height(&cell(&[])), Some(44));
        assert_eq!(
            params_of(&cell(&["--platform", "other"])).driven_height(&cell(&[])),
            Some(34),
        );
        assert_eq!(
            params_of(&cell(&["--right", "--no-back"])).driven_height(&cell(&[])),
            Some(44),
        );
    }

    #[test]
    fn a_non_numeric_platform_is_rejected_by_name() {
        let line = ["--surface", "sidebar-project-header", "--platform", "windows"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    /// Every flag is unmodelled, and the surface declares that on purpose —
    /// the two facts `no_surface_declares_its_entire_state_axis_unmodelled`
    /// (`surface.rs`) requires to agree with each other.
    #[test]
    fn every_flag_is_unmodelled_and_the_surface_declares_it() {
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(Params::default().no_state_axis());
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--right", "--no-back", "--no-forward"] {
            let line = ["--surface", "skeleton", option];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a skeleton option",
            );
        }
    }

    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--right", "--platform", "--no-back", "--no-forward"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "sidebar-project-header");
        assert_eq!(SURFACE.root, "sidebar-project-header");
        assert!(!SURFACE.full_bleed);
    }
}
