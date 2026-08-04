//! `--surface dialog` — the second surface drawn by a **wrapped**
//! `gpui-component` widget, and the first that needs `window`/`App` besides
//! `theme`/`anchors` to build.
//!
//! An **open** dialog popup: `DialogPopup` (`DialogContent`) plus whichever
//! of `DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogFooter` a call
//! site nests. `crowbar_ui::primitives::dialog` carries the division of
//! labour and what had to be overridden; this file is the cell.
//!
//! # Full-bleed, and why that is arithmetic rather than a style choice
//!
//! `dialog.tsx`'s `DialogViewport` is `fixed inset-0` — the whole window,
//! centring the popup inside it — so the popup's own `w-full max-w-*` is a
//! function of the **window**, not of some sidebar-sized column. Declaring
//! [`Surface::full_bleed`] is what makes that true here too: the surface
//! takes no horizontal inset, so `--width` (drawn flush at `x = 0`) and
//! `--viewport-width` (the window) can be driven equal, which is the
//! configuration `crowbar_ui::primitives::dialog::Dialog::render` assumes
//! when it reads `window.viewport_size()`.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled.** `dialog.tsx` carries no `hover:`/`focus:`/selected rule on `DialogPopup`, `DialogHeader`, `DialogTitle`, `DialogDescription` or `DialogFooter` — `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' dialog.tsx` is empty on all five. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//! | `empty` | **real.** A dialog with no header and no footer is a picture this primitive genuinely has — every `AppDialog` call site without a `title`/`icon`/`headerActions` prop takes exactly this shape for its header half, and a bare `DialogPopup` with only a body is legitimate. Removes `dialog-header` (and `dialog-title`/`dialog-description` under it) and `dialog-footer` together, which is the one cell on this surface that can fail. |
//!
//! Five of the six are declared on [`SURFACE`], which is what makes the
//! binary say so on stderr instead of rendering a cell that cannot fail.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::dialog::{self, Dialog};
use gpui::{AnyElement, App, SharedString, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "dialog",
    root: dialog::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live popup is 307px tall; the caption sits below whatever the cell
    // drives it to. A floor, not a ceiling.
    min_window_height: 360,
    // `DialogViewport` is `fixed inset-0`: the popup's own width is a
    // function of the *window*, not of some sidebar-sized column. See the
    // module docs.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--max-width`'s default: the live `add-repository-modal` and
/// `import-project-modal`'s own `sm:max-w-md`.
pub const DEFAULT_MAX_WIDTH: u16 = 448;

/// `--body-height`'s default: the same call sites' form, measured off the
/// running app at 172px.
pub const DEFAULT_BODY_HEIGHT: u16 = 172;

/// `--footer-height`'s default: the same call sites' two-button row.
pub const DEFAULT_FOOTER_HEIGHT: u16 = 32;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--max-width`: `max-w-*`, whichever a call site's `className` resolves
    /// tailwind-merge to.
    pub max_width: u16,
    /// `--body-height`: how tall the call site's own content comes out,
    /// between the header and the footer.
    pub body_height: u16,
    /// `--title`: renders a `DialogTitle`.
    pub title: Option<SharedString>,
    /// `--description`: renders a `DialogDescription`.
    ///
    /// `None` by default: the two live dialogs a parity run can drive render
    /// no description, and an anchor the reference cannot produce is a
    /// `FieldPresence` delta that forgives nothing.
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
            title: Some(SharedString::new_static("Add repository")),
            description: None,
            footer_content_height: Some(DEFAULT_FOOTER_HEIGHT),
        }
    }
}

impl Params {
    /// The popup this cell describes.
    ///
    /// `empty` clears the header and the footer together: a dialog with
    /// neither is a picture this primitive genuinely has (every `AppDialog`
    /// call site with no `title`/`icon`/`headerActions` takes exactly this
    /// shape), and modelling it any other way would leave one of the two
    /// anchors unmoved.
    #[must_use]
    pub fn dialog(&self, cell: &Cell) -> Dialog {
        let empty = cell.has(StateFlag::Empty);
        Dialog {
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
        let dialog = self.dialog(cell);
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
        if self.description.is_some() {
            out.push_str(
                " · description: no live call site has one, so this cell has no reference",
            );
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no header, no footer");
        }
    }

    // `render` is never called on this surface: `render_row` always calls
    // `render_ctx`, and `dialog::Dialog::render` needs `window`/`cx` to mint
    // the `FocusHandle` `GpuiDialog::new` carries — see that method's docs.
    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "dialog needs window/App context; render_row calls render_ctx, never render \
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
        self.dialog(cell).render(window, cx, theme, anchors)
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
        ("--title <text>", "render a DialogTitle".into()),
        (
            "--description <text>",
            "render a DialogDescription; no live dialog a run can reach has one".into(),
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
        let mut line = vec!["--surface", "dialog"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a dialog cell carries this surface's bag")
    }

    /// The defaults are the live `add-repository-modal`, measured off the
    /// running app.
    #[test]
    fn the_defaults_are_the_live_add_repository_modal() {
        let bag = Params::default();

        assert_eq!(bag.max_width, DEFAULT_MAX_WIDTH);
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.title.as_deref(), Some("Add repository"));
        assert_eq!(bag.description, None);
        assert_eq!(bag.footer_content_height, Some(DEFAULT_FOOTER_HEIGHT));

        let dialog = bag.dialog(&cell(&[]));
        assert_eq!(dialog.max_width, px(448.0));
        assert_eq!(dialog.body_height, px(172.0));
        assert_eq!(bag.popup_height(&cell(&[])), 307);
    }

    /// **`empty` is the one flag this surface models**, and it moves the
    /// popup by removing the header and the footer together.
    #[test]
    fn empty_removes_the_header_and_the_footer() {
        let resting = params_of(&cell(&[])).popup_height(&cell(&[]));
        let empty_cell = cell(&["--flags", "empty"]);
        let empty = params_of(&empty_cell).popup_height(&empty_cell);

        // Two borders and the body only: 2 + 172.
        assert_eq!(empty, 174);
        assert_eq!(resting, 307);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));

        let dialog = params_of(&empty_cell).dialog(&empty_cell);
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
        assert_eq!(params_of(&tall).popup_height(&tall), 535);
        assert!(tall.window_extent() > 535);

        let no_footer = cell(&["--no-footer"]);
        // Losing the footer loses its own border and both paddings besides
        // its content: 1 + 16 + 16 + 32 = 65, off the 307px resting height.
        assert_eq!(params_of(&no_footer).popup_height(&no_footer), 307 - 65);
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
            let mut full = vec!["--surface", "dialog"];
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
        assert!(usage.contains("dialog"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "dialog");
        assert_eq!(SURFACE.root, "dialog-popup");
        assert!(SURFACE.full_bleed);
    }
}
