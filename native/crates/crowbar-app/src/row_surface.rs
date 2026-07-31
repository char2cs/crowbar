//! The Phase 1 gate surface: one [`GitStatusRow`] in one §8.3 matrix cell.
//!
//! The whole point of this module is that the matrix cell is **data on the
//! command line**, not an interaction to be synthesised. `--flags hover` paints
//! the hovered row; it does not move a pointer. That is not a shortcut:
//! `crowbar-driver`'s extractor reads an element's *base* style during
//! `prepaint`, and gpui only materialises a `.hover(…)` refinement once a hitbox
//! has actually been hit — so a row whose hover lived in a refinement would
//! report its resting paint and the hovered cells of the matrix would converge
//! while proving nothing.
//!
//! ```text
//! crowbar-app --width 320 --theme dark --content overflow --flags hover,selected
//! ```
//!
//! Every option has a default, so a bare `cargo run -p crowbar-app` renders a
//! cell rather than a usage message.

use std::fmt::Write as _;

use crowbar_ui::components::{AnchorSink, ContentLength, GitStatusRow, RowState, TrailingContent};
use crowbar_ui::{Appearance, Theme};
use gpui::{
    Context, IntoElement, ParentElement as _, Pixels, Render, SharedString, Size, Styled as _,
    Window, div, px, relative, size,
};

/// The horizontal inset the surface is drawn at inside the window.
///
/// Non-zero on purpose, and for the same reason `driver_surface.rs` offsets
/// itself: with the root anchor at the window origin the root-relative
/// arithmetic in the snapshot would be a tautology, and a snapshot that is only
/// right at the origin proves nothing.
pub const INSET_X: f32 = 24.0;

/// The vertical inset. See [`INSET_X`].
pub const INSET_Y: f32 = 16.0;

/// The surface's name in a snapshot.
pub const SURFACE: &str = "git-status-row";

/// Why a command line did not produce a cell.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ParseError {
    /// `--help`: print the usage and stop, successfully.
    HelpRequested,
    /// Something the parser could not accept, already phrased for a human.
    Rejected(String),
}

/// The §8.3 state vocabulary, spelled exactly as `native/oracle/ANCHORS.md`
/// v1.1 fixes it.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum StateFlag {
    /// The row has nothing on its trailing edge: no badge, no counts.
    Empty,
    /// **No original.** See [`StateFlag::unmodelled`].
    Loading,
    /// **No original.** See [`StateFlag::unmodelled`].
    Error,
    /// The pointer is over the row.
    Hover,
    /// The row holds keyboard focus.
    Focus,
    /// `data-active='true'`.
    Selected,
}

impl StateFlag {
    /// Its name in the snapshot's `state.flags`.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Empty => "empty",
            Self::Loading => "loading",
            Self::Error => "error",
            Self::Hover => "hover",
            Self::Focus => "focus",
            Self::Selected => "selected",
        }
    }

    /// Whether the React original has no state to compare against.
    ///
    /// `GitFileItem` has no loading and no error rendering — not a gap in this
    /// port, a gap in the thing being ported. Driving those cells renders the
    /// resting row on both sides, so they compare equal and **prove nothing**.
    /// Said out loud on stderr rather than quietly rendered, because a matrix
    /// cell that cannot fail is the cheapest possible fake convergence.
    #[must_use]
    pub const fn unmodelled(self) -> bool {
        matches!(self, Self::Loading | Self::Error)
    }

    fn parse(word: &str) -> Option<Self> {
        match word {
            "empty" => Some(Self::Empty),
            "loading" => Some(Self::Loading),
            "error" => Some(Self::Error),
            "hover" => Some(Self::Hover),
            "focus" => Some(Self::Focus),
            "selected" => Some(Self::Selected),
            _ => None,
        }
    }
}

/// One cell of the §8.3 matrix, plus the row parameters the matrix vocabulary
/// has no word for.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Cell {
    /// Surface width in logical pixels — the width the row is given, not the
    /// window's.
    pub width: u16,
    /// Which token table is in force.
    pub appearance: Appearance,
    /// Which of the three fixture strings the row shows.
    pub content: ContentLength,
    /// The §8.3 state flags, in the order they were given.
    pub flags: Vec<StateFlag>,
    /// `indentLevel`.
    pub depth: u16,
    /// The depth of the row above; caps the guides at the top of a run.
    pub previous_depth: u16,
    /// The depth of the row below; caps them at the bottom.
    pub next_depth: u16,
    /// `showDirectory`.
    pub show_directory: bool,
    /// `showFileIcon`.
    pub show_file_icon: bool,
    /// `compactGitStatusBadges`.
    pub compact: bool,
    /// `diffStats.additions`. Zero omits the `git-row-added` anchor entirely,
    /// exactly as `additions > 0` gates the span in the React original.
    pub additions: u32,
    /// `diffStats.deletions`. Zero omits `git-row-deleted`.
    ///
    /// **Configurable because the reference is.** The two sides have to render
    /// the *same content* or the comparison is between two different rows: the
    /// first gate run had the native fixture on `+12 / -3` against a reference
    /// row showing `+1` and no deletions, which produced four deltas that said
    /// nothing about the port.
    pub deletions: u32,
}

impl Default for Cell {
    fn default() -> Self {
        Self {
            width: 320,
            appearance: Appearance::Dark,
            content: ContentLength::Normal,
            flags: Vec::new(),
            // Not zero: at depth 0 `TreeGuides` renders nothing, so three of the
            // contract's nine anchors would never appear and the indent
            // arithmetic would be invisible at runtime. Two levels gives
            // `git-row-guide-0` and `git-row-guide-1`.
            depth: 2,
            previous_depth: 2,
            next_depth: 2,
            show_directory: true,
            show_file_icon: true,
            compact: false,
            // The live fixture's counts, so a bare `cargo run` still renders
            // the row the gate was designed around.
            additions: 12,
            deletions: 3,
        }
    }
}

impl Cell {
    /// Whether the cell carries a flag.
    #[must_use]
    pub fn has(&self, flag: StateFlag) -> bool {
        self.flags.contains(&flag)
    }

    /// The flags whose cells cannot fail, because the original has no such
    /// state to disagree with.
    #[must_use]
    pub fn unmodelled_flags(&self) -> Vec<StateFlag> {
        self.flags
            .iter()
            .copied()
            .filter(|flag| flag.unmodelled())
            .collect()
    }

    /// The visual state the row is painted in.
    #[must_use]
    pub fn row_state(&self) -> RowState {
        RowState {
            hovered: self.has(StateFlag::Hover),
            focused: self.has(StateFlag::Focus),
            selected: self.has(StateFlag::Selected),
        }
    }

    /// The row this cell describes.
    #[must_use]
    pub fn row(&self) -> GitStatusRow {
        let mut row = GitStatusRow::fixture(self.content);
        row.depth = self.depth;
        row.previous_depth = self.previous_depth;
        row.next_depth = self.next_depth;
        row.show_directory = self.show_directory;
        row.show_file_icon = self.show_file_icon;
        row.state = self.row_state();
        row.trailing = if self.has(StateFlag::Empty) {
            TrailingContent::empty()
        } else {
            TrailingContent {
                compact: self.compact,
                additions: self.additions,
                deletions: self.deletions,
                ..row.trailing
            }
        };
        row
    }

    /// The token table this cell is drawn from.
    #[must_use]
    pub fn theme(&self) -> Theme {
        Theme::for_appearance(self.appearance)
    }

    /// The surface width, as gpui takes it.
    #[must_use]
    pub fn width_px(&self) -> Pixels {
        px(f32::from(self.width))
    }

    /// A one-line description, for the caption and for stderr.
    #[must_use]
    pub fn describe(&self) -> String {
        let theme = match self.appearance {
            Appearance::Light => "light",
            Appearance::Dark => "dark",
        };
        let content = match self.content {
            ContentLength::Short => "short",
            ContentLength::Normal => "normal",
            ContentLength::Overflow => "overflow",
        };
        let mut names: Vec<&str> = self.flags.iter().map(|flag| flag.name()).collect();
        names.sort_unstable();
        let flags = if names.is_empty() {
            "-".to_owned()
        } else {
            names.join(",")
        };
        let mut out = format!(
            "{SURFACE} · {}px · {theme} · {content} · flags {flags} · depth {}",
            self.width, self.depth,
        );
        if !self.show_directory {
            out.push_str(" · no-directory");
        }
        if !self.show_file_icon {
            out.push_str(" · no-icon");
        }
        out
    }

    /// Parses the cell out of the command line.
    ///
    /// # Errors
    ///
    /// [`ParseError::Rejected`] for an unknown option, a missing value or a
    /// value outside the fixed vocabulary; [`ParseError::HelpRequested`] for
    /// `--help`, which is a request to print the usage rather than a mistake.
    pub fn parse<I: IntoIterator<Item = String>>(args: I) -> Result<Self, ParseError> {
        let mut cell = Self::default();
        let mut previous_depth = None;
        let mut next_depth = None;
        let mut args = args.into_iter();

        while let Some(arg) = args.next() {
            match arg.as_str() {
                "--width" => cell.width = parse_u16(&value(&mut args, &arg)?)?,
                "--theme" => cell.appearance = parse_appearance(&value(&mut args, &arg)?)?,
                "--content" => cell.content = parse_content(&value(&mut args, &arg)?)?,
                "--flags" => cell.flags = parse_flags(&value(&mut args, &arg)?)?,
                "--depth" => cell.depth = parse_u16(&value(&mut args, &arg)?)?,
                "--prev-depth" => previous_depth = Some(parse_u16(&value(&mut args, &arg)?)?),
                "--next-depth" => next_depth = Some(parse_u16(&value(&mut args, &arg)?)?),
                "--added" => cell.additions = parse_u32(&value(&mut args, &arg)?)?,
                "--deleted" => cell.deletions = parse_u32(&value(&mut args, &arg)?)?,
                "--no-directory" => cell.show_directory = false,
                "--no-icon" => cell.show_file_icon = false,
                "--compact" => cell.compact = true,
                "--help" | "-h" => return Err(ParseError::HelpRequested),
                other => return Err(ParseError::Rejected(format!("unknown option {other}"))),
            }
        }

        // Defaulted after the loop, not during it: `--depth` may be given after
        // them, and `TreeGuides`' own default is "the same depth as this row".
        cell.previous_depth = previous_depth.unwrap_or(cell.depth);
        cell.next_depth = next_depth.unwrap_or(cell.depth);

        if cell.width == 0 {
            return Err(ParseError::Rejected(
                "--width must be greater than zero".to_owned(),
            ));
        }
        Ok(cell)
    }
}

fn value<I: Iterator<Item = String>>(args: &mut I, option: &str) -> Result<String, ParseError> {
    args.next()
        .ok_or_else(|| ParseError::Rejected(format!("{option} needs a value")))
}

fn parse_u16(raw: &str) -> Result<u16, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("{raw} is not a whole number")))
}

fn parse_u32(raw: &str) -> Result<u32, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("{raw} is not a whole number")))
}

fn parse_appearance(raw: &str) -> Result<Appearance, ParseError> {
    match raw {
        "light" => Ok(Appearance::Light),
        "dark" => Ok(Appearance::Dark),
        other => Err(ParseError::Rejected(format!(
            "--theme takes light or dark, not {other}"
        ))),
    }
}

fn parse_content(raw: &str) -> Result<ContentLength, ParseError> {
    match raw {
        "short" => Ok(ContentLength::Short),
        "normal" => Ok(ContentLength::Normal),
        "overflow" => Ok(ContentLength::Overflow),
        other => Err(ParseError::Rejected(format!(
            "--content takes short, normal or overflow, not {other}"
        ))),
    }
}

/// The flags, deduplicated but kept in the order they were given.
fn parse_flags(raw: &str) -> Result<Vec<StateFlag>, ParseError> {
    let mut flags = Vec::new();
    for word in raw.split(',').filter(|word| !word.is_empty()) {
        let flag = StateFlag::parse(word).ok_or_else(|| {
            ParseError::Rejected(format!(
                "{word} is not a state flag: empty, loading, error, hover, focus, selected"
            ))
        })?;
        if !flags.contains(&flag) {
            flags.push(flag);
        }
    }
    Ok(flags)
}

/// What to print for `--help`, and alongside a rejection.
#[must_use]
pub fn usage() -> String {
    let mut out = String::from(
        "crowbar-app — the Phase 1 gate surface: one git-status row in one matrix cell.\n\n\
         Options (all optional; the defaults are one cell):\n",
    );
    for (option, description) in [
        ("--width <px>", "surface width, logical px [320]"),
        ("--theme light|dark", "token table [dark]"),
        (
            "--content short|normal|overflow",
            "which fixture string [normal]",
        ),
        (
            "--flags <csv>",
            "empty,loading,error,hover,focus,selected [none]",
        ),
        ("--depth <n>", "indent level [2]"),
        ("--prev-depth <n>", "depth of the row above [= --depth]"),
        ("--next-depth <n>", "depth of the row below [= --depth]"),
        ("--added <n>", "diffStats.additions; 0 omits the span [12]"),
        ("--deleted <n>", "diffStats.deletions; 0 omits the span [3]"),
        ("--no-directory", "showDirectory = false"),
        ("--no-icon", "showFileIcon = false"),
        ("--compact", "compactGitStatusBadges = true"),
    ] {
        let _ = writeln!(out, "  {option:<32}{description}");
    }
    out.push_str(
        "\nA driver build (--features driver) additionally honours\n  \
         CROWBAR_ROW_SNAPSHOT=<path|->   emit one snapshot of the first frame and quit\n",
    );
    out
}

/// The window: the row at the requested width, and a caption naming the cell.
pub struct RowSurface {
    cell: Cell,
    anchors: Box<dyn AnchorSink>,
    caption: SharedString,
}

impl RowSurface {
    /// A surface for one cell.
    #[must_use]
    pub fn new(cell: Cell, anchors: Box<dyn AnchorSink>, caption: impl Into<SharedString>) -> Self {
        Self {
            cell,
            anchors,
            caption: caption.into(),
        }
    }

    /// The window that holds the surface, its inset and the caption.
    #[must_use]
    pub fn window_size(cell: &Cell) -> Size<Pixels> {
        size(
            cell.width_px() + px(INSET_X * 2.0),
            px(INSET_Y * 2.0 + 72.0),
        )
    }
}

impl Render for RowSurface {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = self.cell.theme();
        let row = self.cell.row();

        div()
            .size_full()
            .bg(theme.background)
            .pl(px(INSET_X))
            .pt(px(INSET_Y))
            .font_family(theme.font_sans.primary().unwrap_or("sans-serif"))
            .child(
                div()
                    .w(self.cell.width_px())
                    .child(row.render(&theme, self.anchors.as_ref())),
            )
            // Outside the root anchor, so it cannot reach the snapshot: every
            // bound is reported relative to `git-row-item`.
            .child(
                div()
                    .pt(px(12.0))
                    .text_size(theme.ui_text_xs.value())
                    .line_height(relative(1.35))
                    .text_color(theme.muted_foreground)
                    .child(self.caption.clone()),
            )
    }
}

#[cfg(test)]
mod tests {
    use super::{Cell, ParseError, RowSurface, StateFlag, usage};
    use crowbar_ui::Appearance;
    use crowbar_ui::components::ContentLength;

    fn parse(args: &[&str]) -> Result<Cell, ParseError> {
        Cell::parse(args.iter().map(|arg| (*arg).to_owned()))
    }

    #[test]
    fn no_arguments_is_one_cell_not_a_usage_error() {
        assert_eq!(parse(&[]), Ok(Cell::default()));
    }

    #[test]
    fn the_default_cell_shows_its_guides() {
        assert!(Cell::default().depth > 0, "the guide anchors need a depth");
    }

    #[test]
    fn every_option_parses() {
        let cell = parse(&[
            "--width",
            "240",
            "--theme",
            "light",
            "--content",
            "overflow",
            "--flags",
            "hover,selected",
            "--depth",
            "3",
            "--prev-depth",
            "1",
            "--next-depth",
            "5",
            "--added",
            "1",
            "--deleted",
            "0",
            "--no-directory",
            "--no-icon",
            "--compact",
        ])
        .expect("a well-formed command line");

        assert_eq!(cell.width, 240);
        assert_eq!(cell.appearance, Appearance::Light);
        assert_eq!(cell.content, ContentLength::Overflow);
        assert_eq!(cell.flags, vec![StateFlag::Hover, StateFlag::Selected]);
        assert_eq!(cell.depth, 3);
        assert_eq!(cell.previous_depth, 1);
        assert_eq!(cell.next_depth, 5);
        assert_eq!(cell.additions, 1);
        assert_eq!(cell.deletions, 0);
        assert!(!cell.show_directory);
        assert!(!cell.show_file_icon);
        assert!(cell.compact);
    }

    /// The reason the counts are on the command line at all: the two sides have
    /// to render the same content, and the live reference row shows `+1` with
    /// no deletions.
    #[test]
    fn the_counts_reach_the_row_and_a_zero_omits_its_span() {
        let row = parse(&["--added", "1", "--deleted", "0"])
            .expect("well-formed")
            .row();

        assert_eq!(row.trailing.additions, 1);
        assert_eq!(row.trailing.deletions, 0);
        assert!(row.trailing.has_counts(), "+1 still renders the group");
    }

    /// Both at zero drops the counts group entirely, which is what the React
    /// original's `hasDiffStats` does — while leaving the badge alone, because
    /// that is a different prop.
    #[test]
    fn zero_on_both_counts_leaves_only_the_badge() {
        let row = parse(&["--added", "0", "--deleted", "0"])
            .expect("well-formed")
            .row();

        assert!(!row.trailing.has_counts());
        assert!(row.trailing.uncommitted);
    }

    /// The defaults are the fixture the gate was designed around, so a bare
    /// `cargo run` is unchanged by the counts becoming configurable.
    #[test]
    fn the_counts_default_to_the_live_fixtures() {
        let cell = Cell::default();

        assert_eq!(cell.additions, 12);
        assert_eq!(cell.deletions, 3);
    }

    /// `TreeGuides` defaults both neighbours to this row's own depth, and
    /// `--depth` may be written after them.
    #[test]
    fn the_neighbour_depths_default_to_this_rows_depth() {
        let cell = parse(&["--prev-depth", "1", "--depth", "4"]).expect("well-formed");

        assert_eq!(cell.previous_depth, 1);
        assert_eq!(cell.next_depth, 4);
    }

    #[test]
    fn a_repeated_flag_is_kept_once_in_the_order_given() {
        let cell = parse(&["--flags", "selected,hover,selected"]).expect("well-formed");

        assert_eq!(cell.flags, vec![StateFlag::Selected, StateFlag::Hover]);
    }

    #[test]
    fn an_empty_flag_list_is_no_flags() {
        assert_eq!(parse(&["--flags", ""]).expect("well-formed").flags, vec![]);
    }

    #[test]
    fn the_vocabulary_is_closed() {
        for line in [
            vec!["--flags", "hovered"],
            vec!["--theme", "Dark"],
            vec!["--content", "long"],
            vec!["--nope"],
            vec!["--width"],
            vec!["--width", "wide"],
            vec!["--width", "0"],
            vec!["--added"],
            vec!["--added", "some"],
            vec!["--deleted", "-1"],
        ] {
            assert!(
                matches!(parse(&line), Err(ParseError::Rejected(_))),
                "{line:?} should have been rejected",
            );
        }
    }

    #[test]
    fn help_is_a_request_to_print_the_usage_not_a_mistake() {
        assert_eq!(parse(&["--help"]), Err(ParseError::HelpRequested));
        assert_eq!(parse(&["-h"]), Err(ParseError::HelpRequested));
    }

    /// Every option the parser accepts has to appear in the usage, or driving
    /// the matrix means reading the source.
    #[test]
    fn the_usage_names_every_option() {
        let usage = usage();
        for option in [
            "--width",
            "--theme",
            "--content",
            "--flags",
            "--depth",
            "--prev-depth",
            "--next-depth",
            "--added",
            "--deleted",
            "--no-directory",
            "--no-icon",
            "--compact",
            "CROWBAR_ROW_SNAPSHOT",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for flag in ["empty", "loading", "error", "hover", "focus", "selected"] {
            assert!(usage.contains(flag), "{flag} is missing from the usage");
        }
    }

    #[test]
    fn the_flags_reach_the_rows_visual_state() {
        let cell = parse(&["--flags", "hover,focus,selected"]).expect("well-formed");
        let state = cell.row_state();

        assert!(state.hovered);
        assert!(state.focused);
        assert!(state.selected);
        assert_eq!(cell.row().state, state);
    }

    /// The one flag with a real original: the row's trailing edge empties.
    #[test]
    fn the_empty_flag_clears_the_badge_and_the_counts() {
        let row = parse(&["--flags", "empty"]).expect("well-formed").row();

        assert!(!row.trailing.uncommitted);
        assert!(!row.trailing.has_counts());
    }

    #[test]
    fn loading_and_error_are_the_two_with_no_original() {
        let cell = parse(&["--flags", "loading,error,hover"]).expect("well-formed");

        assert_eq!(
            cell.unmodelled_flags(),
            vec![StateFlag::Loading, StateFlag::Error],
        );
        for flag in [
            StateFlag::Empty,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(!flag.unmodelled(), "{} has an original", flag.name());
        }
    }

    /// The vocabulary is `ANCHORS.md` v1.1's, lowercase, no synonyms — one side
    /// spelling `Dark` against the other's `dark` makes every comparison refuse.
    #[test]
    fn the_flag_names_are_the_contract_spelling() {
        assert_eq!(StateFlag::Empty.name(), "empty");
        assert_eq!(StateFlag::Loading.name(), "loading");
        assert_eq!(StateFlag::Error.name(), "error");
        assert_eq!(StateFlag::Hover.name(), "hover");
        assert_eq!(StateFlag::Focus.name(), "focus");
        assert_eq!(StateFlag::Selected.name(), "selected");
    }

    #[test]
    fn the_caption_names_the_cell() {
        let cell = parse(&["--width", "420", "--theme", "light", "--flags", "hover"])
            .expect("well-formed");
        let description = cell.describe();

        assert!(description.contains("git-status-row"));
        assert!(description.contains("420px"));
        assert!(description.contains("light"));
        assert!(description.contains("normal"));
        assert!(description.contains("hover"));
        assert!(description.contains("depth 2"));
    }

    #[test]
    fn the_cell_selects_the_token_table() {
        assert_eq!(
            parse(&["--theme", "light"]).expect("well-formed").theme(),
            crowbar_ui::Theme::LIGHT,
        );
        assert_eq!(
            parse(&["--theme", "dark"]).expect("well-formed").theme(),
            crowbar_ui::Theme::DARK,
        );
    }

    /// The window has to be wider than the surface, or the row is clipped and
    /// every `clipped` in the snapshot is an artefact of the window size.
    #[test]
    fn the_window_holds_the_surface_and_its_inset() {
        let cell = parse(&["--width", "420"]).expect("well-formed");
        let window = RowSurface::window_size(&cell);

        assert_eq!(window.width, cell.width_px() + gpui::px(48.0));
        assert!(window.height > gpui::px(24.0));
    }
}
