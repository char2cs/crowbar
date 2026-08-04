//! `--surface search` — `SearchPopover`, rooted the way `find-bar.tsx` and
//! `terminal-search.tsx` both mount it.
//!
//! `crowbar_ui::surfaces::search` carries the full division of labour (the
//! size-6/`sm:h-8` trap, the `button.rs`-overturning content-sized finding
//! on the replace row's two buttons, and why the two boxes are built by hand
//! rather than through `Input::render`); this file is the cell.
//!
//! # Two references, one surface
//!
//! `--replace` mounts [`crowbar_ui::surfaces::search::SearchReplaceRow`]
//! beneath the options/nav row and adds the `preserve-case` toggle — the
//! shape `/tmp/p3-ref-search-replace-row.json` was captured from, rooted at
//! `search-replace-row` rather than `search-popover` for the reason the
//! module docs give (a second, un-overridable `Input` would otherwise
//! collide on `data-oracle-id="input-control"`/`"input"`). This surface's own
//! `--surface search` root is always `search-popover`; a cell driven with
//! `--replace` still renders the replace row, it is simply not what this
//! surface's own snapshot is rooted at — reproducing
//! `/tmp/p3-ref-search-replace-row.json` needs a **second, scoped** capture
//! the way `native/mapping/search.md` §6 records taking it, not a flag on
//! this one.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real**: drives the "Match case" toggle's `active` — the one option this surface can put a live pointer on. Not driven live (`native/mapping/search.md` §7); `search-toggle-icons.md` §6 already measured the one field it moves on the glyph this surface's toggle wraps. |
//! | `empty` | **unmodelled.** `search.tsx` has no "row with nothing on its trailing edge" concept — the git-status-row vocabulary this flag comes from does not apply here. |
//! | `hover`, `focus` | **unmodelled**, `button`'s own reason: no reference (`CGPreflightPostEventAccess()` false; a focused control's state is a `ring`, `ANCHORS.md` §6). |
//! | `loading`, `error` | unmodelled, as on every surface so far. |

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::search::{self, SearchPopover, SearchReplaceRow, ToggleState};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "search",
    root: search::ID_POPOVER,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
    ],
    // The captured cell's own height (root ~84px, ~159px with `--replace`)
    // plus the caption. A floor, not a ceiling — this surface drives no
    // window height of its own (`Params::driven_height` returns `None`).
    min_window_height: 200,
    // A floating panel with its own fixed 320px width, positioned by the
    // caller — never the viewport.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
///
/// `Default` is derived: both fields' `false` is the captured cell's own
/// state (`replace` collapsed, `can_navigate` false — an empty query has no
/// matches), so the derive and the intended default agree.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Params {
    /// `--replace`: mounts the replace row and the fourth (`preserve-case`)
    /// option toggle, matching `find-bar.tsx`'s single `isReplaceVisible`.
    pub replace: bool,
    /// `--can-navigate`: enables the two nav chevrons. `false` is the
    /// captured cell's own state (an empty query has no matches).
    pub can_navigate: bool,
}

impl Params {
    /// The popover this cell describes.
    #[must_use]
    pub fn popover(&self, cell: &Cell) -> SearchPopover {
        SearchPopover {
            breakpoint: cell.breakpoint(),
            toggles: ToggleState {
                case: cell.has(StateFlag::Selected),
                whole_word: false,
                regex: false,
            },
            preserve_case_active: false,
            can_navigate: self.can_navigate,
            replace: self.replace.then(|| SearchReplaceRow {
                can_replace: self.can_navigate,
                can_replace_all: self.can_navigate,
                breakpoint: cell.breakpoint(),
            }),
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--replace" => self.replace = true,
            "--can-navigate" => self.can_navigate = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Every box on this surface authors its own height —
    /// [`search::icon_button_extent`] or `search.tsx`'s own `h-8` — so no
    /// option here decides a window extent the way `--body-height` does on
    /// `dropdown`/`popover`.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if self.replace {
            out.push_str(
                " · replace: the row plus the preserve-case toggle — captured as its own \
                 sibling-rooted snapshot, see the module docs",
            );
        }
        if self.can_navigate {
            out.push_str(" · can-navigate: both chevrons at rest opacity");
        } else {
            out.push_str(
                " · nav disabled: opacity 0.64 — button.tsx's own disabled: pseudo-class, \
                 not this surface's opacity-50",
            );
        }
        if cell.has(StateFlag::Selected) {
            out.push_str(" · selected: the \"Match case\" toggle is active (not captured live)");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.popover(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--replace".to_owned(),
            "mount the replace row and the preserve-case toggle [false]".to_owned(),
        ),
        (
            "--can-navigate".to_owned(),
            "enable the two nav chevrons — the captured cell has none [false]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams as _;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "search"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a search cell carries this surface's bag")
    }

    /// The default cell is the captured, collapsed reference:
    /// `/tmp/p3-ref-search.json`.
    #[test]
    fn the_default_cell_is_the_collapsed_captured_reference() {
        let bag = Params::default();
        assert!(!bag.replace);
        assert!(!bag.can_navigate);

        let popover = bag.popover(&cell(&[]));
        assert!(popover.replace.is_none());
        assert!(!popover.can_navigate);
        assert_eq!(
            popover.toggles,
            crowbar_ui::surfaces::search::ToggleState::default()
        );
        assert_eq!(
            popover.breakpoint,
            crowbar_ui::surfaces::rows::git_status_row::Breakpoint::Sm
        );
    }

    /// `--replace` mounts the row and the fourth toggle together — one flag,
    /// the same shape `find-bar.tsx`'s single `isReplaceVisible` gates both
    /// from.
    #[test]
    fn replace_mounts_the_row_and_the_fourth_toggle_together() {
        let driven = cell(&["--replace"]);
        let popover = params_of(&driven).popover(&driven);
        assert!(popover.replace.is_some());
    }

    /// `--can-navigate` reaches both the popover's own flag and the mounted
    /// replace row's two `can_replace*` fields.
    #[test]
    fn can_navigate_reaches_the_popover_and_the_replace_row() {
        let driven = cell(&["--replace", "--can-navigate"]);
        let popover = params_of(&driven).popover(&driven);
        assert!(popover.can_navigate);
        let replace = popover.replace.expect("--replace was passed");
        assert!(replace.can_replace);
        assert!(replace.can_replace_all);
    }

    /// `selected` drives the "Match case" toggle and only that one — the
    /// other two stay resting, matching the one live pointer state this
    /// surface's caption can honestly describe.
    #[test]
    fn selected_drives_only_the_case_toggle() {
        let driven = cell(&["--flags", "selected"]);
        let popover = params_of(&driven).popover(&driven);
        assert!(popover.toggles.case);
        assert!(!popover.toggles.whole_word);
        assert!(!popover.toggles.regex);
    }

    /// This surface drives no window height of its own.
    #[test]
    fn no_option_drives_a_window_height() {
        assert_eq!(params_of(&cell(&[])).driven_height(&cell(&[])), None);
        assert_eq!(
            params_of(&cell(&["--replace"])).driven_height(&cell(&["--replace"])),
            None
        );
    }

    /// The five interaction/empty flags are declared unmodelled.
    #[test]
    fn the_five_unmodelled_flags_are_declared_so() {
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
    }

    /// The vocabulary is closed.
    #[test]
    fn the_option_vocabulary_is_closed() {
        let line = ["--surface", "search", "--variant", "ghost"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    /// These options belong to this surface and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--replace", "--can-navigate"] {
            let line = ["--surface", "git-status-row", option];
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
        assert!(usage.contains("search"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "search");
        assert_eq!(SURFACE.root, "search-popover");
    }

    /// The caption names the replace/nav state honestly.
    #[test]
    fn the_caption_names_replace_and_nav_state() {
        assert!(cell(&["--replace"]).describe().contains("replace:"));
        assert!(cell(&[]).describe().contains("nav disabled"));
        assert!(
            cell(&["--can-navigate"])
                .describe()
                .contains("can-navigate:")
        );
        let _ = px(0.0);
    }
}
