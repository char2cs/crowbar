//! `--surface sheet` — the third surface drawn by a **wrapped**
//! `gpui-component` widget, and the first with **no live reference at all**.
//!
//! `crowbar_ui::primitives::sheet` carries the finding in full: the one
//! React consumer (`sidebar.tsx`'s `Sidebar`) is never mounted anywhere in
//! `web/src`, and the vendor widget this crate wraps cannot be driven past
//! `Placement::Right` without a `Root` this measurement harness does not
//! mount. This file exists so the primitive is drawable and testable —
//! "port it and say so" — not because a parity run can point at anything.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled** — `sheet.tsx` carries no such rule, the same grep `dialog`'s module docs run. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//! | `empty` | **real**, on the same construction `dialog`'s is: removes the header (and the title/description under it), which is a picture `SheetPopup` genuinely has when a call site nests no `SheetHeader`. |

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::sheet::{self, Sheet};
use gpui::{AnyElement, App, SharedString, Window, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sheet",
    root: sheet::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    min_window_height: 400,
    // `SheetViewport` is `fixed inset-0`, exactly `dialog`'s — see that
    // surface's module docs.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--body-height`'s default: an arbitrary, undocumented number — there is
/// no live sheet to measure one off.
pub const DEFAULT_BODY_HEIGHT: u16 = 200;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--body-height`: how tall the call site's own content is. Rendered,
    /// but — see `crowbar_ui::primitives::sheet`'s module docs, point 4 —
    /// does not drive `sheet-popup`'s own height, which the vendor's
    /// placement-based positioning fixes regardless of content.
    pub body_height: u16,
    /// `--title`.
    pub title: Option<SharedString>,
    /// `--description`.
    pub description: Option<SharedString>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            body_height: DEFAULT_BODY_HEIGHT,
            title: Some(SharedString::new_static("Sidebar")),
            description: None,
        }
    }
}

impl Params {
    /// The panel this cell describes. `empty` clears the header, exactly as
    /// `dialog`'s does.
    #[must_use]
    pub fn sheet(&self, cell: &Cell) -> Sheet {
        let empty = cell.has(StateFlag::Empty);
        Sheet {
            body_height: px(f32::from(self.body_height)),
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
            "--body-height" => self.body_height = pixels(&value(args, option)?, option)?,
            "--title" => self.title = Some(SharedString::from(value(args, option)?)),
            "--description" => self.description = Some(SharedString::from(value(args, option)?)),
            _ => return Ok(false),
        }
        Ok(true)
    }

    // No number here is measured against anything real (module docs, point
    // 4), so the window simply keeps the surface's own floor.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        out.push_str(" · no live call site mounts a sheet at all, so this cell has no reference");
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no header");
        }
    }

    // `render` is never called on this surface — see `dialog`'s equivalent
    // note.
    fn render(&self, _cell: &Cell, _theme: &Theme, _anchors: &dyn AnchorSink) -> AnyElement {
        unreachable!(
            "sheet needs window/App context; render_row calls render_ctx, never render \
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
        self.sheet(cell).render(window, cx, theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--body-height <px>",
            format!("how tall the call site's own content is [{DEFAULT_BODY_HEIGHT}]"),
        ),
        ("--title <text>", "render a SheetTitle".into()),
        ("--description <text>", "render a SheetDescription".into()),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_BODY_HEIGHT, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sheet"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sheet cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_have_a_title_and_no_description() {
        let bag = Params::default();
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.title.as_deref(), Some("Sidebar"));
        assert_eq!(bag.description, None);
    }

    /// `empty` clears the header.
    #[test]
    fn empty_clears_the_header() {
        let empty_cell = cell(&["--flags", "empty"]);
        let sheet = params_of(&empty_cell).sheet(&empty_cell);
        assert_eq!(sheet.title, None);
        assert_eq!(sheet.description, None);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

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
        }
    }

    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--body-height", "-1"],
            vec!["--body-height"],
            vec!["--title"],
        ] {
            let mut full = vec!["--surface", "sheet"];
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

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        let option = "--body-height";
        let line = ["--surface", "git-status-row", option, "24"];
        assert!(
            matches!(
                Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                Err(ParseError::Rejected(_)),
            ),
            "{option} should not be a git-status-row option",
        );
    }

    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--body-height", "--title", "--description"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("sheet"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "sheet");
        assert_eq!(SURFACE.root, "sheet-popup");
        assert!(SURFACE.full_bleed);
    }
}
