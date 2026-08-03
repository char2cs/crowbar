//! `--surface project-home-row` — the sidebar's own "home" row for the
//! active project.
//!
//! `crowbar_ui::components::project_home_row` carries the composition (why
//! it does not call `Button::render`, why `AddRepositoryModal` is not
//! composed, the `size-6` tailwind-merge arithmetic); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real**, via `--active` — see `project_home_row.rs`'s own account of why `isActive` maps onto this flag rather than a bespoke option. |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled** — see `crowbar_ui::components::project_home_row`'s own module docs. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::project_home_row::ProjectHomeRow;
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "project-home-row",
    root: crowbar_ui::components::project_home_row::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
    ],
    // The row's own `h-9` (36px) plus a little room for the caption — a
    // floor, not a ceiling; `driven_height` returns `None` below, so this is
    // what the window falls back to.
    min_window_height: 60,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--working`: the home workspace's agent-in-flight spinner, replacing
    /// the `Library` glyph.
    pub working: bool,
    /// `--project-name`: `activeProject?.name ?? 'home'`, already resolved.
    pub project_name: SharedString,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            working: false,
            project_name: SharedString::new_static("home"),
        }
    }
}

impl Params {
    /// The row this cell describes. `--flags selected` drives `isActive` —
    /// `StateFlag::Selected` is exactly the "is this row's own active
    /// picture on" concept for a single row, the same reading
    /// `native/oracle/ANCHORS.md` v1.1 gives it.
    #[must_use]
    pub fn row(&self, cell: &Cell) -> ProjectHomeRow {
        ProjectHomeRow {
            is_active: cell.has(StateFlag::Selected),
            working: self.working,
            project_name: self.project_name.clone(),
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
            "--working" => self.working = true,
            "--project-name" => self.project_name = value(args, option)?.into(),
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · project {}", self.project_name);
        if self.working {
            out.push_str(" · working");
        }
        if cell.has(StateFlag::Selected) {
            out.push_str(" · active");
        }
    }

    /// **`false`.** `selected` is a real, driveable flag on this surface —
    /// see the module docs.
    fn no_state_axis(&self) -> bool {
        false
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.row(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--working".to_owned(),
            "the home workspace's agent-in-flight spinner, replacing the Library glyph [idle]"
                .to_owned(),
        ),
        (
            "--project-name <name>".to_owned(),
            "the active project's name, or the 'home' fallback [home]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::project_home_row::ProjectHomeRow;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "project-home-row"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a project-home-row cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_idle_inactive_home_fallback() {
        let bag = Params::default();
        assert!(!bag.working);
        assert_eq!(bag.project_name, "home");

        let default_cell = cell(&[]);
        let row = params_of(&default_cell).row(&default_cell);
        assert_eq!(row, ProjectHomeRow::fixture());
    }

    #[test]
    fn every_option_reaches_the_row_independently() {
        let working_cell = cell(&["--working"]);
        assert!(params_of(&working_cell).row(&working_cell).working);

        let named_cell = cell(&["--project-name", "crowbar"]);
        assert_eq!(
            params_of(&named_cell).row(&named_cell).project_name,
            "crowbar"
        );
    }

    /// `--flags selected` is what drives `isActive` — not a bespoke option.
    ///
    /// **Mutation, run:** swapped `StateFlag::Selected` for
    /// `StateFlag::Hover` on line 72 (`Params::row`'s own `is_active` field
    /// — `describe`'s separate `StateFlag::Selected` read on line 102 was
    /// left untouched, isolating the one call site). `cargo test -p
    /// crowbar-app --bin crowbar-app flags_selected_drives_is_active`
    /// failed as predicted: `assertion failed:
    /// params_of(&selected).row(&selected).is_active`. Reverted after
    /// confirming.
    #[test]
    fn flags_selected_drives_is_active() {
        let resting = cell(&[]);
        assert!(!params_of(&resting).row(&resting).is_active);

        let selected = cell(&["--flags", "selected"]);
        assert!(params_of(&selected).row(&selected).is_active);
    }

    #[test]
    fn the_unmodelled_list_is_everything_but_selected() {
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        assert!(!Params::default().no_state_axis());
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--working", "--project-name"] {
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
        assert_eq!(SURFACE.name, "project-home-row");
        assert_eq!(
            SURFACE.root,
            crowbar_ui::components::project_home_row::ID_ROOT
        );
        assert!(!SURFACE.full_bleed);
    }
}
