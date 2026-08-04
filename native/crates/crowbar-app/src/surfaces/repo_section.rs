//! `--surface repo-section` — one repo's header row plus, via `--roots`/
//! `--pending`, its own top-level workspace rows.
//!
//! `crowbar_ui::components::repo_section` carries the composition (why
//! `RepoImportDialog` is not composed, why the trigger composition is
//! collision-free); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real**, via `--flags selected` — `activeWorkspaceId === repo.defaultWorkspaceId`, the same reading every other row-shaped surface in this port gives it. |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled** — see `crowbar_ui::components::repo_section`'s own module docs. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::pending_create_row::PendingCreateRow;
use crowbar_ui::components::repo_icon_popover::Trigger;
use crowbar_ui::components::repo_section::RepoSection;
use crowbar_ui::components::row_base::RowMode;
use crowbar_ui::components::workspace_tree_item::WorkspaceTreeItem;
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "repo-section",
    root: crowbar_ui::components::repo_section::ID_ROOT,
    unmodelled: &[StateFlag::Empty, StateFlag::Loading, StateFlag::Error, StateFlag::Hover, StateFlag::Focus],
    // The header row plus room for a handful of root workspaces at the
    // default `--roots` — a floor, not a ceiling; `driven_height` returns
    // `None` below.
    min_window_height: 200,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--name`.
    pub name: SharedString,
    /// `--collapsed`.
    pub is_collapsed: bool,
    /// `--renaming`/`--creating-child` (mutually exclusive; the last one
    /// given wins), folded into
    /// [`crowbar_ui::components::row_base::RowMode`] directly rather than
    /// kept as two independent bools — the same fold `crowbar-app/src/
    /// surfaces/workspace_tree_item.rs` makes, needed here too to keep this
    /// struct under clippy's `struct_excessive_bools`.
    pub mode: RowMode,
    /// `--no-default-workspace`.
    pub has_default_workspace: bool,
    /// `--roots`: how many leaf root workspace rows this cell renders.
    /// **Outside `web/src/lib/oracle/extract.ts`'s own declared
    /// `repo-section` scope regardless of count** — the `workspace-tree-
    /// item` family is excluded there unconditionally, `select-item`'s own
    /// reasoning.
    pub roots: u8,
    /// `--pending`: how many root-level pending-create rows this cell
    /// renders.
    pub pending: u8,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            name: SharedString::new_static("crowbar"),
            is_collapsed: false,
            mode: RowMode::Normal,
            has_default_workspace: true,
            roots: 1,
            pending: 0,
        }
    }
}

impl Params {
    /// The section this cell describes. `--flags selected` drives whether
    /// this repo's default workspace is the active one.
    #[must_use]
    pub fn section(&self, cell: &Cell, theme: &Theme) -> RepoSection {
        let roots = (0..self.roots)
            .map(|i| WorkspaceTreeItem {
                branch: SharedString::from(format!("root-{i}")),
                ..WorkspaceTreeItem::fixture()
            })
            .collect();
        let pending_creates = (0..self.pending)
            .map(|i| PendingCreateRow {
                branch: SharedString::from(format!("pending-{i}")),
                error: false,
                // `repo-section.tsx`'s own root-level `paddingLeft: 14`.
                padding_left: px(14.0),
            })
            .collect();

        RepoSection {
            name: self.name.clone(),
            is_active: cell.has(StateFlag::Selected),
            is_collapsed: self.is_collapsed,
            mode: self.mode,
            has_default_workspace: self.has_default_workspace,
            trigger: Trigger::fixture(theme),
            roots,
            pending_creates,
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
            "--name" => self.name = value(args, option)?.into(),
            "--collapsed" => self.is_collapsed = true,
            "--renaming" => self.mode = RowMode::Renaming,
            "--creating-child" => self.mode = RowMode::CreatingChild,
            "--no-default-workspace" => self.has_default_workspace = false,
            "--roots" => self.roots = parse_u8(&value(args, option)?, option)?,
            "--pending" => self.pending = parse_u8(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {}", self.name);
        if cell.has(StateFlag::Selected) {
            out.push_str(" · active");
        }
        if self.is_collapsed {
            out.push_str(" · collapsed");
        }
        match self.mode {
            RowMode::Renaming => out.push_str(" · renaming"),
            RowMode::CreatingChild => out.push_str(" · creating-child"),
            RowMode::Normal => {}
        }
        if !self.has_default_workspace {
            out.push_str(" · no default workspace");
        }
        let _ = write!(out, " · {} root(s)", self.roots);
        if self.pending > 0 {
            let _ = write!(out, " · {} pending", self.pending);
        }
    }

    /// **`false`.** `selected` is a real, driveable flag on this surface —
    /// see the module docs.
    fn no_state_axis(&self) -> bool {
        false
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.section(cell, theme).render(theme, anchors)
    }
}

fn parse_u8(raw: &str, option: &str) -> Result<u8, ParseError> {
    raw.parse().map_err(|_| ParseError::Rejected(format!("{option} takes a whole number 0..=255, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    [
        ("--name <name>".to_owned(), "the repo's own name [crowbar]".to_owned()),
        ("--collapsed".to_owned(), "hide root workspaces [off]".to_owned()),
        ("--renaming".to_owned(), "show the (unpainted) rename input instead of the label [off]".to_owned()),
        ("--creating-child".to_owned(), "show the root-level (unpainted) create input [off]".to_owned()),
        ("--no-default-workspace".to_owned(), "no default workspace — hides the add-child action [off]".to_owned()),
        ("--roots <n>".to_owned(), "how many leaf root workspace rows to render [1]".to_owned()),
        ("--pending <n>".to_owned(), "how many root-level pending-create rows to render [0]".to_owned()),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::Theme;
    use crowbar_ui::components::row_base::RowMode;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "repo-section"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a repo-section cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_one_root_workspace_and_a_default_workspace() {
        let theme = Theme::DARK;
        let default_cell = cell(&[]);
        let section = params_of(&default_cell).section(&default_cell, &theme);
        assert_eq!(section.name, "crowbar");
        assert!(!section.is_active);
        assert!(!section.is_collapsed);
        assert!(section.has_default_workspace);
        assert_eq!(section.roots.len(), 1);
        assert!(section.pending_creates.is_empty());
    }

    #[test]
    fn roots_and_pending_reach_the_section_at_the_root_level_padding() {
        let theme = Theme::DARK;
        let three_roots = cell(&["--roots", "3"]);
        let section = params_of(&three_roots).section(&three_roots, &theme);
        assert_eq!(section.roots.len(), 3);
        for root in &section.roots {
            assert_eq!(root.depth, 0);
        }

        let two_pending = cell(&["--pending", "2"]);
        let section = params_of(&two_pending).section(&two_pending, &theme);
        assert_eq!(section.pending_creates.len(), 2);
        for pending in &section.pending_creates {
            assert_eq!(pending.padding_left, gpui::px(14.0));
        }
    }

    /// `--flags selected` is what drives `isActive`.
    ///
    /// **Mutation, run:** swapped `StateFlag::Selected` for
    /// `StateFlag::Hover` on `Params::section`'s own `is_active` field.
    /// `cargo test -p crowbar-app --bin crowbar-app
    /// flags_selected_drives_is_active` failed as predicted: `assertion
    /// failed: params_of(&selected).section(&selected, \&theme).is_active`.
    /// Reverted after confirming.
    #[test]
    fn flags_selected_drives_is_active() {
        let theme = Theme::DARK;
        let resting = cell(&[]);
        assert!(!params_of(&resting).section(&resting, &theme).is_active);

        let selected = cell(&["--flags", "selected"]);
        assert!(params_of(&selected).section(&selected, &theme).is_active);
    }

    #[test]
    fn every_bool_option_reaches_the_section_independently() {
        let theme = Theme::DARK;
        assert!(params_of(&cell(&["--collapsed"])).section(&cell(&["--collapsed"]), &theme).is_collapsed);
        assert_eq!(
            params_of(&cell(&["--renaming"])).section(&cell(&["--renaming"]), &theme).mode,
            RowMode::Renaming
        );
        assert_eq!(
            params_of(&cell(&["--creating-child"]))
                .section(&cell(&["--creating-child"]), &theme)
                .mode,
            RowMode::CreatingChild
        );
        assert!(
            !params_of(&cell(&["--no-default-workspace"]))
                .section(&cell(&["--no-default-workspace"]), &theme)
                .has_default_workspace
        );
    }

    #[test]
    fn the_unmodelled_list_is_everything_but_selected() {
        for flag in [StateFlag::Empty, StateFlag::Loading, StateFlag::Error, StateFlag::Hover, StateFlag::Focus] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        assert!(!Params::default().no_state_axis());
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--name", "--collapsed", "--roots", "--pending"] {
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
        assert_eq!(SURFACE.name, "repo-section");
        assert_eq!(SURFACE.root, crowbar_ui::components::repo_section::ID_ROOT);
        assert!(!SURFACE.full_bleed);
    }
}
