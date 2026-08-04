//! `--surface detach-holder-modal` — a **call site** of the wrap `--surface
//! dialog` already measures, with two `className` overrides of its own.
//!
//! `crowbar_ui::surfaces::detach_holder_modal` carries the full account of
//! why this is its own module rather than a second construction of
//! `dialog::Dialog`; this file is the cell, in exactly the shape `dialog.rs`'s
//! own surface takes.
//!
//! # Full-bleed, for the identical reason `dialog`'s is
//!
//! `DialogViewport` is `fixed inset-0` on this call site too — see
//! `dialog.rs`'s own module docs.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled.** `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' detach-holder-modal.tsx` is empty. |
//! | `loading`, `error` | unmodelled, as on every surface. |
//! | `empty` | **real**, the identical arithmetic `dialog`'s own `empty` arm takes: removes the header and the footer together. No live call site of this file actually takes this shape (the one reachable call site always renders both) — the same "port it and say so" call `dialog`'s and `alert-dialog`'s own `empty` arms make. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::detach_holder_modal::{self, DetachHolderModal};
use gpui::{AnyElement, App, SharedString, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "detach-holder-modal",
    root: detach_holder_modal::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live popup's own body is 0px, but its description wraps to several
    // lines at this call site's own narrower (`pr-10`) content width — a
    // floor generous enough that the fixed floor, not the estimate, decides
    // the window's height on every cell this surface's tests drive. See
    // `crowbar_ui::surfaces::detach_holder_modal::DetachHolderModal::
    // popup_height_estimate`'s own doc comment for why the estimate
    // undercounts a wrapped description on purpose.
    min_window_height: 500,
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--max-width`'s default: `max-w-md`, the live call site's own cap.
pub const DEFAULT_MAX_WIDTH: u16 = 448;

/// `--body-height`'s default: the live call site nests no content between
/// its header and its footer at all.
pub const DEFAULT_BODY_HEIGHT: u16 = 0;

/// `--footer-height`'s default: both of this call site's buttons are
/// default-sized, the same number `dialog`'s own default footer content
/// height is.
pub const DEFAULT_FOOTER_HEIGHT: u16 = 32;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--max-width`.
    pub max_width: u16,
    /// `--body-height`.
    pub body_height: u16,
    /// `--title`.
    pub title: Option<SharedString>,
    /// `--description`.
    pub description: Option<SharedString>,
    /// `--footer-height`. `None` omits the footer entirely.
    pub footer_content_height: Option<u16>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            max_width: DEFAULT_MAX_WIDTH,
            body_height: DEFAULT_BODY_HEIGHT,
            title: Some(SharedString::new_static("Detach to manage main")),
            description: Some(SharedString::new_static(
                "The checkout at /Users/dev/crowbar-worktrees/main will move to a detached \
                 HEAD, releasing main so Crowbar can manage it in its own worktree. Your \
                 files are safe — only the working directory's current branch changes; \
                 uncommitted changes and commits are preserved.",
            )),
            footer_content_height: Some(DEFAULT_FOOTER_HEIGHT),
        }
    }
}

impl Params {
    /// The popup this cell describes.
    ///
    /// `empty` clears the header and the footer together — see the module
    /// docs' state-axis table.
    #[must_use]
    pub fn dialog(&self, cell: &Cell) -> DetachHolderModal {
        let empty = cell.has(StateFlag::Empty);
        DetachHolderModal {
            max_width: px(f32::from(self.max_width)),
            body_height: px(f32::from(self.body_height)),
            title: if empty { None } else { self.title.clone() },
            description: if empty {
                None
            } else {
                self.description.clone()
            },
            footer_content_height: if empty {
                None
            } else {
                self.footer_content_height.map(|h| px(f32::from(h)))
            },
        }
    }

    /// An estimate of the popup's own height, for window sizing only — see
    /// `DetachHolderModal::popup_height_estimate`'s own doc comment for why
    /// this is deliberately conservative rather than exact on a wrapped
    /// description.
    #[must_use]
    pub fn popup_height_estimate(&self, cell: &Cell) -> u16 {
        let dialog = self.dialog(cell);
        let height = dialog.popup_height_estimate(&cell.theme());
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole number of px; \
                      `Cell` needs a `u16` to stay `Eq`"
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
            "--max-width" => self.max_width = pixels(&value(args, option)?, option)?,
            "--body-height" => self.body_height = pixels(&value(args, option)?, option)?,
            "--title" => self.title = Some(SharedString::from(value(args, option)?)),
            "--description" => self.description = Some(SharedString::from(value(args, option)?)),
            "--footer-height" => {
                self.footer_content_height = Some(pixels(&value(args, option)?, option)?);
            }
            "--no-footer" => self.footer_content_height = None,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.popup_height_estimate(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · max-w {}px · body {}px",
            self.max_width, self.body_height,
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no header, no footer");
        }
        out.push_str(
            " · a call site of dialog's own primitive — see \
             native/mapping/detach-holder-modal.md §0",
        );
    }

    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "detach-holder-modal needs window/App context; render_row calls render_ctx, never \
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
            "--body-height <px>",
            format!("how tall the call site's own content is [{DEFAULT_BODY_HEIGHT}]"),
        ),
        ("--title <text>", "render a DialogTitle".into()),
        ("--description <text>", "render a DialogDescription".into()),
        (
            "--footer-height <px>",
            format!("how tall the footer's own buttons are [{DEFAULT_FOOTER_HEIGHT}]"),
        ),
        ("--no-footer", "omit the footer entirely".into()),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{
        DEFAULT_BODY_HEIGHT, DEFAULT_FOOTER_HEIGHT, DEFAULT_MAX_WIDTH, Params, SURFACE, options,
    };
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "detach-holder-modal"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a detach-holder-modal cell carries this surface's bag")
    }

    /// The defaults are the live `detach-holder-modal.tsx`.
    #[test]
    fn the_defaults_are_the_live_detach_holder_modal() {
        let bag = Params::default();

        assert_eq!(bag.max_width, DEFAULT_MAX_WIDTH);
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.title.as_deref(), Some("Detach to manage main"));
        assert_eq!(bag.footer_content_height, Some(DEFAULT_FOOTER_HEIGHT));

        let dialog = bag.dialog(&cell(&[]));
        assert_eq!(dialog.max_width, px(448.0));
        assert_eq!(dialog.body_height, px(0.0));
    }

    /// `empty` removes the header and the footer together.
    #[test]
    fn empty_removes_the_header_and_the_footer() {
        let empty_cell = cell(&["--flags", "empty"]);
        let dialog = params_of(&empty_cell).dialog(&empty_cell);

        assert_eq!(dialog.title, None);
        assert_eq!(dialog.description, None);
        assert_eq!(dialog.footer_content_height, None);
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
            vec!["--body-height", "-1"],
            vec!["--title"],
            vec!["--variant", "app"],
        ] {
            let mut full = vec!["--surface", "detach-holder-modal"];
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
        for option in [
            "--max-width",
            "--body-height",
            "--footer-height",
            "--no-footer",
        ] {
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

        for option in [
            "--max-width",
            "--body-height",
            "--title",
            "--description",
            "--footer-height",
            "--no-footer",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("detach-holder-modal"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "detach-holder-modal");
        assert_eq!(SURFACE.root, "detach-holder-modal-popup");
        assert!(SURFACE.full_bleed);
    }
}
