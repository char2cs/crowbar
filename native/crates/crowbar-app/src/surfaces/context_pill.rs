//! `--surface context-pill` — the sidebar's "you are here" pill: a
//! two-line repo/branch (or project/home) label and a trailing identity
//! glyph, or a bare project name.
//!
//! `crowbar_ui::surfaces::context_pill` carries the full account of this
//! composition's own arithmetic and of why it does not compose
//! `button::Button::render`. This file is the cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `context-pill.tsx`'s own trigger carries only `hover:bg-sidebar-element-hover` — a colour-only rule with no field on this contract — and no `focus:`/`data-active` rule at all. `--kind`, `--status` and `--working` are this surface's own axis instead, the same shape `fps-overlay`'s `--fps`/`--max-dt`/`--drops` are. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::context_pill::ContextPill;
use crowbar_ui::surfaces::repo::repo_avatar::{Kind as RepoAvatarKind, RepoAvatar, Size as RepoAvatarSize};
use crowbar_ui::surfaces::workspace::workspace_branch_icon::Status;
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "context-pill",
    root: crowbar_ui::surfaces::context_pill::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The trigger's own two-line stack (36px) plus its `py-1.5` padding
    // (12px) plus the wrapper's own `pb-1` (4px) is 52px; a floor with room
    // rather than a ceiling — `driven_height` returns `None` below, so the
    // window follows whatever this content-driven pill actually comes out
    // at, the same posture `command`'s own surface takes.
    min_window_height: 90,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--kind`'s closed vocabulary.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    /// A workspace context.
    Workspace,
    /// A project's home context.
    Home,
    /// An active project only, no workspace and no home route.
    Project,
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--kind`.
    pub kind: Kind,
    /// `--status`: only read when `--kind workspace`.
    pub status: Status,
    /// `--working`: the agent-in-flight flag, read on `--kind workspace` and
    /// `--kind home`.
    pub working: bool,
    /// `--avatar`: gives a `--kind workspace` cell a repo avatar (the
    /// default-workspace picture) instead of the status glyph.
    pub avatar: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            kind: Kind::Workspace,
            status: Status::New,
            working: false,
            avatar: false,
        }
    }
}

impl Params {
    /// The pill this cell describes.
    #[must_use]
    pub fn pill(&self) -> ContextPill {
        match self.kind {
            Kind::Workspace => ContextPill::Workspace {
                status: self.status,
                working: self.working,
                repo_name: SharedString::new_static("crowbar"),
                branch_name: SharedString::new_static("main"),
                repo_avatar: self.avatar.then(|| RepoAvatar {
                    kind: RepoAvatarKind::Letter,
                    size: RepoAvatarSize::Lg,
                    label: SharedString::new_static("C"),
                    emoji: SharedString::new_static("\u{1f98a}"),
                    background: crowbar_ui::Color::TRANSPARENT,
                }),
            },
            Kind::Home => ContextPill::Home {
                project_name: SharedString::new_static("crowbar"),
                working: self.working,
            },
            Kind::Project => ContextPill::Project {
                project_name: SharedString::new_static("crowbar"),
            },
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
            "--kind" => self.kind = parse_kind(&value(args, option)?)?,
            "--status" => self.status = parse_status(&value(args, option)?)?,
            "--working" => self.working = true,
            "--avatar" => self.avatar = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(out, " · kind {}", self.kind.name());
        if matches!(self.kind, Kind::Workspace) {
            let _ = write!(out, " · status {}", self.status.name());
        }
        if matches!(self.kind, Kind::Workspace | Kind::Home) && self.working {
            out.push_str(" · working");
        }
        if self.avatar {
            out.push_str(" · avatar");
        }
    }

    /// **`true`, unconditionally.** See the module docs and this surface's
    /// own `unmodelled` list.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.pill().render(theme, anchors)
    }
}

impl Kind {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Workspace => "workspace",
            Self::Home => "home",
            Self::Project => "project",
        }
    }
}

fn parse_kind(raw: &str) -> Result<Kind, ParseError> {
    match raw {
        "workspace" => Ok(Kind::Workspace),
        "home" => Ok(Kind::Home),
        "project" => Ok(Kind::Project),
        other => Err(ParseError::Rejected(format!(
            "--kind takes workspace, home or project, not {other}",
        ))),
    }
}

/// `--status`'s closed vocabulary — the identical spelling
/// `workspace_branch_icon::Status::name` uses.
fn parse_status(raw: &str) -> Result<Status, ParseError> {
    match raw {
        "locked" => Ok(Status::Locked),
        "new" => Ok(Status::New),
        "pr-conflicts" => Ok(Status::PrConflicts),
        "deleted" => Ok(Status::Deleted),
        "pr-open" => Ok(Status::PrOpen),
        "pr-closed" => Ok(Status::PrClosed),
        "pr-merged" => Ok(Status::PrMerged),
        other => Err(ParseError::Rejected(format!(
            "--status takes a WorkspaceStatus, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--kind <workspace|home|project>".to_owned(),
            "which of the three live pictures this cell shows [workspace]".to_owned(),
        ),
        (
            "--status <status>".to_owned(),
            "the workspace status glyph, --kind workspace only [new]".to_owned(),
        ),
        (
            "--working".to_owned(),
            "the agent-in-flight spinner, --kind workspace/home only [idle]".to_owned(),
        ),
        (
            "--avatar".to_owned(),
            "shows the repo avatar instead of the status glyph, --kind workspace only [status glyph]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Kind, Params, SURFACE, options, parse_kind, parse_status};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::surfaces::context_pill::ContextPill;
    use crowbar_ui::surfaces::workspace::workspace_branch_icon::Status;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "context-pill"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a context-pill cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_a_plain_idle_workspace() {
        let bag = Params::default();
        assert_eq!(bag.kind, Kind::Workspace);
        assert_eq!(bag.status, Status::New);
        assert!(!bag.working);
        assert!(!bag.avatar);

        let pill = params_of(&cell(&[])).pill();
        assert!(matches!(pill, ContextPill::Workspace { .. }));
    }

    #[test]
    fn every_option_reaches_the_pill_independently() {
        let ContextPill::Workspace { status, working, repo_avatar, .. } =
            params_of(&cell(&["--status", "pr-open", "--working", "--avatar"])).pill()
        else {
            panic!("--kind workspace by default");
        };
        assert_eq!(status, Status::PrOpen);
        assert!(working);
        assert!(repo_avatar.is_some());

        assert!(matches!(
            params_of(&cell(&["--kind", "home", "--working"])).pill(),
            ContextPill::Home { working: true, .. }
        ));
        assert!(matches!(
            params_of(&cell(&["--kind", "project"])).pill(),
            ContextPill::Project { .. }
        ));
    }

    #[test]
    fn kind_parses_its_closed_vocabulary() {
        assert_eq!(parse_kind("workspace"), Ok(Kind::Workspace));
        assert_eq!(parse_kind("home"), Ok(Kind::Home));
        assert_eq!(parse_kind("project"), Ok(Kind::Project));
        assert!(matches!(parse_kind("bogus"), Err(ParseError::Rejected(_))));
    }

    #[test]
    fn status_parses_its_closed_vocabulary() {
        assert_eq!(parse_status("locked"), Ok(Status::Locked));
        assert!(matches!(parse_status("bogus"), Err(ParseError::Rejected(_))));
    }

    #[test]
    fn a_non_vocabulary_kind_is_rejected_by_name() {
        let line = ["--surface", "context-pill", "--kind", "bogus"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
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
        for option in ["--kind", "--status", "--working", "--avatar"] {
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
        for option in ["--kind", "--status", "--working", "--avatar"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "context-pill");
        assert_eq!(SURFACE.root, "context-pill");
        assert!(!SURFACE.full_bleed);
    }
}
