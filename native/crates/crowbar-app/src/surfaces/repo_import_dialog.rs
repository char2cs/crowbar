//! `--surface repo-import-dialog` — a **call site** of the wrap `--surface
//! dialog` already measures, with a viewport-relative popup height and no
//! footer at all.
//!
//! `crowbar_ui::components::repo_import_dialog` carries the full account,
//! including why this call site's own `--popup-height` — not `--body-height`
//! — is the surface's independent variable. This file is the cell.
//!
//! # Full-bleed, for the identical reason `dialog`'s is
//!
//! `DialogViewport` is `fixed inset-0` on this call site too.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled.** `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' repo-import-dialog.tsx` is empty. |
//! | `loading`, `error` | unmodelled, as on every surface. |
//! | `empty` | **real**: removes the header. No live call site of this file actually takes this shape (the one reachable call site always renders both a title and a description) — the same "port it and say so" call `dialog`'s and `alert-dialog`'s own `empty` arms make. There is no footer to remove alongside it — this call site never renders one. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::repo_import_dialog::{self, RepoImportDialog, popup_height_at};
use gpui::{AnyElement, App, SharedString, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "repo-import-dialog",
    root: repo_import_dialog::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `--popup-height`'s own default is 630 (see DEFAULT_WINDOW_HEIGHT
    // below); the floor is comfortably above it so the fixed floor, not the
    // popup, decides the window on the default cell.
    min_window_height: 700,
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--max-width`'s default: `max-w-md`, the live call site's own cap.
pub const DEFAULT_MAX_WIDTH: u16 = 448;

/// `--window-height`'s default — see
/// `crowbar_ui::components::repo_import_dialog::RepoImportDialog::fixture`'s
/// doc comment for why this is a stated assumption, not a measurement.
pub const DEFAULT_WINDOW_HEIGHT: u16 = 900;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--max-width`.
    pub max_width: u16,
    /// `--window-height`: the assumed enclosing window height `h-[70vh]`
    /// resolves against. Not itself a rendered box — [`Params::dialog`]
    /// turns it into the popup's own `--popup-height` via
    /// [`popup_height_at`].
    pub window_height: u16,
    /// `--title`.
    pub title: Option<SharedString>,
    /// `--description`.
    pub description: Option<SharedString>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            max_width: DEFAULT_MAX_WIDTH,
            window_height: DEFAULT_WINDOW_HEIGHT,
            title: Some(SharedString::new_static("Import branches")),
            description: Some(SharedString::new_static(
                "Bring remote branches into Crowbar as workspaces.",
            )),
        }
    }
}

impl Params {
    /// The popup this cell describes.
    ///
    /// `empty` clears the header — see the module docs' state-axis table.
    #[must_use]
    pub fn dialog(&self, cell: &Cell) -> RepoImportDialog {
        let empty = cell.has(StateFlag::Empty);
        RepoImportDialog {
            max_width: px(f32::from(self.max_width)),
            popup_height: popup_height_at(px(f32::from(self.window_height))),
            title: if empty { None } else { self.title.clone() },
            description: if empty {
                None
            } else {
                self.description.clone()
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
            "--max-width" => self.max_width = pixels(&value(args, option)?, option)?,
            "--window-height" => self.window_height = pixels(&value(args, option)?, option)?,
            "--title" => self.title = Some(SharedString::from(value(args, option)?)),
            "--description" => self.description = Some(SharedString::from(value(args, option)?)),
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        let dialog = self.dialog(cell);
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole number of px; \
                      `Cell` needs a `u16` to stay `Eq`"
        )]
        {
            Some(f32::from(dialog.popup_height).ceil() as u16)
        }
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · max-w {}px · window {}px (popup {}px, 70vh)",
            self.max_width,
            self.window_height,
            f32::from(popup_height_at(px(f32::from(self.window_height)))),
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no header");
        }
        out.push_str(
            " · a call site of dialog's own primitive, no live pixel reference — see \
             native/mapping/repo-import-dialog.md",
        );
    }

    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "repo-import-dialog needs window/App context; render_row calls render_ctx, never \
             render directly — see SurfaceParams::render_ctx's docs"
        )
    }

    fn render_ctx(
        &self,
        cell: &Cell,
        theme: &Theme,
        anchors: &dyn AnchorSink,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        self.dialog(cell).render(window, cx, theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--max-width <px>",
            format!("max-w-md: the call site's own cap [{DEFAULT_MAX_WIDTH}]"),
        ),
        (
            "--window-height <px>",
            format!(
                "the assumed enclosing window height h-[70vh] resolves against \
                 [{DEFAULT_WINDOW_HEIGHT}]"
            ),
        ),
        ("--title <text>", "render a DialogTitle".into()),
        ("--description <text>", "render a DialogDescription".into()),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_MAX_WIDTH, DEFAULT_WINDOW_HEIGHT, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "repo-import-dialog"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a repo-import-dialog cell carries this surface's bag")
    }

    /// The defaults are the live `repo-import-dialog.tsx`.
    #[test]
    fn the_defaults_are_the_live_repo_import_dialog() {
        let bag = Params::default();

        assert_eq!(bag.max_width, DEFAULT_MAX_WIDTH);
        assert_eq!(bag.window_height, DEFAULT_WINDOW_HEIGHT);
        assert_eq!(bag.title.as_deref(), Some("Import branches"));
        assert_eq!(
            bag.description.as_deref(),
            Some("Bring remote branches into Crowbar as workspaces."),
        );

        let dialog = bag.dialog(&cell(&[]));
        assert_eq!(dialog.max_width, px(448.0));
        assert_eq!(dialog.popup_height, px(630.0));
    }

    /// `--window-height` drives the popup and the window follows.
    #[test]
    fn window_height_drives_the_popup_and_the_window_follows() {
        let tall = cell(&["--window-height", "1200"]);
        assert_eq!(params_of(&tall).dialog(&tall).popup_height, px(840.0));
        assert!(tall.window_extent() > 840);
    }

    /// `empty` removes the header, and only the header — there is no footer
    /// concept on this surface at all.
    #[test]
    fn empty_removes_the_header() {
        let empty_cell = cell(&["--flags", "empty"]);
        let dialog = params_of(&empty_cell).dialog(&empty_cell);

        assert_eq!(dialog.title, None);
        assert_eq!(dialog.description, None);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other five are declared unmodelled.
    #[test]
    fn the_five_interaction_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
            assert!(
                cell(&["--flags", flag.name()])
                    .unmodelled_flags()
                    .contains(&flag),
                "{flag:?} should be reported on stderr",
            );
        }
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--max-width", "wide"],
            vec!["--max-width"],
            vec!["--window-height", "-1"],
            vec!["--title"],
            vec!["--no-footer"],
        ] {
            let mut full = vec!["--surface", "repo-import-dialog"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// These options belong to this surface and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--max-width", "--window-height"] {
            let line = ["--surface", "git-status-row", option, "24"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in ["--max-width", "--window-height", "--title", "--description"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("repo-import-dialog"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "repo-import-dialog");
        assert_eq!(SURFACE.root, "repo-import-dialog-popup");
        assert!(SURFACE.full_bleed);
    }
}
