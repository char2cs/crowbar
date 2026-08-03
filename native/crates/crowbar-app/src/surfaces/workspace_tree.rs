//! `--surface workspace-tree` — the project-home row plus, via `--repos`, a
//! scrolling column of repo sections.
//!
//! `crowbar_ui::components::workspace_tree` carries the composition (why
//! `WorkspaceTreeFooter` is omitted, why `ScrollArea::render` is not
//! called); this file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled** — see `crowbar_ui::components::workspace_tree`'s own module docs. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::repo_section::RepoSection;
use crowbar_ui::components::workspace_tree::WorkspaceTree;
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "workspace-tree",
    root: crowbar_ui::components::workspace_tree::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `project-home-row`'s own `h-9` plus the `ScrollArea` fixture's live
    // `936` — a floor, not a ceiling; `driven_height` returns `None` below.
    min_window_height: 980,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--repos`: how many repo sections this cell renders. **Outside
    /// `web/src/lib/oracle/extract.ts`'s own declared `workspace-tree`
    /// scope regardless of count** — `repo-section` is excluded there
    /// unconditionally, `resizable`'s own precedent for a container whose
    /// repeated content is verified through that content's own surface.
    pub repos: u8,
}

impl Default for Params {
    fn default() -> Self {
        Self { repos: 1 }
    }
}

impl Params {
    /// The tree this cell describes.
    #[must_use]
    pub fn tree(&self, theme: &Theme) -> WorkspaceTree {
        let sections = (0..self.repos)
            .map(|i| RepoSection {
                name: SharedString::from(format!("repo-{i}")),
                ..RepoSection::fixture(theme)
            })
            .collect();

        WorkspaceTree {
            sections,
            // `ScrollArea::fixture`'s own live `workspace-tree` measurement.
            scroll_width: px(344.0),
            scroll_height: px(936.0),
            ..WorkspaceTree::fixture(theme)
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
            "--repos" => self.repos = parse_u8(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {} repo(s)", self.repos);
    }

    /// **`true`.** None of the six §8.3 flags has a rule on this surface —
    /// see the module docs.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.tree(theme).render(theme, anchors)
    }
}

fn parse_u8(raw: &str, option: &str) -> Result<u8, ParseError> {
    raw.parse().map_err(|_| ParseError::Rejected(format!("{option} takes a whole number 0..=255, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    [("--repos <n>".to_owned(), "how many repo sections to render [1]".to_owned())]
        .into_iter()
        .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::Theme;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "workspace-tree"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a workspace-tree cell carries this surface's bag")
    }

    #[test]
    fn the_default_is_one_repo() {
        let theme = Theme::DARK;
        let tree = params_of(&cell(&[])).tree(&theme);
        assert_eq!(tree.sections.len(), 1);
        assert_eq!(tree.sections[0].name, "repo-0");
    }

    #[test]
    fn repos_reaches_the_tree() {
        let theme = Theme::DARK;
        let three = cell(&["--repos", "3"]);
        let tree = params_of(&three).tree(&theme);
        assert_eq!(tree.sections.len(), 3);
        assert_eq!(tree.sections[2].name, "repo-2");
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
    fn this_surfaces_options_are_rejected_on_another_surface() {
        let line = ["--surface", "skeleton", "--repos"];
        assert!(Cell::parse(line.iter().map(|arg| (*arg).to_owned())).is_err());
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
        assert_eq!(SURFACE.name, "workspace-tree");
        assert_eq!(SURFACE.root, crowbar_ui::components::workspace_tree::ID_ROOT);
        assert!(!SURFACE.full_bleed);
    }
}
