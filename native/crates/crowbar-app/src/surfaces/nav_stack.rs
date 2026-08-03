//! `--surface nav-stack` — the sidebar's push/pop screen stack.
//!
//! `crowbar_ui::components::nav_stack` carries the full account of this
//! composition's own arithmetic, the unbounded-stack-vs-bounded-contract
//! argument, and the re-derivation of the cluster's own transition-end-state
//! reasoning. This file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** Every interactive class in `nav-stack.tsx`'s own tree lives on the back `<Button>`, which is `button`'s own surface's business — the identical call `sidebar_project_header.rs`'s module docs already make about its own four buttons. `--screen`, `--right`, `--platform` and `--content-width` are this surface's own axis instead. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::keybinding::Platform;
use crowbar_ui::components::nav_stack::{self, NavStack};
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "nav-stack",
    root: nav_stack::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The same floor `sidebar-carousel` takes, for the same reason: this
    // component occupies the identical box in the real app (`nav-
    // stack.tsx` is `sidebar-carousel.tsx`'s own outermost element), so
    // a sidebar-sized column is the honest default rather than an
    // arbitrarily smaller one.
    min_window_height: 700,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--height`'s default, in logical px — the same number `sidebar-carousel`
/// takes, for the reason above.
pub const DEFAULT_HEIGHT: u16 = 600;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--screen`: whether a screen is pushed onto the stack.
    pub screen: bool,
    /// `--right`: `sidebarPosition === 'right'`.
    pub right: bool,
    /// `--platform`: `IS_MAC`'s own branch. Only `mac` has a reference.
    pub platform: Platform,
    /// `--content-width`: drives the claim that the base/body box does not
    /// depend on its own (opaque) content.
    pub content_width: u16,
    /// `--height`: the column `flex-1` resolves against.
    pub height: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            screen: false,
            right: false,
            platform: Platform::Mac,
            content_width: 0,
            height: DEFAULT_HEIGHT,
        }
    }
}

/// The word `cell.content`'s three lengths pick for the pushed screen's own
/// title — the same per-surface mapping `input.rs`'s own `text_of` and
/// `label.rs`'s call sites establish, since `screen.title` is real,
/// arbitrary text `ContentLength` exists to vary.
fn title_of(content: ContentLength) -> &'static str {
    match content {
        ContentLength::Short => "Files",
        ContentLength::Normal => "Switch Project",
        ContentLength::Overflow => {
            "A Very Long Pushed Screen Title That Should Truncate Under The Available Width"
        }
    }
}

impl Params {
    /// The stack this cell describes.
    #[must_use]
    pub fn stack(&self, cell: &Cell) -> NavStack {
        NavStack {
            top: self
                .screen
                .then(|| nav_stack::Screen { title: title_of(cell.content).into() }),
            is_right: self.right,
            platform: self.platform,
            content_width: px(f32::from(self.content_width)),
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
            "--screen" => self.screen = true,
            "--right" => self.right = true,
            "--platform" => self.platform = parse_platform(&value(args, option)?)?,
            "--content-width" => self.content_width = pixels(&value(args, option)?, option)?,
            "--height" => {
                let height = pixels(&value(args, option)?, option)?;
                if height == 0 {
                    return Err(ParseError::Rejected(
                        "--height must be greater than zero: a column with no height leaves \
                         the stack's flex-1 nothing to resolve against"
                            .to_owned(),
                    ));
                }
                self.height = height;
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// `--height`, exactly as `sidebar-carousel`'s own surface takes it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        Some(self.height)
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · {} · platform {} · {}px tall",
            if self.screen { "screen pushed" } else { "resting" },
            if matches!(self.platform, Platform::Mac) { "mac" } else { "other" },
            self.height,
        );
        if self.screen {
            let _ = write!(out, " · title \"{}\"", title_of(cell.content));
        }
        if self.content_width > 0 {
            let _ = write!(out, " · content {}px", self.content_width);
        }
    }

    /// **`true`, unconditionally.** Every §8.3 flag is checked and none
    /// applies — see the module docs.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_col()
            .h(px(f32::from(self.height)))
            .child(self.stack(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// `--platform`'s closed vocabulary — the identical spelling `keybinding`'s
/// and `sidebar-project-header`'s own surfaces take.
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
            "--screen".to_owned(),
            "pushes a screen onto the stack, receding the base layer [resting]".to_owned(),
        ),
        (
            "--right".to_owned(),
            "sidebarPosition === 'right': hides the traffic-light spacer [left-docked]".to_owned(),
        ),
        (
            "--platform <mac|other>".to_owned(),
            "IS_MAC's own branch; only mac is reachable in a running webview [mac]".to_owned(),
        ),
        (
            "--content-width <px>".to_owned(),
            "filler inside the base/body box; drives the claim that it ignores content [0]"
                .to_owned(),
        ),
        (
            "--height <px>".to_owned(),
            format!("the column flex-1 resolves against [{DEFAULT_HEIGHT}]"),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_HEIGHT, Params, SURFACE, options, parse_platform, title_of};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::keybinding::Platform;
    use crowbar_ui::components::ContentLength;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "nav-stack"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a nav-stack cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_resting_stack() {
        let bag = Params::default();
        assert!(!bag.screen);
        assert!(!bag.right);
        assert_eq!(bag.platform, Platform::Mac);
        assert_eq!(bag.content_width, 0);
        assert_eq!(bag.height, DEFAULT_HEIGHT);

        let stack = params_of(&cell(&[])).stack(&cell(&[]));
        assert!(stack.top.is_none());
    }

    #[test]
    fn every_option_reaches_the_stack_independently() {
        let driven = cell(&["--screen", "--right", "--platform", "other", "--content-width", "40"]);
        let stack = params_of(&driven).stack(&driven);
        assert!(stack.top.is_some());
        assert!(stack.is_right);
        assert_eq!(stack.platform, Platform::Other);
        assert_eq!(stack.content_width, px(40.0));
    }

    /// **`--content` picks the pushed screen's own title** — the shared
    /// §8.3 axis, read the same way `input.rs`'s `text_of` reads it.
    #[test]
    fn content_length_picks_the_screens_title() {
        for (length, word) in [
            (ContentLength::Short, "short"),
            (ContentLength::Normal, "normal"),
            (ContentLength::Overflow, "overflow"),
        ] {
            let driven = cell(&["--screen", "--content", word]);
            let stack = params_of(&driven).stack(&driven);
            assert_eq!(
                stack.top.expect("--screen was set").title,
                title_of(length),
            );
        }
    }

    #[test]
    fn platform_parses_its_closed_vocabulary() {
        assert_eq!(parse_platform("mac"), Ok(Platform::Mac));
        assert_eq!(parse_platform("other"), Ok(Platform::Other));
        assert!(matches!(parse_platform("windows"), Err(ParseError::Rejected(_))));
    }

    #[test]
    fn a_zero_height_is_rejected_by_name() {
        let line = ["--surface", "nav-stack", "--height", "0"];
        let Err(ParseError::Rejected(complaint)) =
            Cell::parse(line.iter().map(|arg| (*arg).to_owned()))
        else {
            panic!("a zero-height column is not a picture");
        };
        assert!(complaint.contains("flex-1"), "{complaint}");
    }

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
        for option in ["--screen", "--right", "--content-width"] {
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
        for option in ["--screen", "--right", "--platform", "--content-width", "--height"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "nav-stack");
        assert_eq!(SURFACE.root, "nav-stack");
        assert!(!SURFACE.full_bleed);
    }
}
