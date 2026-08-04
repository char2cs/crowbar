//! `--surface project-switcher-panel` — the body of the pushed "Projects"
//! sidebar screen: a column of project rows plus the static "Import
//! project" row.
//!
//! `crowbar_ui::components::project_switcher_panel` carries the composition
//! (why `h-full` is not modelled, why row ids are index-parameterized, why
//! `ImportProjectModal` is not composed); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real** — zero projects, only the import row. See `project_switcher_panel.rs`'s own module docs. |
//! | `loading`, `error`, `hover`, `focus`, `selected` | **unmodelled** — see `crowbar_ui::components::project_switcher_panel`'s own module docs. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::project_switcher_panel::{ProjectRow, ProjectSwitcherPanel};
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, index, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "project-switcher-panel",
    root: crowbar_ui::components::project_switcher_panel::ID_ROOT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // A floor with room, not a ceiling — `driven_height` always returns
    // `Some`, so the window follows the panel's own content height exactly
    // as `command`'s own popup does.
    min_window_height: 90,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// The three representative project names `--count` cycles through — no
/// claim to being any real project's name, the same posture
/// `context_pill::Params`'s own `"crowbar"` fixture takes.
const FIXTURE_NAMES: [&str; 3] = ["crowbar", "sandbox", "playground"];

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--count`: how many project rows this cell renders, before `--flags
    /// empty` forces zero regardless.
    pub count: u8,
    /// `--active-index`: which row (if any) is the active project.
    pub active_index: Option<usize>,
    /// `--project-name`: overrides row 0's own name — `project-home-row`'s
    /// own `--project-name` (`surfaces/project_home_row.rs`), ported to this
    /// surface's row list. That surface has exactly one row, so its flag
    /// names the row outright; this one has `--count` many, so the flag
    /// names row 0 specifically — the row this item's own live drive
    /// actually compared (`row-0-label.text`), and the only row a fixed
    /// single name can stand for without inventing a name for every other
    /// row too. Rows past 0 keep cycling [`FIXTURE_NAMES`] exactly as
    /// before. `None` leaves row 0 on the cycle as well, so a bare
    /// `--surface project-switcher-panel` is unchanged by this flag's
    /// existence.
    pub project_name: Option<SharedString>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            count: 2,
            active_index: Some(0),
            project_name: None,
        }
    }
}

impl Params {
    /// The panel this cell describes. `--flags empty` forces zero rows
    /// regardless of `--count` — the same swap `command.rs`'s own
    /// `Params::command` makes for its `ListContent::Empty` arm.
    /// `--project-name` overrides row 0's own name only — see the field's
    /// own doc comment.
    #[must_use]
    pub fn panel(&self, cell: &Cell) -> ProjectSwitcherPanel {
        let rows = if cell.has(StateFlag::Empty) {
            Vec::new()
        } else {
            (0..self.count)
                .map(|i| {
                    let name = if i == 0 {
                        self.project_name.clone().unwrap_or_else(|| fixture_name(i))
                    } else {
                        fixture_name(i)
                    };
                    ProjectRow {
                        name,
                        is_active: self.active_index == Some(usize::from(i)),
                    }
                })
                .collect()
        };
        ProjectSwitcherPanel { rows }
    }

    /// The panel's own intrinsic height, spelled here rather than measured
    /// because [`SurfaceParams::driven_height`] is asked before a window
    /// exists — the same reason `command.rs`'s own `popup_height` is a
    /// method rather than a live measurement.
    #[must_use]
    pub fn popup_height(&self, cell: &Cell) -> u16 {
        let height = self.panel(cell).content_height();
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole number of px; `Cell` needs a \
                      `u16` to stay `Eq`"
        )]
        {
            f32::from(height).ceil() as u16
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
            "--count" => self.count = parse_count(&value(args, option)?)?,
            "--active-index" => self.active_index = Some(index(&value(args, option)?, option)?),
            "--no-active" => self.active_index = None,
            "--project-name" => self.project_name = Some(value(args, option)?.into()),
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.popup_height(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no projects imported yet");
            return;
        }
        let _ = write!(out, " · {} project(s)", self.count);
        if let Some(i) = self.active_index {
            let _ = write!(out, " · active {i}");
        }
        if let Some(name) = &self.project_name {
            let _ = write!(out, " · row 0 named {name}");
        }
    }

    /// **`false`.** `empty` is a real, driveable flag on this surface — see
    /// the module docs.
    fn no_state_axis(&self) -> bool {
        false
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.panel(cell).render(theme, anchors)
    }
}

/// One of [`FIXTURE_NAMES`], cycled — `--count` is not bounded by the list's
/// own length.
fn fixture_name(index: u8) -> SharedString {
    let position = usize::from(index) % FIXTURE_NAMES.len();
    SharedString::new_static(FIXTURE_NAMES[position])
}

fn parse_count(raw: &str) -> Result<u8, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("--count takes a whole number 0..=255, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--count <n>".to_owned(),
            "how many project rows this cell renders [2]".to_owned(),
        ),
        (
            "--active-index <i>".to_owned(),
            "which row is the active project [0]".to_owned(),
        ),
        (
            "--no-active".to_owned(),
            "no row is the active project".to_owned(),
        ),
        (
            "--project-name <name>".to_owned(),
            "overrides row 0's own name — the live app's real project name, since --count \
             otherwise cycles a placeholder list [crowbar]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{FIXTURE_NAMES, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "project-switcher-panel"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a project-switcher-panel cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_two_projects_the_first_active() {
        let bag = Params::default();
        assert_eq!(bag.count, 2);
        assert_eq!(bag.active_index, Some(0));
        assert_eq!(bag.project_name, None);

        let default_cell = cell(&[]);
        let panel = params_of(&default_cell).panel(&default_cell);
        assert_eq!(panel.rows.len(), 2);
        assert!(panel.rows[0].is_active);
        assert!(!panel.rows[1].is_active);
        assert_eq!(panel.rows[0].name, FIXTURE_NAMES[0]);
        assert_eq!(panel.rows[1].name, FIXTURE_NAMES[1]);
    }

    /// **`--project-name` overrides row 0's own name only** — the exact gap
    /// this item's own live drive hit: `row-0-label.text: "crowbar",
    /// expected "oracle-fixture"`, with no flag able to close it. Row 1
    /// keeps cycling [`FIXTURE_NAMES`] untouched, so the flag names the one
    /// row a live drive can actually compare without inventing names for
    /// every other row too.
    #[test]
    fn project_name_overrides_only_row_0() {
        let named = cell(&["--count", "2", "--project-name", "oracle-fixture"]);
        let panel = params_of(&named).panel(&named);
        assert_eq!(panel.rows[0].name, "oracle-fixture");
        assert_eq!(panel.rows[1].name, FIXTURE_NAMES[1], "row 1 is untouched");

        let unset = cell(&[]);
        assert_eq!(
            params_of(&unset).panel(&unset).rows[0].name,
            FIXTURE_NAMES[0],
            "no --project-name leaves row 0 on the cycle, unchanged",
        );
    }

    #[test]
    fn count_and_active_index_reach_the_panel_independently() {
        let five = cell(&["--count", "5", "--no-active"]);
        let panel = params_of(&five).panel(&five);
        assert_eq!(panel.rows.len(), 5);
        assert!(panel.rows.iter().all(|row| !row.is_active));

        let third = cell(&["--count", "5", "--active-index", "3"]);
        let panel = params_of(&third).panel(&third);
        assert!(panel.rows[3].is_active);
        assert_eq!(panel.rows.iter().filter(|row| row.is_active).count(), 1);
    }

    /// `--flags empty` forces zero rows regardless of `--count` — the
    /// import row is not part of `ProjectSwitcherPanel::rows` at all, so
    /// this list alone going empty is the whole of what the flag does.
    ///
    /// **Mutation, run:** deleted the `if cell.has(StateFlag::Empty) { .. }
    /// else { .. }` in `Params::panel`, keeping only the `else` arm's row
    /// construction unconditionally. `cargo test -p crowbar-app --bin
    /// crowbar-app flags_empty_forces_zero_rows_regardless_of_count` failed
    /// as predicted: `left: 4, right: 0` — `--flags empty --count 4` still
    /// rendered all four rows. Reverted after confirming.
    #[test]
    fn flags_empty_forces_zero_rows_regardless_of_count() {
        let full = cell(&["--count", "4"]);
        assert_eq!(params_of(&full).panel(&full).rows.len(), 4);

        let empty = cell(&["--count", "4", "--flags", "empty"]);
        assert_eq!(params_of(&empty).panel(&empty).rows.len(), 0);
    }

    #[test]
    fn fixture_names_cycle_past_the_lists_own_length() {
        let wraps = cell(&["--count", "4"]);
        let panel = params_of(&wraps).panel(&wraps);
        assert_eq!(panel.rows[3].name, panel.rows[0].name);
    }

    #[test]
    fn the_unmodelled_list_is_everything_but_empty() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
        assert!(!Params::default().no_state_axis());
    }

    #[test]
    fn count_rejects_a_non_numeric_value() {
        let line = ["--surface", "project-switcher-panel", "--count", "many"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--count", "--active-index", "--no-active", "--project-name"] {
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
        assert_eq!(SURFACE.name, "project-switcher-panel");
        assert_eq!(
            SURFACE.root,
            crowbar_ui::components::project_switcher_panel::ID_ROOT
        );
        assert!(!SURFACE.full_bleed);
    }
}
