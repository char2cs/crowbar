//! `--surface workspace-branch-icon` — the sidebar row's status glyph, one of
//! two P3.50 foundation leaves. See
//! `crowbar_ui::components::workspace_branch_icon`'s module docs for the
//! shape (single swapped element, no persistent wrapper), the "seven statuses,
//! five pictures" finding, and the exhaustive check backing this surface's
//! [`SurfaceParams::no_state_axis`] declaration below: this component takes
//! no `className` and spreads no props anywhere.
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
//! **None of the six §8.3 flags is modelled**, checked exhaustively rather
//! than assumed — see the component's own module docs. Earlier this surface
//! kept a synthetic `empty` flag alive purely to satisfy the registry
//! invariant that used to require at least one non-mandatory flag to be
//! real; that field is gone (P3.50 follow-up). In its place, `Params` below
//! declares [`SurfaceParams::no_state_axis`] — "every flag considered, none
//! applicable" — which the invariant now checks is present exactly when it
//! is true, and absent exactly when it is not.

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
        // All four non-mandatory flags, including `Empty` — this component
        // has no seam of any kind, checked exhaustively (module docs), so
        // there is no edge value left to reach through. `Params::no_state_axis`
        // below is the declaration that makes this deliberate rather than an
        // oversight.
        StateFlag::Empty,
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
    ///
    /// Takes no `cell` state beyond `self`'s own fields — unlike every other
    /// surface with a `describe`-driven cell, there is no flag left on this
    /// one to read. See [`SurfaceParams::no_state_axis`].
    #[must_use]
    pub fn icon(&self) -> WorkspaceBranchIcon {
        WorkspaceBranchIcon {
            status: self.status,
            working: self.working,
            is_placeholder: self.placeholder,
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

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let icon = self.icon();
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
    }

    /// **`true`, unconditionally.** See the module docs and this surface's
    /// own `unmodelled` list: every §8.3 flag is checked and none applies,
    /// there is no `--flags empty` (or any other) branch left for a cell to
    /// drive on this surface.
    fn no_state_axis(&self) -> bool {
        true
    }

    /// The icon, inside the flex row that makes it a flex item — every live
    /// call site renders it as a sibling of a workspace name.
    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_start()
            .child(self.icon().render(theme, anchors))
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
    use crate::surface::SurfaceParams;
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
        let icon = params(&cell).icon();
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
        assert_eq!(params(&cell).icon().glyph(), None);

        let cell = a_cell(&["--status", "locked", "--placeholder"]);
        assert_eq!(params(&cell).icon().glyph(), Some(Glyph::Warning));

        let cell = a_cell(&["--status", "locked"]);
        assert_eq!(params(&cell).icon().glyph(), Some(Glyph::Lock));
    }

    /// `--flags empty` parses (the vocabulary is shared across every
    /// surface) but reaches nothing here — there is no field left on
    /// [`crowbar_ui::components::workspace_branch_icon::WorkspaceBranchIcon`]
    /// for it to drive, and `Params::icon` never reads `cell` at all. The
    /// cell still cannot fail, which is exactly what an unmodelled flag
    /// means.
    #[test]
    fn empty_parses_but_reaches_nothing() {
        let plain = a_cell(&[]);
        let flagged = a_cell(&["--flags", "empty"]);
        assert_eq!(params(&plain).icon(), params(&flagged).icon());
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

    /// All six flags are unmodelled, and the surface declares that on
    /// purpose — the two facts `no_surface_declares_its_entire_state_axis_
    /// unmodelled` (`surface.rs`) requires to agree with each other.
    #[test]
    fn every_flag_is_unmodelled_and_the_surface_declares_it() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Empty,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{}", flag.name());
        }
        assert!(Params::default().no_state_axis());
    }
}
