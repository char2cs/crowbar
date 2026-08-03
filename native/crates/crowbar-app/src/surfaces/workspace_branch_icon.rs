//! `--surface workspace-branch-icon` — the sidebar row's status glyph, one of
//! two P3.50 foundation leaves. See
//! `crowbar_ui::components::workspace_branch_icon`'s module docs for the
//! shape (single swapped element, no persistent wrapper), the "seven statuses,
//! five pictures" finding, and — the reason this surface's `empty` cell is
//! synthetic rather than a real branch — the finding that this component
//! takes no `className` and spreads no props anywhere.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--status` / `--working` / `--placeholder` | **real** — the eight-way branch, in precedence order |
//! | `--theme` | **real for five of six colours.** `theme.foreground` and `theme.warning` move across appearances; `RED_500`/`GREEN_500`/`VIOLET_500` are raw Tailwind literals and do not |
//! | `--content` / `--width` / `--viewport-width` | **vacuous.** No box here paints text, and every box is authored `size-4` |
//!
//! # The state axis
//!
//! `empty` is the one flag kept modelled, and it is **synthetic** — see the
//! component's own module docs and its `empty` field's doc comment for why:
//! measured exhaustively, none of the six §8.3 flags has a real branch on
//! this surface. `hover`/`focus`/`selected` stay unmodelled.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::workspace_branch_icon::{self, ALL_STATUSES, Status, WorkspaceBranchIcon};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "workspace-branch-icon",
    root: workspace_branch_icon::ID,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // No `hover:`/`focus:`/`data-active` rule anywhere in
        // `workspace-branch-icon.tsx` — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `size-4`'s 16px, plus `CAPTION_HEIGHT`'s 29. A floor, not a ceiling.
    min_window_height: 64,
    // A glyph beside a workspace name in the sidebar tree — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--status`: `WorkspaceStatus`, read only once `--working` and
    /// `--placeholder` are both off.
    pub status: Status,
    /// `--working`: checked first; overrides everything else.
    pub working: bool,
    /// `--placeholder`: checked second, ahead of the status switch.
    pub placeholder: bool,
}

impl Default for Params {
    /// A bare `--surface workspace-branch-icon` is the component's own
    /// representative cell — `avatar`'s convention — not each field's
    /// independent default.
    fn default() -> Self {
        let fixture = WorkspaceBranchIcon::fixture();
        Self {
            status: fixture.status,
            working: fixture.working,
            placeholder: fixture.is_placeholder,
        }
    }
}

impl Params {
    /// The icon this cell describes.
    #[must_use]
    pub fn icon(&self, cell: &Cell) -> WorkspaceBranchIcon {
        WorkspaceBranchIcon {
            status: self.status,
            working: self.working,
            is_placeholder: self.placeholder,
            empty: cell.has(StateFlag::Empty),
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
            "--status" => self.status = parse_status(&value(args, option)?)?,
            "--working" => self.working = true,
            "--placeholder" => self.placeholder = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The box is `size-4` always, whatever the cell.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let icon = self.icon(cell);
        if let Some(glyph) = icon.glyph() {
            let _ = write!(out, " · glyph {}", glyph.name());
        } else {
            out.push_str(" · the flip-dot spinner (working)");
        }
        if self.placeholder && !self.working {
            out.push_str(" · isPlaceholder ahead of the status switch");
        }
        if self.working {
            out.push_str(" · working beats isPlaceholder and the status switch");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: synthetic — this component spreads no prop anywhere, so no \
                 live call site can collapse the box; see the module docs",
            );
        }
    }

    /// The icon, inside the flex row that makes it a flex item — every live
    /// call site renders it as a sibling of a workspace name.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_start()
            .child(self.icon(cell).render(theme, anchors))
            .into_any_element()
    }
}

fn parse_status(raw: &str) -> Result<Status, ParseError> {
    ALL_STATUSES
        .into_iter()
        .find(|status| status.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--status takes one of {}, not {raw}",
                names(ALL_STATUSES.into_iter().map(Status::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--status <name>".to_owned(),
            format!(
                "one of {} — WorkspaceStatus's own spelling, read only with neither \
                 --working nor --placeholder [{}]",
                names(ALL_STATUSES.into_iter().map(Status::name)),
                WorkspaceBranchIcon::fixture().status.name(),
            ),
        ),
        (
            "--working".to_owned(),
            "the flip-dot spinner; beats --placeholder and --status [off]".to_owned(),
        ),
        (
            "--placeholder".to_owned(),
            "the warning glyph, ahead of the status switch; beaten by --working [off]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE};
    use crate::row_surface::{Cell, StateFlag};
    use crowbar_ui::components::workspace_branch_icon::{self, Glyph, Status};

    fn a_cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "workspace-branch-icon"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
    }

    fn params(cell: &Cell) -> Params {
        cell.surface_params::<Params>()
            .expect("workspace-branch-icon's own bag")
            .clone()
    }

    /// The surface's root is the primitive's own single anchor.
    #[test]
    fn the_root_anchor_is_the_primitives_own_id() {
        assert_eq!(SURFACE.root, "workspace-branch-icon");
        assert_eq!(workspace_branch_icon::ID, "workspace-branch-icon");
    }

    /// A bare `--surface workspace-branch-icon` is the component's own
    /// fixture: `new`, idle, no placeholder.
    #[test]
    fn the_default_cell_is_the_components_own_fixture() {
        let cell = a_cell(&[]);
        let icon = params(&cell).icon(&cell);
        assert_eq!(icon.status, Status::New);
        assert!(!icon.working);
        assert!(!icon.is_placeholder);
        assert_eq!(icon.glyph(), Some(Glyph::GitBranch));
    }

    /// `--working` beats `--placeholder`, which beats `--status` — the exact
    /// precedence `WorkspaceBranchIcon::glyph` implements.
    #[test]
    fn working_beats_placeholder_beats_status() {
        let cell = a_cell(&["--status", "locked", "--placeholder", "--working"]);
        assert_eq!(params(&cell).icon(&cell).glyph(), None);

        let cell = a_cell(&["--status", "locked", "--placeholder"]);
        assert_eq!(params(&cell).icon(&cell).glyph(), Some(Glyph::Warning));

        let cell = a_cell(&["--status", "locked"]);
        assert_eq!(params(&cell).icon(&cell).glyph(), Some(Glyph::Lock));
    }

    /// `empty` is read straight through to the component's own synthetic
    /// field.
    #[test]
    fn empty_reaches_the_components_own_field() {
        let plain = a_cell(&[]);
        assert!(!params(&plain).icon(&plain).empty);

        let flagged = a_cell(&["--flags", "empty"]);
        assert!(params(&flagged).icon(&flagged).empty);
        assert_eq!(params(&flagged).icon(&flagged).extent(), gpui::px(0.0));
    }

    /// Every status parses, and the vocabulary is closed.
    #[test]
    fn the_status_vocabulary_is_closed() {
        for status in workspace_branch_icon::ALL_STATUSES {
            let cell = a_cell(&["--status", status.name()]);
            assert_eq!(params(&cell).status, status);
        }
        let Err(crate::row_surface::ParseError::Rejected(_)) = Cell::parse(
            [
                "--surface",
                "workspace-branch-icon",
                "--status",
                "open",
            ]
            .iter()
            .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("`open` is not a WorkspaceStatus — it is `pr-open`");
        };
    }

    /// Five of the six flags are unmodelled; `empty` is the one kept real,
    /// and it is synthetic — see the module docs.
    #[test]
    fn only_empty_is_modelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{}", flag.name());
        }
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }
}
