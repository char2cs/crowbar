//! `--surface resizable` — the second Phase 2 surface.
//!
//! A `ResizablePanelGroup` with its panels and the separator between them, drawn
//! in one cell of the §8.3 matrix. The fixture is the IDE shell's own group from
//! `web/src/components/layout/ide-shell.tsx`, which is the **only**
//! `ResizablePanelGroup` in the app.
//!
//! # What this surface measures, and what it cannot
//!
//! It is a **geometry-only** surface. Every anchor reports `bg: #00000000`, no
//! text, and — unless `--with-handle` renders the grip — no radius and no border
//! either. That is what the component paints; it is not a gap in the port. Said
//! per axis, because three of the four are vacuous here and a reader who assumes
//! otherwise will bank cells that cannot fail:
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` / `--viewport-width` | **real, and the point.** `flex-basis: 0` with fractional `flex-grow` around a fixed 1px sibling is one division, and taffy and `WebKit` each round it their own way |
//! | `--theme` | vacuous **without `--with-handle`**: `bg-border` on the grip is the only colour anywhere on the component |
//! | `--content` | vacuous always. Nothing here paints a character |
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `hover` | a **real** state with **no anchor**. `hover:after:bg-border/60` is the only hover rule and it paints the separator's `::after`; `ANCHORS.md` §3 permits a pseudo-backed anchor only while the pseudo is `position: absolute; inset: 0`, and this one is `left: 50%` with a translate. The strip is painted and the cell cannot fail |
//! | `focus` | a **real** state with **no field**. `focus-visible:ring-1` is a box-shadow and §6 has no field for shadows; `focus-visible:outline-hidden` is an outline, which it has no field for either |
//! | `selected` | **unmodelled.** A panel group has no selected state |
//! | `empty` | **unmodelled.** A group with no panels is not a state the app has — `ide-shell.tsx` renders three children unconditionally |
//! | `loading`, `error` | unmodelled, as on every surface so far |
//!
//! `hover` and `focus` are **not** declared unmodelled, and that is deliberate:
//! `Surface::unmodelled` means "this surface's React original has no such state",
//! and both of these originals exist. What is missing is a *field in the
//! contract*, which is a different fact and belongs in the per-cell caption where
//! [`Params::describe`] puts it.
//!
//! # Three options that are not matrix axes
//!
//! `--grow`, `--shell-height` and `--orientation` are all runtime inputs rather
//! than component properties: the growth factors are what
//! `react-resizable-panels`' layout engine resolved, the shell height is the
//! `h-screen` box the group's own `height: 100%` measures against, and the
//! orientation is a prop only one call site sets — to `horizontal`.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::resizable::{
    DEFAULT_SHELL_HEIGHT, GroupEntry, Orientation, ResizablePanelGroup,
};
use crowbar_ui::components::{AnchorSink, resizable};
use gpui::{AnyElement, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "resizable",
    root: resizable::ID_GROUP,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Selected,
    ],
    // The shell box plus the caption below it. `--shell-height` is refused above
    // `MAX_SHELL_HEIGHT` rather than clamped, so this is a bound and not a hope.
    window_height: 200,
    options,
    params: || Box::new(Params::default()),
};

/// The tallest `--shell-height` this surface will draw.
///
/// A group taller than the window would be cut at the window edge, and the
/// driver reports a box cut by its clip as no longer fully visible — so every
/// `visible` in the snapshot would be an artefact of the window size rather than
/// a fact about the layout. The same reasoning `Cell::parse` applies to
/// `--viewport-width`, and the same answer: refuse, and name the number.
pub const MAX_SHELL_HEIGHT: u16 = 160;

/// How many panels the fixture has, and therefore how many values `--grow`
/// takes.
///
/// Fixed at the reference's own count rather than made variable: `ide-shell.tsx`
/// renders exactly two panels and one separator, so a three-panel group is a
/// picture the reference cannot produce and a cell drawn from it could only ever
/// compare the native side against itself.
pub const PANEL_COUNT: usize = 2;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--orientation`.
    pub orientation: Orientation,
    /// `--grow`: the panels' `flex-grow` values, in DOM order.
    ///
    /// Always [`PANEL_COUNT`] long — the parser refuses any other count, because
    /// a list that did not match the fixture would silently leave a panel on its
    /// default while the caption claimed otherwise.
    pub grow: Vec<f32>,
    /// `--shell-height`: the definite-height box the group's `height: 100%`
    /// resolves against.
    pub shell_height: u16,
    /// `--with-handle`: render the separator's grip.
    pub with_handle: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            orientation: Orientation::Horizontal,
            grow: vec![resizable::SIDEBAR_GROW, resizable::CONTENT_GROW],
            shell_height: shell_height_default(),
            with_handle: false,
        }
    }
}

/// [`DEFAULT_SHELL_HEIGHT`] as the whole number of pixels the command line works
/// in.
///
/// Derived from the component's own constant rather than restated, so the two
/// cannot drift into different defaults; `the_default_shell_height_is_the_components`
/// pins the round trip.
fn shell_height_default() -> u16 {
    // Saturating rather than `as`: the constant is 160.0 and a cast that wrapped
    // would be a silent zero-height surface.
    let raw = f32::from(DEFAULT_SHELL_HEIGHT);
    if raw <= 0.0 {
        0
    } else if raw >= f32::from(u16::MAX) {
        u16::MAX
    } else {
        // `raw` is finite, non-negative and below `u16::MAX` here.
        raw as u16
    }
}

impl Params {
    /// The group this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling entries from scratch — so that a bare `--surface resizable`
    /// renders the group the reference actually has, with the panel ids
    /// `ide-shell.tsx` writes.
    #[must_use]
    pub fn group(&self, cell: &Cell) -> ResizablePanelGroup {
        let mut group = ResizablePanelGroup::fixture();
        group.orientation = self.orientation;
        group.shell_height = px(f32::from(self.shell_height));

        // `hover` and `focus` are two different rules here, unlike the menu's —
        // but neither of them is visible to the differ. See the module docs.
        let hovered = cell.has(StateFlag::Hover);
        let focused = cell.has(StateFlag::Focus);

        let mut panel = 0;
        for entry in &mut group.entries {
            match entry {
                GroupEntry::Panel(target) => {
                    if let Some(grow) = self.grow.get(panel) {
                        target.grow = *grow;
                    }
                    panel += 1;
                }
                GroupEntry::Handle(handle) => {
                    handle.with_handle = self.with_handle;
                    handle.state.hovered = hovered;
                    handle.state.focused = focused;
                }
            }
        }
        group
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--orientation" => self.orientation = parse_orientation(&value(args, option)?)?,
            "--grow" => self.grow = parse_grow(&value(args, option)?)?,
            "--shell-height" => {
                let height = pixels(&value(args, option)?, option)?;
                if height == 0 || height > MAX_SHELL_HEIGHT {
                    return Err(ParseError::Rejected(format!(
                        "--shell-height {height} is outside 1..={MAX_SHELL_HEIGHT}; the group \
                         would be cut at the window edge and every `visible` in the snapshot \
                         would be an artefact of the window size",
                    )));
                }
                self.shell_height = height;
            }
            "--with-handle" => self.with_handle = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// The three inputs that decide the picture, and — where it applies — the
    /// fact that an axis was moved that this surface cannot paint.
    ///
    /// Every one of the notes below is a cell that **cannot fail**. They are said
    /// here rather than through `Surface::unmodelled` because that field is per
    /// surface and answers a different question: whether the original has the
    /// state at all. These originals do; the contract has no field for them.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let grow: Vec<String> = self.grow.iter().map(|value| format!("{value}")).collect();
        let _ = write!(
            out,
            " · {} · grow {} · shell {}px",
            self.orientation.name(),
            grow.join("/"),
            self.shell_height,
        );
        if self.with_handle {
            out.push_str(" · with handle");
        }

        // Nothing on this surface paints a character, so the content axis moves
        // three identical pictures. Said on every cell, because it is true on
        // every cell and a reader comparing three of them deserves to know they
        // are the same one.
        out.push_str(" · no text on this surface, so --content cannot fail");
        if !self.with_handle {
            out.push_str(" · no colour without --with-handle, so --theme cannot fail");
        }
        if cell.has(StateFlag::Hover) {
            out.push_str(
                " · hover: the only hover rule paints the separator's ::after, which \
                 ANCHORS.md §3 forbids anchoring here, so this cell cannot fail",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: the ring is a box-shadow, which ANCHORS.md §6 has no field \
                 for, so this cell cannot fail",
            );
        }
        if self.orientation == Orientation::Vertical {
            out.push_str(" · vertical: no live call site asks for it, so there is no reference");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.group(cell).render(theme, anchors)
    }
}

fn parse_orientation(raw: &str) -> Result<Orientation, ParseError> {
    match raw {
        "horizontal" => Ok(Orientation::Horizontal),
        "vertical" => Ok(Orientation::Vertical),
        other => Err(ParseError::Rejected(format!(
            "--orientation takes horizontal or vertical, not {other}"
        ))),
    }
}

/// The panels' growth factors: exactly [`PANEL_COUNT`] finite, non-negative
/// numbers.
///
/// The count is checked rather than zipped against whatever arrived, because a
/// short list would leave the trailing panel on the fixture's factor while the
/// caption printed the list that was asked for — a picture that matches neither
/// side.
fn parse_grow(raw: &str) -> Result<Vec<f32>, ParseError> {
    let mut out = Vec::new();
    for word in raw.split(',') {
        let parsed: f32 = word.trim().parse().map_err(|_| {
            ParseError::Rejected(format!("--grow takes {PANEL_COUNT} numbers, not {raw}"))
        })?;
        if !parsed.is_finite() || parsed < 0.0 {
            return Err(ParseError::Rejected(format!(
                "--grow takes finite non-negative numbers, not {word}"
            )));
        }
        out.push(parsed);
    }
    if out.len() != PANEL_COUNT {
        return Err(ParseError::Rejected(format!(
            "--grow takes {PANEL_COUNT} comma-separated numbers, one per panel; got {} in {raw}",
            out.len(),
        )));
    }
    if out.iter().all(|value| *value <= 0.0) {
        return Err(ParseError::Rejected(format!(
            "--grow needs at least one positive factor, or every panel collapses to the \
             zero flex-basis and the group has no layout to compare: {raw}",
        )));
    }
    Ok(out)
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--orientation horizontal|vertical".to_owned(),
            "ResizablePanelGroup orientation; only horizontal has a reference [horizontal]"
                .to_owned(),
        ),
        (
            "--grow <a,b>".to_owned(),
            format!(
                "the panels' resolved flex-grow, one per panel [{}/{}]",
                resizable::SIDEBAR_GROW,
                resizable::CONTENT_GROW,
            ),
        ),
        (
            "--shell-height <px>".to_owned(),
            format!(
                "the box the group's height:100% resolves against, 1..={MAX_SHELL_HEIGHT} [{}]",
                shell_height_default(),
            ),
        ),
        (
            "--with-handle".to_owned(),
            "render the separator's grip; the only colour on this surface [off]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{MAX_SHELL_HEIGHT, PANEL_COUNT, Params, SURFACE, options, shell_height_default};
    use crate::row_surface::{Cell, ParseError};
    use crowbar_ui::components::resizable::{
        CONTENT_GROW, DEFAULT_SHELL_HEIGHT, GroupEntry, Handle, Orientation, Panel, SIDEBAR_GROW,
    };
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "resizable"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// The bag the cell carries, downcast back to this surface's own type.
    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a resizable cell carries this surface's bag")
    }

    /// The panels the cell's group ends up with, in DOM order.
    fn panels(cell: &Cell) -> Vec<Panel> {
        params_of(cell)
            .group(cell)
            .entries
            .into_iter()
            .filter_map(|entry| match entry {
                GroupEntry::Panel(panel) => Some(panel),
                GroupEntry::Handle(_) => None,
            })
            .collect()
    }

    /// The one separator.
    fn handle(cell: &Cell) -> Handle {
        params_of(cell)
            .group(cell)
            .entries
            .into_iter()
            .find_map(|entry| match entry {
                GroupEntry::Handle(handle) => Some(handle),
                GroupEntry::Panel(_) => None,
            })
            .expect("the fixture has one separator")
    }

    #[test]
    fn the_defaults_are_the_live_ide_shells() {
        let bag = Params::default();

        assert_eq!(bag.orientation, Orientation::Horizontal);
        assert_eq!(bag.grow, vec![SIDEBAR_GROW, CONTENT_GROW]);
        assert_eq!(bag.shell_height, shell_height_default());
        assert!(!bag.with_handle);

        let group = bag.group(&cell(&[]));
        assert_eq!(group.entries.len(), PANEL_COUNT + 1);
        assert_eq!(group.shell_height, DEFAULT_SHELL_HEIGHT);
        assert_eq!(group.orientation, Orientation::Horizontal);
    }

    /// The default is derived from the component's constant, not restated, so the
    /// two cannot drift into different pictures.
    #[test]
    fn the_default_shell_height_is_the_components() {
        assert_eq!(px(f32::from(shell_height_default())), DEFAULT_SHELL_HEIGHT);
        assert!(shell_height_default() <= MAX_SHELL_HEIGHT);
    }

    /// `--grow` reaches the panels **in DOM order**, and leaves everything else
    /// alone — the ids especially, because they are what the differ matches on.
    #[test]
    fn the_growth_factors_reach_the_panels_in_order() {
        let driven = panels(&cell(&["--grow", "30,70"]));

        assert!((driven[0].grow - 30.0).abs() < f32::EPSILON);
        assert!((driven[1].grow - 70.0).abs() < f32::EPSILON);
        assert_eq!(driven[0].id, "resize-panel-sidebar");
        assert_eq!(driven[1].id, "resize-panel-content");

        // And the default cell keeps the fixture's own pair.
        let resting = panels(&cell(&[]));
        assert!((resting[0].grow - SIDEBAR_GROW).abs() < f32::EPSILON);
        assert!((resting[1].grow - CONTENT_GROW).abs() < f32::EPSILON);
    }

    /// The two interaction flags reach the separator and **nothing else** — there
    /// is no panel state on this component at all.
    #[test]
    fn the_interaction_flags_reach_only_the_separator() {
        assert_eq!(handle(&cell(&[])).state.hovered, false);
        assert_eq!(handle(&cell(&[])).state.focused, false);

        let hovered = handle(&cell(&["--flags", "hover"]));
        assert!(hovered.state.hovered);
        assert!(!hovered.state.focused);

        let focused = handle(&cell(&["--flags", "focus"]));
        assert!(focused.state.focused);
        assert!(!focused.state.hovered);

        // Unlike the menu's, these two are **different** rules, so a cell
        // carrying both is a third picture rather than a repeat of either.
        let both = handle(&cell(&["--flags", "hover,focus"]));
        assert!(both.state.hovered && both.state.focused);

        // `selected` is declared unmodelled and reaches nothing.
        let selected = handle(&cell(&["--flags", "selected"]));
        assert!(!selected.state.hovered && !selected.state.focused);
        assert!(SURFACE.unmodelled(crate::row_surface::StateFlag::Selected));
    }

    /// `--with-handle` is the only thing on this surface that paints a colour, so
    /// it is the only thing that makes the theme axis real.
    #[test]
    fn the_grip_is_off_by_default_and_is_the_only_paint() {
        assert!(!handle(&cell(&[])).with_handle);
        assert!(handle(&cell(&["--with-handle"])).with_handle);

        // And the caption says so, per cell, on both sides of the switch.
        let plain = cell(&[]).describe();
        assert!(plain.contains("--theme cannot fail"), "{plain}");
        let gripped = cell(&["--with-handle"]).describe();
        assert!(!gripped.contains("--theme cannot fail"), "{gripped}");
        assert!(gripped.contains("with handle"), "{gripped}");
    }

    /// **The two cells that cannot fail say so.** Both flags have real originals,
    /// so `Surface::unmodelled` is the wrong place for them — the caption is.
    #[test]
    fn the_two_invisible_states_are_named_in_the_caption() {
        let hover = cell(&["--flags", "hover"]).describe();
        assert!(hover.contains("::after"), "{hover}");
        assert!(hover.contains("cannot fail"), "{hover}");

        let focus = cell(&["--flags", "focus"]).describe();
        assert!(focus.contains("box-shadow"), "{focus}");
        assert!(focus.contains("cannot fail"), "{focus}");

        // And they are *not* declared unmodelled, because the originals exist.
        assert!(!SURFACE.unmodelled(crate::row_surface::StateFlag::Hover));
        assert!(!SURFACE.unmodelled(crate::row_surface::StateFlag::Focus));

        // The content axis is vacuous on every cell, so it is said on every cell.
        for content in ["short", "normal", "overflow"] {
            let described = cell(&["--content", content]).describe();
            assert!(described.contains("--content cannot fail"), "{described}");
        }
    }

    /// The vertical group has no reference anywhere in the app, and the caption
    /// says so rather than rendering a cell that can only compare the native side
    /// against itself.
    #[test]
    fn the_vertical_orientation_is_named_as_unreachable() {
        let vertical = cell(&["--orientation", "vertical"]);
        assert_eq!(params_of(&vertical).orientation, Orientation::Vertical);
        assert_eq!(
            params_of(&vertical).group(&vertical).orientation,
            Orientation::Vertical,
        );
        assert!(
            vertical.describe().contains("no reference"),
            "{}",
            vertical.describe()
        );

        assert!(!cell(&[]).describe().contains("no reference"));
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--orientation", "sideways"],
            vec!["--orientation"],
            vec!["--grow", "30"],
            vec!["--grow", "30,40,30"],
            vec!["--grow", "wide,narrow"],
            vec!["--grow", "-1,2"],
            vec!["--grow", "0,0"],
            vec!["--grow"],
            vec!["--shell-height", "0"],
            vec!["--shell-height", "4000"],
            vec!["--shell-height", "tall"],
            vec!["--shell-height"],
        ] {
            let mut full = vec!["--surface", "resizable"];
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
            ["--surface", "resizable", "--grow", "30"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("one factor is not a two-panel group");
        };
        assert!(complaint.contains("one per panel"), "{complaint}");

        // The refusal that protects the snapshot, not the parser: it names the
        // bound and says what a bigger window would cost.
        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "resizable", "--shell-height", "400"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("400px is taller than the window");
        };
        assert!(complaint.contains("160"), "{complaint}");
        assert!(complaint.contains("artefact"), "{complaint}");
    }

    /// **These options belong to this surface and to no other.** Driving one on
    /// another surface is a rejection, which is the property the registry exists
    /// for: a surface's vocabulary does not leak into the shared parser.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--orientation", "--grow", "--shell-height", "--with-handle"] {
            let line = ["--surface", "dropdown-menu", option, "horizontal"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a dropdown-menu option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage, or driving the
    /// matrix means reading the source.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in ["--orientation", "--grow", "--shell-height", "--with-handle"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("resizable"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "resizable");
        assert_eq!(SURFACE.root, "resize-group");
        assert!(SURFACE.window_height >= MAX_SHELL_HEIGHT);
    }
}
