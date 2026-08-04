//! `--surface workspace-tree` — the project-home row plus, via `--repos`, a
//! scrolling column of repo sections.
//!
//! `crowbar_ui::surfaces::workspace::workspace_tree` carries the composition (why
//! `WorkspaceTreeFooter` is omitted, why `ScrollArea::render` is not
//! called); this file is the cell.
//!
//! # Two cell axes on the composed `project-home-row`
//!
//! Before this item, this cell had no way to drive either of the composed
//! home row's two real fields, which made this surface permanently
//! un-passable against a live app on both:
//!
//! * **`--project-name`** overrides the row's own name, the same flag
//!   `project-home-row`'s own surface (`surfaces/project_home_row.rs`)
//!   takes, threaded through this composition — the shape P3.64 already
//!   established for `project-switcher-panel`'s row 0
//!   (`surfaces/project_switcher_panel.rs`'s own `project_name` field).
//!   Without it, `project-home-row-label.text` could only ever read the
//!   `'home'` fixture fallback (`native/mapping/workspace-tree.md`'s own
//!   verdict, cause C: `text: "home", expected "oracle-fixture"`).
//! * **`--home-active`** sets the row's own `isActive`, `project-switcher-
//!   panel`'s `--active-index`/`--no-active` precedent — simplified to a
//!   bare switch because this surface composes exactly *one* such row
//!   rather than a list of them, so there is nothing for an index to
//!   select between. Without it, `project-home-row.bg`/`border.color` could
//!   only ever read `row_base::inactive`'s transparent resting picture,
//!   never the live app's `#1f1f1eff` active one
//!   (`native/mapping/workspace-tree.md`'s own verdict, cause B).
//!
//! Both default to the composed row's own idle fixture (`'home'`,
//! inactive) — a bare `--surface workspace-tree` is unchanged by either
//! flag's existence.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled** — see `crowbar_ui::surfaces::workspace::workspace_tree`'s own module docs. `--home-active` above is a bespoke, surface-specific option rather than a rule on `StateFlag::Selected`, exactly as `project-switcher-panel`'s `--active-index` is: this container's own `selected` stays unmodelled (it has no selection picture of its own), and the flag is about the *row it composes*, not about itself. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::project_home_row::ProjectHomeRow;
use crowbar_ui::surfaces::repo::repo_section::RepoSection;
use crowbar_ui::surfaces::rows::row_base;
use crowbar_ui::surfaces::workspace::workspace_tree::WorkspaceTree;
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "workspace-tree",
    root: crowbar_ui::surfaces::workspace::workspace_tree::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `project-home-row`'s own `h-9` plus its margin plus the scroll area's
    // own share of the column (`Params::tree`'s own derivation) — a floor,
    // not a ceiling; `driven_height` returns `None` below.
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
    /// `--project-name`: overrides the composed `project-home-row`'s own
    /// name. `None` leaves it on `ProjectHomeRow::fixture`'s own `'home'`
    /// fallback. See the module docs.
    pub project_name: Option<SharedString>,
    /// `--home-active`: the composed `project-home-row`'s own `isActive`.
    /// See the module docs.
    pub home_active: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            repos: 1,
            project_name: None,
            home_active: false,
        }
    }
}

impl Params {
    /// The tree this cell describes. `--project-name`/`--home-active`
    /// override the composed home row's own two fields; see the module
    /// docs.
    #[must_use]
    pub fn tree(&self, cell: &Cell, theme: &Theme) -> WorkspaceTree {
        let sections = (0..self.repos)
            .map(|i| RepoSection {
                name: SharedString::from(format!("repo-{i}")),
                ..RepoSection::fixture(theme)
            })
            .collect();

        let mut project_home = ProjectHomeRow::fixture();
        project_home.is_active = self.home_active;
        if let Some(name) = &self.project_name {
            project_home.project_name = name.clone();
        }

        WorkspaceTree {
            project_home,
            sections,
            // `ScrollAreaPrimitive.Root` is `size-full` inside the call
            // site's own `flex-1` column
            // (`web/src/components/layout/workspace-tree.tsx:71`,
            // `<ScrollArea className="flex-1">`) — its width *is* the
            // sidebar column's, the same quantity the tree root already
            // tracks (`WorkspaceTree::render`'s `.flex_1()`, filling this
            // cell's own `--width`). A fixed `px(344.0)` here disagreed
            // with the root the instant a cell drove a different width;
            // `native/QUEUE.md`'s "workspace-tree hardcodes its scroll
            // width" entry.
            scroll_width: cell.width_px(),
            // The scroll area's own share of vertical space in a column
            // whose total height is fixed by its container, not by its
            // children's sum — the same `flex-1`-in-a-fixed-height
            // reasoning `scroll_width` above just stopped needing (width
            // there is the containing block's; height here is what is
            // *left over* after the row above it). `936` was `ScrollArea::
            // fixture`'s own live measurement, taken before `WorkspaceTree::
            // home_row` existed: `mx-1.5 my-0.5` has been on `ROW_BASE`
            // since the file was created
            // (`web/src/components/layout/workspace-row-base.ts:2`,
            // `6d7e51f2`), so the live scroll area's real height already
            // had `row_base::MARGIN_Y * 2` (P3.66, `d7287c76`) taken out of
            // it — the port's `936` just never had that subtraction done to
            // it, because P3.66 added the row's own margin without touching
            // this sibling field. Deriving it from the constant rather than
            // writing `932` keeps the two in sync if `MARGIN_Y` ever moves.
            scroll_height: px(936.0) - row_base::MARGIN_Y * 2.0,
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
            "--project-name" => self.project_name = Some(value(args, option)?.into()),
            "--home-active" => self.home_active = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {} repo(s)", self.repos);
        if let Some(name) = &self.project_name {
            let _ = write!(out, " · home named {name}");
        }
        if self.home_active {
            out.push_str(" · home active");
        }
    }

    /// **`true`.** None of the six §8.3 flags has a rule on this surface —
    /// see the module docs.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.tree(cell, theme).render(theme, anchors)
    }
}

fn parse_u8(raw: &str, option: &str) -> Result<u8, ParseError> {
    raw.parse().map_err(|_| ParseError::Rejected(format!("{option} takes a whole number 0..=255, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    [
        ("--repos <n>".to_owned(), "how many repo sections to render [1]".to_owned()),
        (
            "--project-name <name>".to_owned(),
            "overrides the composed project-home-row's own name — the live app's real \
             project name, since the fixture otherwise reads 'home' [home]"
                .to_owned(),
        ),
        (
            "--home-active".to_owned(),
            "the composed project-home-row renders active (isActive true) [inactive]".to_owned(),
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
    fn the_default_is_one_repo_and_the_home_row_stays_on_its_own_idle_fixture() {
        let theme = Theme::DARK;
        let bag = Params::default();
        assert_eq!(bag.project_name, None);
        assert!(!bag.home_active);

        let bare = cell(&[]);
        let tree = params_of(&bare).tree(&bare, &theme);
        assert_eq!(tree.sections.len(), 1);
        assert_eq!(tree.sections[0].name, "repo-0");
        assert_eq!(tree.project_home.project_name, "home");
        assert!(!tree.project_home.is_active);
    }

    #[test]
    fn repos_reaches_the_tree() {
        let theme = Theme::DARK;
        let three_repos = cell(&["--repos", "3"]);
        let tree = params_of(&three_repos).tree(&three_repos, &theme);
        assert_eq!(tree.sections.len(), 3);
        assert_eq!(tree.sections[2].name, "repo-2");
    }

    /// **`--project-name` overrides the composed home row's own name only**
    /// — the exact gap `native/mapping/workspace-tree.md`'s own verdict
    /// hit: `project-home-row-label.text: "home", expected
    /// "oracle-fixture"`, with no flag able to close it.
    #[test]
    fn project_name_overrides_the_composed_home_row() {
        let theme = Theme::DARK;
        let named = cell(&["--project-name", "oracle-fixture"]);
        assert_eq!(
            params_of(&named).tree(&named, &theme).project_home.project_name,
            "oracle-fixture"
        );

        let unset = cell(&[]);
        assert_eq!(
            params_of(&unset).tree(&unset, &theme).project_home.project_name,
            "home",
            "no --project-name leaves the composed row on its own fixture fallback",
        );
    }

    /// **`--home-active` sets the composed home row's own `isActive`** —
    /// the exact gap `native/mapping/workspace-tree.md`'s own verdict hit:
    /// `project-home-row.bg`/`border.color` could only ever read
    /// `row_base::inactive`'s resting picture, never the live app's active
    /// one.
    #[test]
    fn home_active_reaches_the_composed_row() {
        let theme = Theme::DARK;
        let inactive = cell(&[]);
        assert!(!params_of(&inactive).tree(&inactive, &theme).project_home.is_active);

        let active = cell(&["--home-active"]);
        assert!(params_of(&active).tree(&active, &theme).project_home.is_active);
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
        for option in ["--repos", "--project-name", "--home-active"] {
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
        assert_eq!(SURFACE.name, "workspace-tree");
        assert_eq!(SURFACE.root, crowbar_ui::surfaces::workspace::workspace_tree::ID_ROOT);
        assert!(!SURFACE.full_bleed);
    }
}
