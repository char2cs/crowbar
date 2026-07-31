//! `--surface dropdown-menu` — the first Phase 2 surface.
//!
//! An **open** dropdown menu popup, drawn in one cell of the §8.3 matrix. The
//! fixture is the live comment menu from
//! `web/src/features/git/components/review-thread-item.tsx`, which is the only
//! `dropdown-menu` in the app that is neither inside the Plate editor nor hidden
//! above a 720px viewport — so it is the one a parity run can drive at the
//! matrix's widths.
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus` | the same paint. There is **no `hover:` rule** anywhere in `dropdown-menu.tsx`; the only interaction style a row carries is `focus:`, and `base-ui` reaches it by moving DOM focus to the row under the pointer. Which row is `--focus-row`. |
//! | `selected` | the tick on a checkbox or radio row. It needs `--tick`, because the comment menu has none — the caption says so rather than rendering a cell that cannot fail. |
//! | `empty` | **unmodelled.** A menu with no rows is not a state the app has: every menu in it renders at least one unconditional row. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//!
//! # Two options that are not matrix axes
//!
//! `--anchor-width` and `--min-width` are the popup's two width declarations,
//! and both are runtime inputs rather than component properties:
//! `w-(--anchor-width)` is written onto the popup by Floating UI from the
//! trigger's measured box, and `min-w-*` comes from a call site's `className`.
//! The comment menu's trigger is `size-6` and its `className` is `min-w-40`, so
//! the defaults here are 24 and 160 — and 160 is what wins, which is the CSS
//! clamp the layout test pins.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::dropdown_menu::{
    DropdownMenu, ID_CHECKBOX_ITEM, ID_LABEL, ID_RADIO_ITEM, MenuEntry, RowKind,
};
use crowbar_ui::components::{AnchorSink, dropdown_menu};
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, index, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "dropdown-menu",
    root: dropdown_menu::ID_POPUP,
    unmodelled: &[StateFlag::Empty, StateFlag::Loading, StateFlag::Error],
    // A four-entry popup is a little over 100px; a label and a `--tick` row push
    // it further, and the caption sits below it. A floor rather than a ceiling,
    // and the whole answer here: this surface drives no height (see
    // `driven_height`), because a popup's height is its rows'.
    min_window_height: 220,
    options,
    params: || Box::new(Params::default()),
};

/// `--anchor-width`'s default: `size-6` on the comment menu's `DotsThree`
/// trigger, which is what Floating UI measures and writes as `--anchor-width`.
pub const DEFAULT_ANCHOR_WIDTH: u16 = 24;

/// `--min-width`'s default: the comment menu's own `className="min-w-40"`,
/// which tailwind-merge keeps in place of the primitive's `min-w-32`.
pub const DEFAULT_MIN_WIDTH: u16 = 160;

/// `--focus-row`'s default: the **second** row, which is the one `--content`
/// moves — so the focused cell and the content cell interact instead of being
/// independent.
pub const DEFAULT_FOCUS_ROW: usize = 1;

/// Which tick primitive `--focus-row` is rendered as.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Tick {
    /// A plain `DropdownMenuItem`: no gutter, no tick, and `selected` paints
    /// nothing. The comment menu's own shape, and the default.
    #[default]
    None,
    /// `DropdownMenuCheckboxItem`.
    Checkbox,
    /// `DropdownMenuRadioItem` — the same class list under another name and
    /// another anchor id, which is the only part of the difference the differ
    /// can see. Live in the Plate toolbar's `ToolbarMenuGroup`.
    Radio,
}

impl Tick {
    /// Its word on the command line and in the caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Checkbox => "checkbox",
            Self::Radio => "radio",
        }
    }

    /// The row primitive it selects.
    #[must_use]
    pub const fn kind(self) -> RowKind {
        match self {
            Self::None => RowKind::Item,
            Self::Checkbox => RowKind::CheckboxItem,
            Self::Radio => RowKind::RadioItem,
        }
    }

    /// The anchor `dropdown-menu.tsx` puts on the primitive, which changes with
    /// it: a row that becomes a checkbox item stops being `menu-item-copy` and
    /// becomes `menu-checkbox-item`, on both sides.
    #[must_use]
    const fn anchor(self) -> Option<&'static str> {
        match self {
            Self::None => None,
            Self::Checkbox => Some(ID_CHECKBOX_ITEM),
            Self::Radio => Some(ID_RADIO_ITEM),
        }
    }

    fn parse(raw: &str) -> Result<Self, ParseError> {
        match raw {
            "none" => Ok(Self::None),
            "checkbox" => Ok(Self::Checkbox),
            "radio" => Ok(Self::Radio),
            other => Err(ParseError::Rejected(format!(
                "--tick takes none, checkbox or radio, not {other}"
            ))),
        }
    }
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--anchor-width`: `w-(--anchor-width)`, the trigger's measured width.
    pub anchor_width: u16,
    /// `--min-width`: `min-w-32`, or whatever a call site's `className` raises
    /// it to.
    pub min_width: u16,
    /// `--focus-row`: which row the `hover`/`focus` flags highlight, and which
    /// row `--tick` and `--shortcut` apply to. Zero-based over the popup's
    /// *rows*, so a `--label` does not shift the numbering.
    pub focus_row: usize,
    /// `--disable-row`: which row is `data-disabled`. A row parameter, not a
    /// §8.3 flag — the matrix vocabulary is fixed at six words and this is not
    /// one of them.
    pub disable_row: Option<usize>,
    /// `--tick`.
    pub tick: Tick,
    /// `--label`: a `DropdownMenuLabel` prepended to the popup.
    pub label: Option<SharedString>,
    /// `--shortcut`: a `DropdownMenuShortcut` on `--focus-row`.
    pub shortcut: Option<SharedString>,
    /// `--inset`: `inset` on every row and on the label.
    pub inset: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            anchor_width: DEFAULT_ANCHOR_WIDTH,
            min_width: DEFAULT_MIN_WIDTH,
            focus_row: DEFAULT_FOCUS_ROW,
            disable_row: None,
            tick: Tick::None,
            label: None,
            shortcut: None,
            inset: false,
        }
    }
}

impl Params {
    /// The popup this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling entries from scratch — so that a bare
    /// `--surface dropdown-menu` renders the menu the reference actually has.
    #[must_use]
    pub fn menu(&self, cell: &Cell) -> DropdownMenu {
        let mut menu = DropdownMenu::fixture(cell.content);
        menu.anchor_width = px(f32::from(self.anchor_width));
        menu.min_width = px(f32::from(self.min_width));

        // `hover` and `focus` are one paint here; see the module docs.
        let focused = cell.has(StateFlag::Hover) || cell.has(StateFlag::Focus);
        let checked = cell.has(StateFlag::Selected);

        for (row_index, row) in menu
            .entries
            .iter_mut()
            .filter_map(|entry| match entry {
                MenuEntry::Row(row) => Some(row),
                MenuEntry::Label { .. } | MenuEntry::Separator { .. } => None,
            })
            .enumerate()
        {
            row.inset = self.inset;
            row.state.disabled = self.disable_row == Some(row_index);
            if row_index != self.focus_row {
                continue;
            }
            row.state.focused = focused;
            row.kind = self.tick.kind();
            row.shortcut.clone_from(&self.shortcut);
            if let Some(anchor) = self.tick.anchor() {
                // The primitive changed, so the id `dropdown-menu.tsx` writes
                // changed with it. Leaving `menu-item-copy` here would name an
                // anchor the reference cannot produce.
                row.id = SharedString::new_static(anchor);
                row.state.checked = checked;
            }
        }

        if let Some(text) = &self.label {
            menu.entries.insert(
                0,
                MenuEntry::Label {
                    id: SharedString::new_static(ID_LABEL),
                    text: text.clone(),
                    inset: self.inset,
                },
            );
        }
        menu
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--anchor-width" => self.anchor_width = pixels(&value(args, option)?, option)?,
            "--min-width" => self.min_width = pixels(&value(args, option)?, option)?,
            "--focus-row" => self.focus_row = index(&value(args, option)?, option)?,
            "--disable-row" => self.disable_row = Some(index(&value(args, option)?, option)?),
            "--tick" => self.tick = Tick::parse(&value(args, option)?)?,
            "--label" => self.label = Some(SharedString::from(value(args, option)?)),
            "--shortcut" => self.shortcut = Some(SharedString::from(value(args, option)?)),
            "--inset" => self.inset = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Every option this surface takes is a *width* — the popup's
    /// height is however tall its rows come out, and no number on the command
    /// line decides it. A surface with nothing for the window to follow keeps
    /// its floor, which is exactly what this one has always had.
    ///
    /// If a later option did move the popup's height — a `--rows`, say — this
    /// would be where it is said, and the window would follow it. Until then a
    /// popup that outgrew the floor is caught where every clipped surface is:
    /// `row_snapshot::emit` refuses a frame whose anchors the window cut.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// The popup's two width declarations, the focused row, and — where it
    /// applies — the fact that a flag was asked for that this cell cannot paint.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · anchor {}px · min {}px · focus row {}",
            self.anchor_width, self.min_width, self.focus_row,
        );
        if self.tick != Tick::None {
            let _ = write!(out, " · tick {}", self.tick.name());
        }
        if let Some(row) = self.disable_row {
            let _ = write!(out, " · disabled row {row}");
        }
        if self.inset {
            out.push_str(" · inset");
        }
        // A `selected` cell with no tick row compares resting against resting,
        // which is the cheapest possible fake convergence. `Surface::unmodelled`
        // is per surface and this is per *cell*, so it is said here instead of
        // being silently rendered.
        if cell.has(StateFlag::Selected) && self.tick == Tick::None {
            out.push_str(" · selected: no tick row, so this cell cannot fail");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.menu(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--anchor-width <px>",
            format!("w-(--anchor-width): the trigger's width [{DEFAULT_ANCHOR_WIDTH}]"),
        ),
        (
            "--min-width <px>",
            format!("min-w-*, from the call site's className [{DEFAULT_MIN_WIDTH}]"),
        ),
        (
            "--focus-row <n>",
            format!("which row hover/focus highlights [{DEFAULT_FOCUS_ROW}]"),
        ),
        ("--disable-row <n>", "data-disabled on one row [none]".into()),
        (
            "--tick none|checkbox|radio",
            "render --focus-row as a tick item; `selected` needs it [none]".into(),
        ),
        ("--label <text>", "prepend a DropdownMenuLabel".into()),
        (
            "--shortcut <text>",
            "a DropdownMenuShortcut on --focus-row".into(),
        ),
        ("--inset", "inset on every row and the label".into()),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{
        DEFAULT_ANCHOR_WIDTH, DEFAULT_FOCUS_ROW, DEFAULT_MIN_WIDTH, Params, SURFACE, Tick, options,
    };
    use crate::row_surface::{Cell, ParseError};
    use crowbar_ui::components::dropdown_menu::{MenuEntry, MenuRow, RowKind, RowVariant};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "dropdown-menu"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// The bag the cell carries, downcast back to this surface's own type.
    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a dropdown-menu cell carries this surface's bag")
    }

    /// The rows the cell's popup ends up with, in DOM order.
    fn rows(cell: &Cell) -> Vec<MenuRow> {
        params_of(cell)
            .menu(cell)
            .entries
            .into_iter()
            .filter_map(|entry| match entry {
                MenuEntry::Row(row) => Some(row),
                MenuEntry::Label { .. } | MenuEntry::Separator { .. } => None,
            })
            .collect()
    }

    #[test]
    fn the_defaults_are_the_live_comment_menus() {
        let bag = Params::default();

        assert_eq!(bag.anchor_width, DEFAULT_ANCHOR_WIDTH);
        assert_eq!(bag.min_width, DEFAULT_MIN_WIDTH);
        assert_eq!(bag.focus_row, DEFAULT_FOCUS_ROW);
        assert_eq!(bag.tick, Tick::None);
        assert_eq!(bag.disable_row, None);
        assert!(!bag.inset);

        let menu = bag.menu(&cell(&[]));
        assert_eq!(menu.anchor_width, px(24.0));
        assert_eq!(menu.min_width, px(160.0));
        assert_eq!(menu.entries.len(), 4);
    }

    /// `hover` and `focus` are the same paint, and they reach **one** row.
    #[test]
    fn both_interaction_flags_focus_the_same_single_row() {
        for flag in ["hover", "focus"] {
            let rows = rows(&cell(&["--flags", flag]));
            let focused: Vec<bool> = rows.iter().map(|row| row.state.focused).collect();
            assert_eq!(focused, [false, true, false], "{flag}");
        }

        // And nothing is focused at rest.
        let resting = rows(&cell(&[]));
        assert!(resting.iter().all(|row| !row.state.focused));

        // `--focus-row` moves it.
        let moved = rows(&cell(&["--flags", "focus", "--focus-row", "2"]));
        let focused: Vec<bool> = moved.iter().map(|row| row.state.focused).collect();
        assert_eq!(focused, [false, false, true]);
    }

    /// `--tick` changes the primitive **and its anchor id**, because
    /// `dropdown-menu.tsx` writes a different default id for a different
    /// primitive. Leaving the item's id would name an anchor the reference
    /// cannot produce, which is a `FieldPresence` delta that forgives nothing.
    #[test]
    fn a_tick_row_takes_its_primitives_own_anchor() {
        for (word, kind, id) in [
            ("checkbox", RowKind::CheckboxItem, "menu-checkbox-item"),
            ("radio", RowKind::RadioItem, "menu-radio-item"),
        ] {
            let rows = rows(&cell(&["--tick", word, "--flags", "selected"]));

            assert_eq!(rows[1].kind, kind, "{word}");
            assert_eq!(rows[1].id, id, "{word}");
            assert!(rows[1].state.checked, "{word}");
            // And only that row: the others keep their item ids and are not
            // ticked.
            assert_eq!(rows[0].id, "menu-item-edit");
            assert_eq!(rows[0].kind, RowKind::Item);
            assert!(!rows[0].state.checked);
        }

        // Without `--tick` the flag reaches nothing, which is why the caption
        // says so.
        let untouched = rows(&cell(&["--flags", "selected"]));
        assert!(untouched.iter().all(|row| !row.state.checked));
        assert_eq!(untouched[1].id, "menu-item-copy");
    }

    /// The destructive row is the fixture's third, and no option moves it —
    /// the variant is the menu's, not the cell's.
    #[test]
    fn the_destructive_row_stays_destructive_through_every_option() {
        for line in [
            vec![],
            vec!["--flags", "focus"],
            vec!["--tick", "checkbox"],
            vec!["--inset"],
            vec!["--disable-row", "2"],
        ] {
            let rows = rows(&cell(&line));
            assert_eq!(rows[2].variant, RowVariant::Destructive, "{line:?}");
            assert_eq!(rows[0].variant, RowVariant::Default, "{line:?}");
        }
    }

    /// `--label` prepends an entry **without** shifting the row numbering, which
    /// is what makes `--focus-row` mean the same thing with and without one.
    #[test]
    fn a_label_does_not_shift_the_row_numbering() {
        let with = cell(&["--label", "Comment", "--flags", "focus"]);
        let menu = params_of(&with).menu(&with);

        assert!(matches!(menu.entries[0], MenuEntry::Label { .. }));
        assert_eq!(menu.entries.len(), 5);

        let focused: Vec<bool> = rows(&with).iter().map(|row| row.state.focused).collect();
        assert_eq!(focused, [false, true, false]);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--tick", "check"],
            vec!["--tick"],
            vec!["--anchor-width", "wide"],
            vec!["--anchor-width"],
            vec!["--min-width", "-1"],
            vec!["--focus-row", "first"],
            vec!["--disable-row"],
            vec!["--label"],
            vec!["--shortcut"],
        ] {
            let mut full = vec!["--surface", "dropdown-menu"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "dropdown-menu", "--tick", "check"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("`check` is not a tick kind");
        };
        assert!(complaint.contains("checkbox"), "{complaint}");
        assert!(complaint.contains("radio"), "{complaint}");
    }

    /// **These options belong to this surface and to no other.** Driving one on
    /// a row surface is a rejection, which is the property the registry exists
    /// for: a surface's vocabulary does not leak into the shared parser.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--anchor-width", "--min-width", "--tick", "--inset"] {
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

    /// Every option the parser accepts appears in the usage, or driving the
    /// matrix means reading the source.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in [
            "--anchor-width",
            "--min-width",
            "--focus-row",
            "--disable-row",
            "--tick",
            "--label",
            "--shortcut",
            "--inset",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("dropdown-menu"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "dropdown-menu");
        assert_eq!(SURFACE.root, "menu-popup");
    }
}
