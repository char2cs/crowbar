//! `--surface workspace-tree-item` — one row of the workspace tree, plus,
//! via `--children`/`--pending`, its own nested rows.
//!
//! `crowbar_ui::components::workspace_tree_item` carries the composition
//! (why the status icon composition is only oracle-safe for a leaf cell,
//! why `hasChildren`/`isCreatingChild`/`showPlaceholderDetails` are derived
//! rather than stored); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real**, via `--flags selected` — `isActive`, the same reading every other row-shaped surface in this port gives it. |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled** — see `crowbar_ui::components::workspace_tree_item`'s own module docs. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::pending_create_row::PendingCreateRow;
use crowbar_ui::components::workspace_branch_icon::Status;
use crowbar_ui::components::workspace_tree_item::WorkspaceTreeItem;
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "workspace-tree-item",
    root: crowbar_ui::components::workspace_tree_item::ID_ROOT,
    unmodelled: &[StateFlag::Empty, StateFlag::Loading, StateFlag::Error, StateFlag::Hover, StateFlag::Focus],
    // The row's own `h-9` plus enough room for a few nested children and
    // pending rows at the default `--children`/`--pending` — a floor, not a
    // ceiling; `driven_height` returns `None` below.
    min_window_height: 240,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--branch`.
    pub branch: SharedString,
    /// `--row-depth`.
    pub depth: u8,
    /// `--status`.
    pub status: Status,
    /// `--working`.
    pub working: bool,
    /// `--placeholder`.
    pub is_placeholder: bool,
    /// `--expanded`.
    pub expanded: bool,
    /// `--renaming`.
    pub is_renaming: bool,
    /// `--creating-child`.
    pub is_creating_child: bool,
    /// `--added-count`.
    pub added: Option<u32>,
    /// `--deleted-count`.
    pub deleted: Option<u32>,
    /// `--children`: how many leaf child rows this cell renders. **Non-zero
    /// values are outside `web/src/lib/oracle/extract.ts`'s own declared
    /// `workspace-tree-item` scope** — `crowbar_ui::components::
    /// workspace_tree_item`'s own module docs record why (the recursive
    /// `workspace-tree-item`/`workspace-branch-icon` ids repeat).
    pub children: u8,
    /// `--pending`: how many optimistic pending-create rows this cell
    /// renders as this node's own children.
    pub pending: u8,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            branch: SharedString::new_static("feature/example"),
            depth: 0,
            status: Status::New,
            working: false,
            is_placeholder: false,
            expanded: true,
            is_renaming: false,
            is_creating_child: false,
            added: None,
            deleted: None,
            children: 0,
            pending: 0,
        }
    }
}

impl Params {
    /// The row this cell describes. `--flags selected` drives `isActive` —
    /// `StateFlag::Selected` is exactly the "is this row's own active
    /// picture on" concept for a single row, `project_home_row.rs`'s own
    /// reading.
    #[must_use]
    pub fn row(&self, cell: &Cell) -> WorkspaceTreeItem {
        let children = (0..self.children)
            .map(|i| WorkspaceTreeItem {
                branch: SharedString::from(format!("{}-child-{i}", self.branch)),
                depth: u32::from(self.depth) + 1,
                ..WorkspaceTreeItem::fixture()
            })
            .collect();
        // `(depth + 2) * 14` — `WorkspaceTreeItem::nested_padding_left`'s
        // own arithmetic, restated here because `Params::row` builds
        // `PendingCreateRow`s directly rather than through the parent row's
        // own method.
        let nested_padding = px((f32::from(self.depth) + 2.0) * 14.0);
        let pending_creates = (0..self.pending)
            .map(|i| PendingCreateRow {
                branch: SharedString::from(format!("pending-{i}")),
                error: false,
                padding_left: nested_padding,
            })
            .collect();

        WorkspaceTreeItem {
            branch: self.branch.clone(),
            depth: u32::from(self.depth),
            status: self.status,
            working: self.working,
            is_placeholder: self.is_placeholder,
            is_active: cell.has(StateFlag::Selected),
            expanded: self.expanded,
            is_renaming: self.is_renaming,
            is_creating_child: self.is_creating_child,
            added: self.added,
            deleted: self.deleted,
            children,
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
            "--branch" => self.branch = value(args, option)?.into(),
            "--row-depth" => self.depth = parse_u8(&value(args, option)?, option)?,
            "--status" => self.status = parse_status(&value(args, option)?)?,
            "--working" => self.working = true,
            "--placeholder" => self.is_placeholder = true,
            "--expanded" => self.expanded = true,
            "--collapsed" => self.expanded = false,
            "--renaming" => self.is_renaming = true,
            "--creating-child" => self.is_creating_child = true,
            "--added-count" => self.added = Some(parse_u32(&value(args, option)?, option)?),
            "--deleted-count" => self.deleted = Some(parse_u32(&value(args, option)?, option)?),
            "--children" => self.children = parse_u8(&value(args, option)?, option)?,
            "--pending" => self.pending = parse_u8(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {} · depth {} · {}", self.branch, self.depth, self.status.name());
        if self.working {
            out.push_str(" · working");
        }
        if self.is_placeholder {
            out.push_str(" · placeholder");
        }
        if cell.has(StateFlag::Selected) {
            out.push_str(" · active");
        }
        if self.expanded {
            out.push_str(" · expanded");
        }
        if self.is_renaming {
            out.push_str(" · renaming");
        }
        if self.is_creating_child {
            out.push_str(" · creating-child");
        }
        if let Some(n) = self.added {
            let _ = write!(out, " · +{n}");
        }
        if let Some(n) = self.deleted {
            let _ = write!(out, " · -{n}");
        }
        if self.children > 0 {
            let _ = write!(out, " · {} children", self.children);
        }
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
        self.row(cell).render(theme, anchors)
    }
}

fn parse_u8(raw: &str, option: &str) -> Result<u8, ParseError> {
    raw.parse().map_err(|_| ParseError::Rejected(format!("{option} takes a whole number 0..=255, not {raw}")))
}

fn parse_u32(raw: &str, option: &str) -> Result<u32, ParseError> {
    raw.parse().map_err(|_| ParseError::Rejected(format!("{option} takes a whole number, not {raw}")))
}

fn parse_status(raw: &str) -> Result<Status, ParseError> {
    crowbar_ui::components::workspace_branch_icon::ALL_STATUSES
        .into_iter()
        .find(|s| s.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--status takes one of {}, not {raw}",
                crowbar_ui::components::workspace_branch_icon::ALL_STATUSES
                    .iter()
                    .map(|s| s.name())
                    .collect::<Vec<_>>()
                    .join(", "),
            ))
        })
}

fn options() -> Vec<(String, String)> {
    [
        ("--branch <name>".to_owned(), "the branch name [feature/example]".to_owned()),
        ("--row-depth <n>".to_owned(), "how many ancestors this row has [0]".to_owned()),
        ("--status <s>".to_owned(), "the workspace status (new, locked, pr-open, ...) [new]".to_owned()),
        ("--working".to_owned(), "show the agent-in-flight spinner instead of the status glyph [off]".to_owned()),
        ("--placeholder".to_owned(), "show the placeholder warning glyph [off]".to_owned()),
        ("--expanded".to_owned(), "show children (if any) [on]".to_owned()),
        ("--collapsed".to_owned(), "hide children (if any) [off]".to_owned()),
        ("--renaming".to_owned(), "show the (unpainted) rename input instead of the label [off]".to_owned()),
        ("--creating-child".to_owned(), "show the (unpainted) create-child input instead of the +New button [off]".to_owned()),
        ("--added-count <n>".to_owned(), "the added-lines count, shown only when active [none]".to_owned()),
        ("--deleted-count <n>".to_owned(), "the deleted-lines count, shown only when active [none]".to_owned()),
        ("--children <n>".to_owned(), "how many leaf child rows to render [0]".to_owned()),
        ("--pending <n>".to_owned(), "how many pending-create rows to render as this node's children [0]".to_owned()),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::workspace_branch_icon::Status;
    use crowbar_ui::components::workspace_tree_item::WorkspaceTreeItem;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "workspace-tree-item"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a workspace-tree-item cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_a_leaf_row_matching_the_component_fixture() {
        let default_cell = cell(&[]);
        let row = params_of(&default_cell).row(&default_cell);
        assert_eq!(row.branch, WorkspaceTreeItem::fixture().branch);
        assert_eq!(row.depth, 0);
        assert_eq!(row.status, Status::New);
        assert!(!row.working);
        assert!(!row.is_active);
        assert!(!row.has_children());
        assert!(row.pending_creates.is_empty());
    }

    #[test]
    fn children_and_pending_reach_the_row_at_the_right_depth() {
        let three_children = cell(&["--row-depth", "1", "--children", "3"]);
        let row = params_of(&three_children).row(&three_children);
        assert_eq!(row.children.len(), 3);
        assert!(row.has_children());
        for child in &row.children {
            assert_eq!(child.depth, 2);
        }

        let two_pending = cell(&["--row-depth", "1", "--pending", "2"]);
        let row = params_of(&two_pending).row(&two_pending);
        assert_eq!(row.pending_creates.len(), 2);
        for pending in &row.pending_creates {
            // (1 + 2) * 14
            assert_eq!(pending.padding_left, px(42.0));
        }
    }

    /// `--flags selected` is what drives `isActive` — not a bespoke option.
    ///
    /// **Mutation, run:** swapped `StateFlag::Selected` for
    /// `StateFlag::Hover` on `Params::row`'s own `is_active` field.
    /// `cargo test -p crowbar-app --bin crowbar-app
    /// flags_selected_drives_is_active` failed as predicted: `assertion
    /// failed: params_of(&selected).row(&selected).is_active`. Reverted
    /// after confirming.
    #[test]
    fn flags_selected_drives_is_active() {
        let resting = cell(&[]);
        assert!(!params_of(&resting).row(&resting).is_active);

        let selected = cell(&["--flags", "selected"]);
        assert!(params_of(&selected).row(&selected).is_active);
    }

    #[test]
    fn every_bool_option_reaches_the_row_independently() {
        assert!(params_of(&cell(&["--working"])).row(&cell(&["--working"])).working);
        assert!(params_of(&cell(&["--placeholder"])).row(&cell(&["--placeholder"])).is_placeholder);
        assert!(params_of(&cell(&["--renaming"])).row(&cell(&["--renaming"])).is_renaming);
        assert!(
            params_of(&cell(&["--creating-child"])).row(&cell(&["--creating-child"])).is_creating_child
        );
        assert!(!params_of(&cell(&["--collapsed"])).row(&cell(&["--collapsed"])).expanded);
        assert_eq!(params_of(&cell(&["--added-count", "12"])).row(&cell(&["--added-count", "12"])).added, Some(12));
        assert_eq!(
            params_of(&cell(&["--deleted-count", "3"])).row(&cell(&["--deleted-count", "3"])).deleted,
            Some(3)
        );
    }

    #[test]
    fn status_rejects_an_unknown_word() {
        let line = ["--surface", "workspace-tree-item", "--status", "not-a-status"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(crate::row_surface::ParseError::Rejected(_)),
        ));
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
        for option in ["--branch", "--row-depth", "--status", "--working", "--children", "--pending"] {
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
        assert_eq!(SURFACE.name, "workspace-tree-item");
        assert_eq!(SURFACE.root, crowbar_ui::components::workspace_tree_item::ID_ROOT);
        assert!(!SURFACE.full_bleed);
    }
}
