//! `--surface alert-dialog` — the second surface drawn by a **wrapped**
//! `gpui-component` widget with `window`/`App` access, and the first two
//! surfaces (`dialog`, this one) whose params are otherwise nearly identical
//! by construction — see `crowbar_ui::components::alert_dialog`'s module docs
//! for the finding that makes this so rather than an accident.
//!
//! An **open** alert-dialog popup: `AlertDialogPopup` (`AlertDialogContent`)
//! plus whichever of `AlertDialogHeader`/`AlertDialogTitle`/
//! `AlertDialogDescription`/`AlertDialogFooter` a call site nests.
//! `crowbar_ui::components::alert_dialog` carries the division of labour; this
//! file is the cell.
//!
//! # Full-bleed, for the identical reason `dialog`'s is
//!
//! `alert-dialog.tsx`'s `AlertDialogViewport` is `fixed inset-0`, exactly
//! `dialog.tsx`'s `DialogViewport` — the whole window, centring the popup
//! inside it. [`Surface::full_bleed`] is what lets `--width` and
//! `--viewport-width` be driven equal, the configuration
//! `AlertDialog::render` assumes when it reads `window.viewport_size()`.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled.** `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' alert-dialog.tsx` is empty on all five named boxes. |
//! | `loading`, `error` | unmodelled, as on every surface. |
//! | `empty` | **real**, on the identical arithmetic `dialog`'s is: removes the header (and the title/description under it) and the footer together. No live call site of `alert-dialog.tsx` actually takes this shape — the one reachable call site always renders both — but the primitive itself permits it precisely as `DialogPopup`'s does, and declaring it here is the same "port it and say so" call `dialog`'s `empty` arm and `sheet`'s whole module make. |
//!
//! Five of the six are declared on [`SURFACE`].

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::alert_dialog::{self, AlertDialog};
use gpui::{AnyElement, App, SharedString, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "alert-dialog",
    root: alert_dialog::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The one reachable popup is 159px tall (module docs' §"fixture" account);
    // a floor, not a ceiling.
    min_window_height: 220,
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--max-width`'s default: `alert-dialog.tsx`'s own unmodified `max-w-lg` —
/// the one live call site passes no `className`.
pub const DEFAULT_MAX_WIDTH: u16 = 512;

/// `--body-height`'s default: the one reachable call site nests no content
/// between the header and the footer at all — see
/// `crowbar_ui::components::alert_dialog`'s module docs §3.
pub const DEFAULT_BODY_HEIGHT: u16 = 0;

/// `--footer-height`'s default: `button::Size::Sm`'s own already-ported height
/// at the `sm` breakpoint (`h-8 sm:h-7` → 28px) — the one live call site's
/// Cancel/Delete pair, both `size="sm"`.
pub const DEFAULT_FOOTER_HEIGHT: u16 = 28;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--max-width`: `max-w-*`, whichever a call site's `className` resolves
    /// tailwind-merge to.
    pub max_width: u16,
    /// `--body-height`: how tall the call site's own content comes out,
    /// between the header and the footer.
    pub body_height: u16,
    /// `--title`: renders an `AlertDialogTitle`.
    pub title: Option<SharedString>,
    /// `--description`: renders an `AlertDialogDescription`.
    ///
    /// Defaults to `Some`, unlike `dialog`'s own `--description` default of
    /// `None`: the one reachable `alert-dialog` call site always nests one.
    pub description: Option<SharedString>,
    /// `--footer-height`: how tall the footer's own buttons come out. `None`
    /// omits the footer entirely.
    pub footer_content_height: Option<u16>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            max_width: DEFAULT_MAX_WIDTH,
            body_height: DEFAULT_BODY_HEIGHT,
            title: Some(SharedString::new_static("Delete this thread?")),
            description: Some(SharedString::new_static(
                "This permanently deletes the comment and its thread.",
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
    pub fn alert_dialog(&self, cell: &Cell) -> AlertDialog {
        let empty = cell.has(StateFlag::Empty);
        AlertDialog {
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

    /// The popup's own height, spelled here rather than measured because
    /// [`SurfaceParams::driven_height`] is asked *before* a window exists.
    #[must_use]
    pub fn popup_height(&self, cell: &Cell) -> u16 {
        let dialog = self.alert_dialog(cell);
        let height = dialog.popup_height(&cell.theme());
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
        Some(self.popup_height(cell))
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
        out.push_str(" · no live pixel reference — see native/mapping/alert-dialog.md §3");
    }

    // `render` is never called on this surface — see `dialog`'s identical
    // arm and `crowbar_ui::components::alert_dialog::AlertDialog::render`'s
    // docs for why `GpuiDialog::new` needs `window`/`cx`.
    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "alert-dialog needs window/App context; render_row calls render_ctx, never render \
             directly — see SurfaceParams::render_ctx's docs"
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
        self.alert_dialog(cell).render(window, cx, theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--max-width <px>",
            format!("max-w-*: the call site's own cap [{DEFAULT_MAX_WIDTH}]"),
        ),
        (
            "--body-height <px>",
            format!("how tall the call site's own content is [{DEFAULT_BODY_HEIGHT}]"),
        ),
        ("--title <text>", "render an AlertDialogTitle".into()),
        (
            "--description <text>",
            "render an AlertDialogDescription".into(),
        ),
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
        let mut line = vec!["--surface", "alert-dialog"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("an alert-dialog cell carries this surface's bag")
    }

    /// The defaults are the one reachable review-thread delete confirmation.
    #[test]
    fn the_defaults_are_the_live_delete_confirmation() {
        let bag = Params::default();

        assert_eq!(bag.max_width, DEFAULT_MAX_WIDTH);
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.title.as_deref(), Some("Delete this thread?"));
        assert_eq!(
            bag.description.as_deref(),
            Some("This permanently deletes the comment and its thread."),
        );
        assert_eq!(bag.footer_content_height, Some(DEFAULT_FOOTER_HEIGHT));

        let dialog = bag.alert_dialog(&cell(&[]));
        assert_eq!(dialog.max_width, px(512.0));
        assert_eq!(dialog.body_height, px(0.0));
        assert_eq!(bag.popup_height(&cell(&[])), 159);
    }

    /// `empty` clears the header and the footer together, the same shape
    /// `dialog`'s own `empty` arm takes.
    #[test]
    fn empty_removes_the_header_and_the_footer() {
        let resting = params_of(&cell(&[])).popup_height(&cell(&[]));
        let empty_cell = cell(&["--flags", "empty"]);
        let empty = params_of(&empty_cell).popup_height(&empty_cell);

        // Two borders and a zero-height body only.
        assert_eq!(empty, 2);
        assert_eq!(resting, 159);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));

        let dialog = params_of(&empty_cell).alert_dialog(&empty_cell);
        assert_eq!(dialog.title, None);
        assert_eq!(dialog.footer_content_height, None);
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

    /// `--body-height`/`--footer-height` move the popup and the window
    /// follows.
    #[test]
    fn the_body_and_footer_heights_drive_the_popup_and_the_window() {
        let tall = cell(&["--body-height", "400"]);
        assert_eq!(params_of(&tall).popup_height(&tall), 559);
        assert!(tall.window_extent() > 559);

        let no_footer = cell(&["--no-footer"]);
        // Losing the footer loses its own border and both paddings besides
        // its content: 1 + 16 + 16 + 28 = 61, off the 159px resting height.
        assert_eq!(params_of(&no_footer).popup_height(&no_footer), 159 - 61);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--max-width", "wide"],
            vec!["--max-width"],
            vec!["--body-height", "-1"],
            vec!["--body-height"],
            vec!["--title"],
            vec!["--variant", "app"],
        ] {
            let mut full = vec!["--surface", "alert-dialog"];
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
        assert!(usage.contains("alert-dialog"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "alert-dialog");
        assert_eq!(SURFACE.root, "alert-dialog-popup");
        assert!(SURFACE.full_bleed);
    }
}
