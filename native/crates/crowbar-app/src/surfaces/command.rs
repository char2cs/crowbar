//! `--surface command` — P3.32's composed, reachable surface: the workspace
//! switcher, an open `CommandDialogPopup` with `Command`/`CommandInput`/
//! `CommandPanel`/`CommandList`/`CommandItem`/`CommandFooter` inside it.
//!
//! `crowbar_ui::primitives::command` carries the division of labour (a
//! second hand-rolled dialog shell, the same shape `AppDialog` is, wrapping
//! `crowbar_ui::primitives::autocomplete`'s own boxes for everything else);
//! this file is the cell. No `--surface autocomplete` exists alongside it —
//! see `autocomplete.rs`'s own module docs for why: `Autocomplete`
//! (`AutocompletePrimitive.Root`) renders no box of its own, so there is no
//! primitive-level composition to capture that is not already this one.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real.** [`StateFlag::Empty`] swaps [`autocomplete::ListContent::Item`] for [`autocomplete::ListContent::Empty`] — the primitive's other picture, modelled but with no live reference (every workspace fixture this dev environment holds has at least one row). |
//! | `selected`, `loading`, `error`, `hover`, `focus` | unmodelled. `data-highlighted` is real on [`Item`] (a plain field), but not through this axis: `autoHighlight="always"` plus one row means the live reference is *unconditionally* highlighted — there is no cell this surface can reach where a call site's own interaction toggles it, the same "no original, on either surface" reasoning `Loading`/`Error` carry elsewhere. [`Params::command`] always renders the highlighted picture; a non-highlighted [`Item`] is reachable in Rust (set the field directly, as `autocomplete.rs`'s own tests do) but not through `--flags`. |
//!
//! Full-bleed for the same reason `dialog`'s is: `CommandDialogViewport` is
//! `fixed inset-0`, so the popup's own `w-full max-w-xl` is a function of the
//! window, and `--width`/`--viewport-width` need to be driveable equal.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::autocomplete::{Item, List, ListContent};
use crowbar_ui::primitives::command::{self, Command};
use gpui::{AnyElement, App, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "command",
    root: command::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live popup is 142px tall; the caption sits below whatever the cell
    // drives it to.
    min_window_height: 220,
    // `CommandDialogViewport` is `fixed inset-0` — see the module docs.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--max-width`'s default: the live workspace switcher's own `max-w-xl`.
pub const DEFAULT_MAX_WIDTH: u16 = 576;

/// `--footer-height`'s default: the footer's two `<KbdGroup>`/`<Kbd>` rows,
/// measured off the running app at 20px.
pub const DEFAULT_FOOTER_HEIGHT: u16 = 20;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--max-width`: `max-w-xl`.
    pub max_width: u16,
    /// `--footer-height`: the footer's own content row.
    pub footer_content_height: Option<u16>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            max_width: DEFAULT_MAX_WIDTH,
            footer_content_height: Some(DEFAULT_FOOTER_HEIGHT),
        }
    }
}

impl Params {
    /// The popup this cell describes.
    ///
    /// `empty` swaps the one row for the (unreached) empty state. The row,
    /// when present, always carries [`Item::fixture`]'s own `highlighted:
    /// true` — the live, unconditional picture; see the module docs for why
    /// `selected` is not the flag that drives it.
    #[must_use]
    pub fn command(&self, cell: &Cell) -> Command {
        let content = if cell.has(StateFlag::Empty) {
            ListContent::Empty
        } else {
            ListContent::Item(Item::fixture())
        };

        Command {
            max_width: px(f32::from(self.max_width)),
            footer_content_height: self.footer_content_height.map(|h| px(f32::from(h))),
            list: List {
                content,
                ..Command::fixture().list
            },
            ..Command::fixture()
        }
    }

    /// The popup's own height, spelled here rather than measured because
    /// [`SurfaceParams::driven_height`] is asked before a window exists.
    #[must_use]
    pub fn popup_height(&self, cell: &Cell) -> u16 {
        let height = self.command(cell).popup_height();
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
        let _ = write!(out, " · max-w {}px", self.max_width);
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no live workspace-switcher capture has zero workspaces");
        }
    }

    // `render` is never called on this surface: `render_row` always calls
    // `render_ctx` — see `dialog.rs`'s own surface for why (`Command::render`
    // needs `window`/`cx` for the same reason `Dialog::render` does).
    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "command needs window/App context; render_row calls render_ctx, never render \
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
        self.command(cell).render(window, cx, theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--max-width <px>",
            format!("max-w-xl: the popup's own cap [{DEFAULT_MAX_WIDTH}]"),
        ),
        (
            "--footer-height <px>",
            format!("how tall the footer's own row is [{DEFAULT_FOOTER_HEIGHT}]"),
        ),
        ("--no-footer", "omit the footer entirely".into()),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_FOOTER_HEIGHT, DEFAULT_MAX_WIDTH, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::primitives::autocomplete::ListContent;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "command"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a command cell carries this surface's bag")
    }

    /// The defaults are the live workspace switcher, measured off the
    /// running app.
    #[test]
    fn the_defaults_are_the_live_workspace_switcher() {
        let bag = Params::default();

        assert_eq!(bag.max_width, DEFAULT_MAX_WIDTH);
        assert_eq!(bag.footer_content_height, Some(DEFAULT_FOOTER_HEIGHT));

        let cmd = bag.command(&cell(&[]));
        assert_eq!(cmd.max_width, px(576.0));
        assert_eq!(bag.popup_height(&cell(&[])), 142);
    }

    /// The row is highlighted **unconditionally** — `autoHighlight="always"`
    /// plus one row means there is no reachable cell where it is not, so
    /// `selected` is declared unmodelled rather than gating it (see the
    /// module docs).
    #[test]
    fn the_row_is_highlighted_unconditionally() {
        let resting_cell = cell(&[]);
        let resting = params_of(&resting_cell).command(&resting_cell);
        match resting.list.content {
            ListContent::Item(item) => assert!(item.highlighted),
            ListContent::Empty => panic!("the default fixture has one row"),
        }
        assert!(SURFACE.unmodelled(StateFlag::Selected));
    }

    /// `empty` swaps the row for the (unreached) empty state.
    #[test]
    fn empty_swaps_the_row_for_the_empty_state() {
        let empty_cell = cell(&["--flags", "empty"]);
        let cmd = params_of(&empty_cell).command(&empty_cell);
        assert!(matches!(cmd.list.content, ListContent::Empty));
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The five interaction flags are declared unmodelled.
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

    /// `--footer-height`/`--no-footer` move the popup and the window
    /// follows.
    #[test]
    fn the_footer_height_drives_the_popup_and_the_window() {
        let tall = cell(&["--footer-height", "100"]);
        assert!(params_of(&tall).popup_height(&tall) > 142);
        assert!(tall.window_extent() > 142);

        let no_footer = cell(&["--no-footer"]);
        // Losing the footer loses its own border and both paddings besides
        // its 20px content: 1 + 12 + 12 + 20 = 45, off the 142px resting
        // height.
        assert_eq!(params_of(&no_footer).popup_height(&no_footer), 142 - 45);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--max-width", "wide"],
            vec!["--max-width"],
            vec!["--footer-height", "-1"],
            vec!["--variant", "app"],
        ] {
            let mut full = vec!["--surface", "command"];
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
        for option in ["--max-width", "--footer-height", "--no-footer"] {
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

        for option in ["--max-width", "--footer-height", "--no-footer"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("command"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "command");
        assert_eq!(SURFACE.root, "command-dialog-popup");
        assert!(SURFACE.full_bleed);
    }
}
