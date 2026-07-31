//! A Phase 1 gate surface — one row, in one §8.3 matrix cell.
//!
//! Two surfaces live behind `--surface`, and they divide the matrix between
//! them:
//!
//! | `--surface` | What it measures | Why |
//! |---|---|---|
//! | `git-status-row` (default) | the **geometry** axis | its filename and directory truncate against each other through three nested flex containers |
//! | `file-tree-row` | the **state** axis | it is the row that actually enters `hover`, `selected` and `focus` |
//!
//! The split is not tidiness. `SidebarTreeRow` takes an `active` prop that
//! neither live consumer passes, so `data-active` never fires on the git status
//! row: every `selected` cell of its matrix compares a resting row against a
//! resting row and converges while proving nothing. The file explorer row sets
//! `data-active` from `isActive` on every render **and** sits inside
//! `.file-tree-container`, which brings the scoped `:focus-visible` border rule
//! into play as well.
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
//! crowbar-app --surface file-tree-row --width 294 --theme dark --flags selected
//! ```
//!
//! Every option has a default, so a bare `cargo run -p crowbar-app` renders a
//! cell rather than a usage message.
//!
//! # `--git-status` is a row parameter, not a matrix flag
//!
//! ```text
//! crowbar-app --surface file-tree-row --git-status modified
//! ```
//!
//! The file explorer colours a filename by its git status
//! ([`crowbar_ui::components::GitStatus`]), and a reference fixture whose `a.ts`
//! is modified paints it amber while a port with no way to say so paints the
//! foreground. So the status has to be drivable — but it is **not** part of
//! §8.3's cell: `SurfaceState` carries width, theme, content and flags, and a
//! snapshot that smuggled a sixth quantity in there would make the differ refuse
//! comparisons it should make. It sits beside `--added` and `--depth`, which are
//! the other things the two apps have to be told to render the same.
//!
//! It is honoured on `file-tree-row` only, because `GitFileItem` pins its
//! filename at `text-foreground` and never moves it.
//!
//! # `--width` and `--viewport-width` are two different quantities
//!
//! `--width` is the **surface**: how wide the row is drawn, which is the
//! sidebar's width in the app being ported. `--viewport-width` is the
//! **window**, which is what a CSS `@media (width >= 40rem)` asks about — so it
//! is what selects the badge's `sm:` variant, and it is what goes into the
//! snapshot's `state.width`.
//!
//! Conflating them has already cost a run: a reference captured in a 569px
//! webview rendered the badge's narrow variant (20px tall, 12px face) against a
//! native row drawing the wide one (16px / 10px), and the differ reported four
//! geometry deltas that were neither side's fault. They are separate options
//! here because the matrix needs to vary the viewport while holding the surface
//! at the reference's fixed 294px sidebar.

use std::fmt::Write as _;

use crowbar_ui::components::{
    ALL_GIT_STATUSES, AnchorSink, Breakpoint, ContentLength, FileTreeRow, GitStatus, GitStatusRow,
    RowState, TrailingContent,
};
use crowbar_ui::{Appearance, Theme};
use gpui::{
    AnyElement, Context, IntoElement, ParentElement as _, Pixels, Render, SharedString, Size,
    Styled as _, Window, div, px, relative, size,
};

use crate::surface::{Surface, SurfaceOptions};

/// The horizontal inset the surface is drawn at inside the window.
///
/// Non-zero on purpose, and for the same reason `driver_surface.rs` offsets
/// itself: with the root anchor at the window origin the root-relative
/// arithmetic in the snapshot would be a tautology, and a snapshot that is only
/// right at the origin proves nothing.
pub const INSET_X: f32 = 24.0;

/// [`INSET_X`] as a whole number of pixels, for the `u16` width arithmetic the
/// command line works in. Held equal to it by `the_two_spellings_of_the_inset_agree`.
const INSET_X_WHOLE: u16 = 24;

/// The vertical inset. See [`INSET_X`].
pub const INSET_Y: f32 = 16.0;

/// The gap between the surface and the caption below it, in logical px.
///
/// Read by [`RowSurface::render`], which draws the gap, and by
/// [`CAPTION_HEIGHT`], which reserves room for it — one constant, so a window
/// that reserved a gap the view does not draw is not expressible.
const CAPTION_GAP: u16 = 12;

/// The caption's own line box, in whole logical px.
///
/// The caption is drawn at `ui_text_xs` (0.6875rem — 11px at gpui's 16px rem)
/// on a 1.35 line height, which is 14.85px, and 17 is the whole-pixel room it
/// is given. Deliberately a little generous: the caption is chrome that no
/// anchor is taken from, so a spare pixel or two costs the window nothing,
/// while a caption sliced in half is a run nobody can read back.
const CAPTION_LINE: u16 = 17;

/// The room the caption needs below the surface.
///
/// Named because two places have to agree on it: the window height, which
/// reserves it, and `sidebar-carousel`'s own floor, which was chosen to hold a
/// 640px carousel *and* this.
pub const CAPTION_HEIGHT: u16 = CAPTION_GAP + CAPTION_LINE;

/// The default `--viewport-width`, in logical px.
///
/// Chosen for one reason: it has to be **at or above** Tailwind's 640px `sm`
/// breakpoint, or introducing the option would silently change what every
/// existing invocation renders — the badge would drop from `sm:h-4`/10px to
/// `h-5`/12px and four geometry deltas would appear out of a flag nobody
/// passed. 800 clears it with room and holds any surface the matrix drives.
pub const DEFAULT_VIEWPORT_WIDTH: u16 = 800;

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
    /// Modelled on `git-status-row` only — see [`Surface::unmodelled`].
    Empty,
    /// **No original, on either surface.** See [`Surface::unmodelled`].
    Loading,
    /// **No original, on either surface.** See [`Surface::unmodelled`].
    Error,
    /// The pointer is over the row.
    Hover,
    /// The row holds keyboard focus. Paints a border on `file-tree-row` and
    /// nothing at all on `git-status-row`, whose `:focus-visible` rule is
    /// scoped to a container it is not inside.
    Focus,
    /// `data-active='true'`. **Only `file-tree-row` really enters this**: the
    /// git status row's `active` prop is passed by no live consumer.
    Selected,
}

/// The §8.3 state vocabulary in full, in `ANCHORS.md` v1.1's order.
///
/// Written down so the usage can say, per surface, which of them are real —
/// and so that adding a seventh flag without deciding what each surface does
/// with it is a compile error rather than a quietly missing line.
pub const ALL_FLAGS: [StateFlag; 6] = [
    StateFlag::Empty,
    StateFlag::Loading,
    StateFlag::Error,
    StateFlag::Hover,
    StateFlag::Focus,
    StateFlag::Selected,
];

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
    /// Which surface is under measurement — an entry in the registry
    /// `build.rs` builds out of `src/surfaces/`.
    ///
    /// A closed set still: the name reaches the snapshot's `surface` field and
    /// the root anchor reaches its `root`, and the differ refuses to compare two
    /// snapshots that disagree on either. What changed is *where* the set is
    /// written down — in the surfaces' own files rather than in an enum here,
    /// so that adding one does not touch another's code.
    pub surface: &'static Surface,
    /// The selected surface's own options.
    ///
    /// Every surface added from Phase 2 onwards puts its options here; the nine
    /// legacy fields below belong to the two Phase 1 surfaces and are the
    /// exception, for the reason `crate::surface`'s module docs give.
    ///
    /// Private, because it is a `dyn` and the typed way in is
    /// [`Cell::surface_params`].
    params: SurfaceOptions,
    /// Surface width in logical pixels — the width the row is given, not the
    /// window's.
    pub width: u16,
    /// Viewport width in logical pixels — the **window's**.
    ///
    /// This is the quantity a CSS media query asks about, so it is what selects
    /// the badge's `sm:` variant, and it is what the snapshot reports as
    /// `state.width`. It is not [`Cell::width`]: a 294px sidebar inside a
    /// 1200px window is a 1200px viewport, and the row it renders is the wide
    /// one.
    pub viewport_width: u16,
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
    /// The file's git status, which colours the name — `file-tree-row` only.
    ///
    /// **Configurable for the same reason the counts are**, and it cost a gate
    /// run to learn it a second time: driving both apps into
    /// `file-tree-row · dark · short · selected` left exactly one delta,
    /// `file-row-name.fg` `#f5f5f5ff` against `#fe9a00ff`, because the reference
    /// fixture's `a.ts` is modified and the port had no way to say so. A row
    /// parameter rather than a matrix flag: `SurfaceState` is §8.3's vocabulary
    /// and this is not in it, so it belongs beside `--added` and `--depth`.
    ///
    /// `None` — `--git-status none` — is the default and the unchanged file.
    pub git_status: Option<GitStatus>,
}

/// The surface a bare command line renders.
///
/// Named by path rather than taken from the head of the registry, which is
/// sorted by file name and would put whichever surface sorts first here. It has
/// to stay `git-status-row`: every invocation written before `--surface` existed
/// must keep rendering the row it rendered, and the archived gate runs are
/// evidence taken at that default.
///
/// This is the **only** place `crate::surfaces` names one surface in particular,
/// and it is a commitment rather than a dispatch.
fn default_surface() -> &'static Surface {
    &crate::surfaces::git_status_row::SURFACE
}

impl Default for Cell {
    fn default() -> Self {
        let surface = default_surface();
        Self {
            surface,
            params: SurfaceOptions::new((surface.params)()),
            width: 320,
            // Comfortably above Tailwind's 640px `sm` breakpoint, so every
            // invocation written before `--viewport-width` existed keeps
            // rendering the variant it was rendering — and wide enough to hold
            // the default surface and its insets several times over.
            viewport_width: DEFAULT_VIEWPORT_WIDTH,
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
            // The unchanged file. Every command line written before
            // `--git-status` existed keeps painting the name on the inherited
            // foreground, which is what it was painting.
            git_status: None,
        }
    }
}

impl Cell {
    /// Whether the cell carries a flag.
    #[must_use]
    pub fn has(&self, flag: StateFlag) -> bool {
        self.flags.contains(&flag)
    }

    /// The selected surface's own parameters, as the surface's own type.
    ///
    /// `None` when `T` is a different surface's bag, which is the honest answer:
    /// a `dropdown-menu` cell carries no `git-status-row` parameters.
    ///
    /// Only a surface's own tests read it — the binary never needs to know which
    /// surface it is holding, which is the whole point of the registry — so it
    /// is genuinely dead in a shipping build, exactly as `row_snapshot.rs` is.
    #[cfg_attr(not(test), allow(dead_code))]
    #[must_use]
    pub fn surface_params<T: 'static>(&self) -> Option<&T> {
        self.params.as_any().downcast_ref::<T>()
    }

    /// The flags whose cells cannot fail, because **this surface's** original
    /// has no such state to disagree with.
    #[must_use]
    pub fn unmodelled_flags(&self) -> Vec<StateFlag> {
        self.flags
            .iter()
            .copied()
            .filter(|flag| self.surface.unmodelled(*flag))
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

    /// The file explorer row this cell describes.
    ///
    /// The `empty` flag is deliberately not consulted: this row has no optional
    /// content to remove, which is why [`Surface::unmodelled`] calls the flag
    /// out on stderr instead of quietly rendering something for it.
    #[must_use]
    pub fn file_row(&self) -> FileTreeRow {
        let mut row = FileTreeRow::fixture(self.content);
        row.depth = self.depth;
        row.previous_depth = self.previous_depth;
        row.next_depth = self.next_depth;
        row.state = self.row_state();
        row.git_status = self.git_status;
        row
    }

    /// The git status row this cell describes.
    #[must_use]
    pub fn row(&self) -> GitStatusRow {
        let mut row = GitStatusRow::fixture(self.content);
        row.depth = self.depth;
        row.previous_depth = self.previous_depth;
        row.next_depth = self.next_depth;
        row.show_directory = self.show_directory;
        row.show_file_icon = self.show_file_icon;
        row.state = self.row_state();
        row.breakpoint = self.breakpoint();
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

    /// The viewport width, as gpui takes it. This is the window's width.
    #[must_use]
    pub fn viewport_width_px(&self) -> Pixels {
        px(f32::from(self.viewport_width))
    }

    /// Which side of the `sm` breakpoint this cell's **viewport** is on.
    #[must_use]
    pub fn breakpoint(&self) -> Breakpoint {
        Breakpoint::of(f32::from(self.viewport_width))
    }

    /// The narrowest window this cell's surface fits in without being clipped.
    ///
    /// A viewport narrower than this would cut the row at the window edge, and
    /// the driver reports a box cut horizontally by its clip as `clipped` — so
    /// every `clipped` in the snapshot would become an artefact of the window
    /// size rather than a fact about the truncation the gate exists to measure.
    #[must_use]
    pub fn minimum_viewport(&self) -> u16 {
        self.width.saturating_add(INSET_X_WHOLE * 2)
    }

    /// The room below the window's top inset **this cell** needs, caption
    /// included.
    ///
    /// # The window follows the surface (P2.5)
    ///
    /// Whichever is larger of the surface's own floor and what this cell drives
    /// its height to. Before P2.5 there was no `max`: the window was the floor,
    /// full stop, and each surface capped its height option below it —
    /// `--shell-height 1..=160`, `--height 1..=640`. The reasoning behind those
    /// caps was right and is kept: a surface cut at the window edge makes every
    /// `visible` in the snapshot an artefact of the window size, which is worse
    /// than refusing. What was wrong was the side it was applied to. The live
    /// IDE shell's `ResizablePanelGroup` is **1119px** tall, so a cap at 160
    /// made the only real reference unreachable — the surface could not be
    /// parity-tested at all, which is the same fake convergence arrived at by
    /// the other door.
    ///
    /// So the window moves instead of the surface, and the "never silently
    /// clipped" property is enforced where it can actually be observed:
    /// `row_snapshot::emit` refuses to write a snapshot whose anchors the window
    /// cut. See that function — this arithmetic decides what to *ask* the
    /// platform for, and only the platform decides what is granted.
    ///
    /// A surface that drives no height keeps its floor exactly, which is what
    /// holds `git-status-row` and `file-tree-row` at the geometry their archived
    /// gate runs were taken at.
    #[must_use]
    pub fn window_extent(&self) -> u16 {
        match self.params.driven_height(self) {
            Some(height) => self
                .surface
                .min_window_height
                .max(height.saturating_add(CAPTION_HEIGHT)),
            None => self.surface.min_window_height,
        }
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
        // The viewport is named alongside the surface width, not instead of it:
        // they are different quantities and a caption that showed only one is
        // how a reader concludes the badge changed size for no reason.
        let mut out = format!(
            "{} · {}px in a {}px viewport · {theme} · {content} · flags {flags} · depth {}",
            self.surface.name, self.width, self.viewport_width, self.depth,
        );
        // The parameters the *other* surfaces have no prop for, each named only
        // where it is honoured, by the surface that honours it. A caption that
        // announced `git modified` on a git status row would be describing a
        // colour nothing painted — and the surface is the only thing that knows
        // that, which is why this is a delegation and not a `match`.
        self.params.describe(self, &mut out);
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
        let words: Vec<String> = args.into_iter().collect();

        // `--surface` is read in its own pass, before anything else, because the
        // selected surface decides which *other* options exist. Reading it in
        // the loop would mean a `--tick checkbox --surface dropdown-menu` line
        // rejected its own surface's option, and a `--surface` written twice
        // would silently discard the first surface's parameters. One pass makes
        // the option order irrelevant, which is what the rest of this parser
        // already promises.
        let mut cell = Self::default();
        let mut selector = words.iter();
        while let Some(word) = selector.next() {
            if word == "--surface" {
                let name = selector
                    .next()
                    .ok_or_else(|| ParseError::Rejected("--surface needs a value".to_owned()))?;
                cell.surface = Surface::parse(name)?;
                cell.params = SurfaceOptions::new((cell.surface.params)());
            }
        }

        let mut previous_depth = None;
        let mut next_depth = None;
        let mut args = words.into_iter();

        while let Some(arg) = args.next() {
            match arg.as_str() {
                // Already consumed by the pass above; its value still has to be
                // stepped over here.
                "--surface" => {
                    value(&mut args, &arg)?;
                }
                "--width" => cell.width = parse_u16(&value(&mut args, &arg)?)?,
                "--viewport-width" => cell.viewport_width = parse_u16(&value(&mut args, &arg)?)?,
                "--theme" => cell.appearance = parse_appearance(&value(&mut args, &arg)?)?,
                "--content" => cell.content = parse_content(&value(&mut args, &arg)?)?,
                "--flags" => cell.flags = parse_flags(&value(&mut args, &arg)?)?,
                "--depth" => cell.depth = parse_u16(&value(&mut args, &arg)?)?,
                "--prev-depth" => previous_depth = Some(parse_u16(&value(&mut args, &arg)?)?),
                "--next-depth" => next_depth = Some(parse_u16(&value(&mut args, &arg)?)?),
                "--added" => cell.additions = parse_u32(&value(&mut args, &arg)?)?,
                "--deleted" => cell.deletions = parse_u32(&value(&mut args, &arg)?)?,
                "--git-status" => cell.git_status = parse_git_status(&value(&mut args, &arg)?)?,
                "--no-directory" => cell.show_directory = false,
                "--no-icon" => cell.show_file_icon = false,
                "--compact" => cell.compact = true,
                "--help" | "-h" => return Err(ParseError::HelpRequested),
                // The **one** arm every surface's own options come through. A
                // surface that does not recognise the word says so and the
                // rejection below is unchanged, so an option belonging to one
                // surface is still an error on another.
                other => {
                    if !cell.params.accept(other, &mut args)? {
                        return Err(ParseError::Rejected(format!("unknown option {other}")));
                    }
                }
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
        // Rejected rather than clamped, and rather than drawn anyway. A window
        // too narrow for the surface cuts the row at its edge, and the driver
        // reports a box cut horizontally by its clip as `clipped` — so every
        // `clipped` in the snapshot would be an artefact of the window size
        // instead of a fact about the truncation this gate exists to measure.
        // Clamping the window instead would leave `state.width` claiming a
        // viewport the run was not taken at, which is the same lie one layer
        // further down.
        let minimum = cell.minimum_viewport();
        if cell.viewport_width < minimum {
            return Err(ParseError::Rejected(format!(
                "--viewport-width {} is narrower than the {}px surface plus its {}px insets; \
                 the row would be cut at the window edge and every `clipped` in the snapshot \
                 would be an artefact of the window size. Give it at least {minimum}",
                cell.viewport_width, cell.width, INSET_X_WHOLE,
            )));
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

/// `none`, or one of [`ALL_GIT_STATUSES`]'s own names.
///
/// The vocabulary is generated from [`GitStatus::name`] rather than restated
/// here, so the word the command line takes and the word the component reports
/// cannot drift into two spellings of one status. `none` is the only word this
/// function adds, and it is the absence — which is why `GitStatus` has no
/// variant for it.
fn parse_git_status(raw: &str) -> Result<Option<GitStatus>, ParseError> {
    if raw == "none" {
        return Ok(None);
    }
    ALL_GIT_STATUSES
        .into_iter()
        .find(|status| status.name() == raw)
        .map(Some)
        .ok_or_else(|| {
            ParseError::Rejected(format!("{raw} is not a git status: {}", git_statuses()))
        })
}

/// The `--git-status` vocabulary as one line, for the usage and for a rejection.
fn git_statuses() -> String {
    let mut words = vec!["none"];
    words.extend(ALL_GIT_STATUSES.iter().map(|status| status.name()));
    words.join(", ")
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
        "crowbar-app — one measurable surface in one §8.3 matrix cell.\n\n\
         Options (all optional; the defaults are one cell):\n",
    );
    // Built rather than written out, so the usage cannot claim a vocabulary the
    // parser does not accept.
    let statuses = format!("file-tree-row: colours the name; {} [none]", git_statuses());
    let surfaces = format!("one of {} [git-status-row]", Surface::names().join(", "));
    for (option, description) in [
        ("--surface <name>", surfaces.as_str()),
        ("--width <px>", "surface width, logical px [320]"),
        (
            "--viewport-width <px>",
            "window width; selects the sm: breakpoint and is the snapshot's \
             state.width [800]",
        ),
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
        (
            "--added <n>",
            "git-status-row: diffStats.additions; 0 omits the span [12]",
        ),
        (
            "--deleted <n>",
            "git-status-row: diffStats.deletions; 0 omits the span [3]",
        ),
        ("--git-status <name>", statuses.as_str()),
        ("--no-directory", "git-status-row: showDirectory = false"),
        ("--no-icon", "git-status-row: showFileIcon = false"),
        ("--compact", "git-status-row: compactGitStatusBadges = true"),
    ] {
        let _ = writeln!(out, "  {option:<32}{description}");
    }

    // Each surface's own options, from the surface itself. Nothing here knows
    // what any of them are, which is the point: adding a surface adds a section.
    for surface in crate::surfaces::ALL {
        let options = (surface.options)();
        if options.is_empty() {
            continue;
        }
        let _ = write!(out, "\n{} only:\n", surface.name);
        for (option, description) in options {
            let _ = writeln!(out, "  {option:<32}{description}");
        }
    }

    out.push_str(
        "\nWhich flags each surface really has (the rest render the resting\n\
         surface and are reported on stderr, because a cell that cannot fail is\n\
         the cheapest possible fake convergence):\n",
    );
    for surface in crate::surfaces::ALL {
        let modelled: Vec<&str> = ALL_FLAGS
            .iter()
            .filter(|flag| !surface.unmodelled(**flag))
            .map(|flag| flag.name())
            .collect();
        let _ = writeln!(out, "  {:<32}{}", surface.name, modelled.join(", "));
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

    /// The window: **the viewport**, and tall enough for the surface and its
    /// caption.
    ///
    /// The width is `--viewport-width` verbatim rather than "whatever the
    /// surface needs", because the window *is* the viewport a `sm:` variant
    /// resolves against — a window sized to the surface would render the wide
    /// badge in a run that asked for a narrow viewport. [`Cell::parse`] has
    /// already refused a viewport too small to hold the surface, so this can
    /// never be narrower than `surface + 2 × INSET_X`.
    ///
    /// The height is the surface's own, because a row is one line and a menu
    /// popup is a column of them. The two Phase 1 surfaces keep the 72 they were
    /// measured at.
    #[must_use]
    pub fn window_size(cell: &Cell) -> Size<Pixels> {
        size(
            cell.viewport_width_px(),
            px(INSET_Y * 2.0 + f32::from(cell.surface.window_height)),
        )
    }
}

/// The one place `--surface` becomes an element tree — and it no longer names a
/// surface.
///
/// A free function over the cell rather than a method on [`RowSurface`], because
/// `row_layout.rs` renders the same dispatch under `cargo test` without a
/// `RowSurface` — and two spellings of "which surface does this cell mean" is
/// exactly the duplication that lets the measured tree drift from the drawn one.
#[must_use]
pub fn render_row(cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
    cell.params.render(cell, theme, anchors)
}

impl Render for RowSurface {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = self.cell.theme();

        div()
            .size_full()
            .bg(theme.background)
            .pl(px(INSET_X))
            .pt(px(INSET_Y))
            .font_family(theme.font_sans.primary().unwrap_or("sans-serif"))
            .child(div().w(self.cell.width_px()).child(render_row(
                &self.cell,
                &theme,
                self.anchors.as_ref(),
            )))
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
    use super::{
        ALL_FLAGS, Cell, DEFAULT_VIEWPORT_WIDTH, INSET_X, INSET_X_WHOLE, ParseError, RowSurface,
        StateFlag, Surface, usage,
    };
    use crowbar_ui::Appearance;
    use crowbar_ui::components::{
        ALL_GIT_STATUSES, BREAKPOINT_SM, Breakpoint, ContentLength, GitStatus,
    };

    fn parse(args: &[&str]) -> Result<Cell, ParseError> {
        Cell::parse(args.iter().map(|arg| (*arg).to_owned()))
    }

    /// A registered surface by name. Every one of these names is asserted to
    /// exist by `surface.rs`, so a lookup that misses here is a registry
    /// failure and not a typo in this file.
    fn a_surface(name: &str) -> &'static Surface {
        Surface::parse(name).expect("a registered surface")
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
            "--viewport-width",
            "600",
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
        assert_eq!(cell.viewport_width, 600);
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
        // Defaulted, because this line does not pass it.
        assert_eq!(cell.git_status, None);
    }

    /// **`--git-status` drives the colour**, one word per status, and `none` is
    /// the default rather than a rejection.
    #[test]
    fn every_git_status_parses_and_none_is_the_absence() {
        assert_eq!(Cell::default().git_status, None);
        assert_eq!(parse(&[]).expect("well-formed").git_status, None);
        assert_eq!(
            parse(&["--git-status", "none"])
                .expect("well-formed")
                .git_status,
            None,
        );

        for status in ALL_GIT_STATUSES {
            let cell = parse(&["--git-status", status.name()]).expect("well-formed");
            assert_eq!(cell.git_status, Some(status), "{}", status.name());
            // And it reaches the row, which is where the colour is read.
            assert_eq!(cell.file_row().git_status, Some(status));
        }

        // The one the defect was found on.
        assert_eq!(
            parse(&["--git-status", "modified"])
                .expect("well-formed")
                .file_row()
                .git_status,
            Some(GitStatus::Modified),
        );
    }

    /// The vocabulary is closed and the complaint lists it, because "invalid" is
    /// not something anyone can act on — and `staged` is the near-miss worth
    /// naming: it is not a status in the React source, it is `modified` with a
    /// boolean, and its word here is `modified-staged`.
    #[test]
    fn the_git_status_vocabulary_is_closed() {
        for line in [
            vec!["--git-status", "staged"],
            vec!["--git-status", "conflicted"],
            vec!["--git-status", "ignored"],
            vec!["--git-status", "Modified"],
            vec!["--git-status", ""],
            vec!["--git-status"],
        ] {
            assert!(
                matches!(parse(&line), Err(ParseError::Rejected(_))),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = parse(&["--git-status", "staged"]) else {
            panic!("`staged` is not a status");
        };
        assert!(complaint.contains("modified-staged"), "{complaint}");
        assert!(complaint.contains("none"), "{complaint}");
    }

    /// It is named in the caption on the surface that honours it, and nowhere
    /// else: a git status row announcing `git modified` would be describing a
    /// colour nothing painted, because `GitFileItem` pins its filename at
    /// `text-foreground`.
    #[test]
    fn the_caption_names_the_status_only_on_the_surface_that_paints_it() {
        let file =
            parse(&["--surface", "file-tree-row", "--git-status", "deleted"]).expect("well-formed");
        assert!(
            file.describe().contains("git deleted"),
            "{}",
            file.describe()
        );

        let git = parse(&["--git-status", "deleted"]).expect("well-formed");
        assert!(
            !git.describe().contains("git deleted"),
            "{}",
            git.describe()
        );

        // And no status says nothing at all, so the default caption is unchanged.
        let plain = parse(&["--surface", "file-tree-row"]).expect("well-formed");
        assert!(
            !plain.describe().contains(" · git "),
            "{}",
            plain.describe()
        );
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
            vec!["--viewport-width"],
            vec!["--viewport-width", "wide"],
            vec!["--viewport-width", "0"],
            // Narrower than the surface plus its insets: the row would be cut
            // at the window edge and every `clipped` would be an artefact.
            vec!["--width", "294", "--viewport-width", "341"],
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
            "--surface",
            "git-status-row",
            "file-tree-row",
            "--width",
            "--viewport-width",
            "--theme",
            "--content",
            "--flags",
            "--depth",
            "--prev-depth",
            "--next-depth",
            "--added",
            "--deleted",
            "--git-status",
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
        // Every word `--git-status` accepts, including the absence.
        assert!(usage.contains("none"), "{usage}");
        for status in ALL_GIT_STATUSES {
            assert!(
                usage.contains(status.name()),
                "{} is missing from the usage",
                status.name(),
            );
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
            assert!(
                !a_surface("git-status-row").unmodelled(flag),
                "{} has an original on the git row",
                flag.name(),
            );
        }
    }

    /// **Which flags each surface really has**, stated as data because the
    /// difference between the two is the reason the second surface exists.
    ///
    /// Neither original has a `loading` or an `error` rendering of the row —
    /// that is a gap in the thing being ported, not in the port. `empty` is the
    /// one that differs: the git row's trailing badge and counts are optional,
    /// where a file explorer row always paints an icon and a name and has no
    /// content to remove. Driving an unmodelled cell renders the resting row on
    /// both sides, so it compares equal and proves nothing — which is why the
    /// binary says so on stderr rather than rendering it quietly.
    #[test]
    fn the_two_surfaces_model_different_flags() {
        let modelled = |name: &str| {
            let surface = a_surface(name);
            ALL_FLAGS
                .iter()
                .copied()
                .filter(|flag| !surface.unmodelled(*flag))
                .collect::<Vec<_>>()
        };

        assert_eq!(
            modelled("git-status-row"),
            vec![
                StateFlag::Empty,
                StateFlag::Hover,
                StateFlag::Focus,
                StateFlag::Selected,
            ],
        );
        assert_eq!(
            modelled("file-tree-row"),
            vec![StateFlag::Hover, StateFlag::Focus, StateFlag::Selected],
        );

        // The two the *contract* has and no app does — on every surface, which
        // is the claim the registry makes it possible to state once.
        for surface in crate::surfaces::ALL {
            assert!(surface.unmodelled(StateFlag::Loading), "{}", surface.name);
            assert!(surface.unmodelled(StateFlag::Error), "{}", surface.name);
        }

        // And the filter follows the cell's surface, not a constant.
        let git = parse(&["--flags", "empty"]).expect("well-formed");
        let file = parse(&["--surface", "file-tree-row", "--flags", "empty"]).expect("well-formed");
        assert_eq!(git.unmodelled_flags(), vec![]);
        assert_eq!(file.unmodelled_flags(), vec![StateFlag::Empty]);
    }

    /// The selector, its default, and the two facts a snapshot carries off it.
    ///
    /// The default has to stay `git-status-row`: every invocation written before
    /// `--surface` existed must keep rendering the row it rendered, and the
    /// archived gate runs are evidence taken at that default.
    #[test]
    fn the_surface_selector_defaults_to_the_geometry_gate() {
        assert_eq!(Cell::default().surface.name, "git-status-row");
        assert_eq!(
            parse(&[]).expect("well-formed").surface.name,
            "git-status-row"
        );
        for name in Surface::names() {
            assert_eq!(
                parse(&["--surface", name])
                    .expect("well-formed")
                    .surface
                    .name,
                name,
            );
        }

        // The vocabulary is closed, and the complaint names what was wanted.
        let Err(ParseError::Rejected(complaint)) = parse(&["--surface", "file-row"]) else {
            panic!("`file-row` is not a surface");
        };
        assert!(complaint.contains("file-tree-row"), "{complaint}");
        assert!(matches!(
            parse(&["--surface"]),
            Err(ParseError::Rejected(_))
        ));
    }

    /// The state axis reaches the file explorer row, which is the whole point of
    /// the second surface — and the fixture strings are shared with the first,
    /// so the same `--content` means the same name on both.
    #[test]
    fn the_file_tree_cell_carries_the_state_and_shares_the_fixture() {
        let cell = parse(&[
            "--surface",
            "file-tree-row",
            "--content",
            "overflow",
            "--flags",
            "selected",
            "--depth",
            "3",
        ])
        .expect("well-formed");
        let row = cell.file_row();

        assert!(row.state.selected);
        assert!(!row.state.hovered);
        assert_eq!(row.depth, 3);
        assert_eq!(row.previous_depth, 3);
        assert_eq!(row.next_depth, 3);
        assert_eq!(row.name, cell.row().parts().0);
    }

    /// The caption names the surface, because a run's stderr is the only record
    /// of which of the two produced a snapshot.
    #[test]
    fn the_caption_names_the_surface() {
        assert!(
            parse(&["--surface", "file-tree-row"])
                .expect("well-formed")
                .describe()
                .contains("file-tree-row"),
        );
        assert!(
            parse(&[])
                .expect("well-formed")
                .describe()
                .contains("git-status-row")
        );

        // The three git-only row parameters are not announced on a surface that
        // has no prop for them.
        let file = parse(&["--surface", "file-tree-row", "--no-directory", "--no-icon"])
            .expect("well-formed");
        assert!(
            !file.describe().contains("no-directory"),
            "{}",
            file.describe()
        );
        assert!(!file.describe().contains("no-icon"), "{}", file.describe());
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
        assert!(description.contains("800px viewport"), "{description}");
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

    /// The window **is** the viewport — that is the whole point of the option,
    /// because a `sm:` variant resolves against the window and not against the
    /// surface. It is still always wide enough to hold the surface and its
    /// insets, which `Cell::parse` guarantees rather than `window_size`
    /// clamping after the fact.
    #[test]
    fn the_window_is_the_viewport_and_still_holds_the_surface() {
        let cell = parse(&["--width", "420", "--viewport-width", "900"]).expect("well-formed");
        let window = RowSurface::window_size(&cell);

        assert_eq!(window.width, cell.viewport_width_px());
        assert_eq!(window.width, gpui::px(900.0));
        assert!(window.width >= cell.width_px() + gpui::px(INSET_X * 2.0));
        assert!(window.height > gpui::px(24.0));

        // At the tightest viewport the parser accepts, the surface exactly
        // fills the window's insets and nothing is cut.
        let tight = parse(&["--width", "294", "--viewport-width", "342"]).expect("well-formed");
        assert_eq!(
            RowSurface::window_size(&tight).width,
            tight.width_px() + gpui::px(INSET_X * 2.0),
        );
        assert_eq!(tight.minimum_viewport(), 342);
    }

    /// The two spellings of the horizontal inset — `f32` for gpui, `u16` for
    /// the command line's width arithmetic — are one number.
    #[test]
    fn the_two_spellings_of_the_inset_agree() {
        assert!((f32::from(INSET_X_WHOLE) - INSET_X).abs() < f32::EPSILON);
    }

    /// **The surface and the viewport are independent**, which is what the
    /// matrix needs: hold the sidebar at the reference's 294px and move the
    /// window across the breakpoint.
    #[test]
    fn the_surface_and_the_viewport_move_independently() {
        let narrow = parse(&["--width", "294", "--viewport-width", "600"]).expect("well-formed");
        let wide = parse(&["--width", "294", "--viewport-width", "800"]).expect("well-formed");

        assert_eq!(narrow.width, wide.width);
        assert_ne!(narrow.viewport_width, wide.viewport_width);
        assert_eq!(narrow.breakpoint(), Breakpoint::Base);
        assert_eq!(wide.breakpoint(), Breakpoint::Sm);
        // And the breakpoint reaches the row, which is where the badge reads it.
        assert_eq!(narrow.row().breakpoint, Breakpoint::Base);
        assert_eq!(wide.row().breakpoint, Breakpoint::Sm);

        // The surface alone never moves it: a 294px row is the wide variant in
        // an 800px window, exactly as the React original is.
        for width in ["240", "294", "420"] {
            let cell = parse(&["--width", width]).expect("well-formed");
            assert_eq!(cell.breakpoint(), Breakpoint::Sm, "surface {width}");
        }
    }

    /// The default clears the `sm` breakpoint, so introducing the option did
    /// not silently change what every existing invocation renders.
    #[test]
    fn the_default_viewport_is_above_the_breakpoint() {
        assert!(f32::from(DEFAULT_VIEWPORT_WIDTH) >= BREAKPOINT_SM);
        assert_eq!(Cell::default().viewport_width, DEFAULT_VIEWPORT_WIDTH);
        assert_eq!(Cell::default().breakpoint(), Breakpoint::Sm);
        assert!(Cell::default().viewport_width >= Cell::default().minimum_viewport());
    }

    /// A rejected viewport says which two numbers disagreed and what to pass,
    /// because "invalid" is not something anyone can act on.
    #[test]
    fn a_viewport_too_small_for_the_surface_names_the_number_it_needs() {
        let Err(ParseError::Rejected(complaint)) =
            parse(&["--width", "294", "--viewport-width", "300"])
        else {
            panic!("a 300px viewport cannot hold a 294px surface plus 48px of insets");
        };
        assert!(complaint.contains("300"), "{complaint}");
        assert!(complaint.contains("294"), "{complaint}");
        assert!(complaint.contains("342"), "{complaint}");
        assert!(complaint.contains("clipped"), "{complaint}");
    }
}
