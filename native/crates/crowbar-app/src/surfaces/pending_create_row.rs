//! `--surface pending-create-row` — the optimistic row shown while a
//! workspace create is in flight.
//!
//! `crowbar_ui::components::pending_create_row` carries the composition
//! (why this surface is always `AnchorSink::boxed`, why the spinner branch
//! composes `workspace-branch-icon` directly); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled** — see `crowbar_ui::components::pending_create_row`'s own module docs. `--error` (below) is a domain option, not `StateFlag::Error`. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::pending_create_row::PendingCreateRow;
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "pending-create-row",
    root: crowbar_ui::components::pending_create_row::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The row's own `h-9` (36px) plus the outer `pl` — a floor, not a
    // ceiling; `driven_height` returns `None` below.
    min_window_height: 60,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--branch`: `pending.branch`.
    pub branch: SharedString,
    /// `--error`: swaps the spinner + label for the `✕` mark + label +
    /// `"failed"` + dismiss button.
    pub error: bool,
    /// `--padding-left`: the caller's own `paddingLeft` prop, in whole
    /// logical px. `u16` rather than `Pixels` so `Cell` stays `Eq`
    /// (`crate::surface::Surface::min_window_height`'s own reason).
    pub padding_left: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            branch: SharedString::new_static("feature/example"),
            error: false,
            padding_left: 28,
        }
    }
}

impl Params {
    /// The row this cell describes.
    #[must_use]
    pub fn row(&self) -> PendingCreateRow {
        PendingCreateRow {
            branch: self.branch.clone(),
            error: self.error,
            padding_left: px(f32::from(self.padding_left)),
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
            "--branch" => self.branch = value(args, option)?.into(),
            "--error" => self.error = true,
            "--padding-left" => self.padding_left = parse_padding(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(out, " · branch {} · padding-left {}px", self.branch, self.padding_left);
        if self.error {
            out.push_str(" · failed");
        }
    }

    /// **`true`.** None of the six §8.3 flags has a rule on this surface —
    /// see the module docs.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.row().render(theme, anchors)
    }
}

fn parse_padding(raw: &str) -> Result<u16, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("--padding-left takes a whole number of px, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    [
        ("--branch <name>".to_owned(), "the branch name shown while creating [feature/example]".to_owned()),
        ("--error".to_owned(), "show the failed picture instead of the spinner [off]".to_owned()),
        ("--padding-left <px>".to_owned(), "the caller's own indentation, in px [28]".to_owned()),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "pending-create-row"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a pending-create-row cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_idle_and_nested_one_level() {
        let bag = Params::default();
        assert!(!bag.error);
        assert_eq!(bag.branch, "feature/example");
        assert_eq!(bag.padding_left, 28);

        let row = params_of(&cell(&[])).row();
        assert!(!row.error);
        assert_eq!(row.padding_left, px(28.0));
    }

    #[test]
    fn every_option_reaches_the_row_independently() {
        let branch_cell = cell(&["--branch", "hotfix/urgent"]);
        assert_eq!(params_of(&branch_cell).row().branch, "hotfix/urgent");

        let error_cell = cell(&["--error"]);
        assert!(params_of(&error_cell).row().error);

        let padding_cell = cell(&["--padding-left", "14"]);
        assert_eq!(params_of(&padding_cell).row().padding_left, px(14.0));
    }

    #[test]
    fn the_unmodelled_list_is_every_flag() {
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
    fn padding_left_rejects_a_non_numeric_value() {
        let line = ["--surface", "pending-create-row", "--padding-left", "many"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(crate::row_surface::ParseError::Rejected(_)),
        ));
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--branch", "--error", "--padding-left"] {
            let line = ["--surface", "skeleton", option];
            assert!(
                Cell::parse(line.iter().map(|arg| (*arg).to_owned())).is_err(),
                "{option} should not be a skeleton option",
            );
        }
    }

    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "pending-create-row");
        assert_eq!(SURFACE.root, crowbar_ui::components::pending_create_row::ID_ROOT);
        assert!(!SURFACE.full_bleed);
    }
}
