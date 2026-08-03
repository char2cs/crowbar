// The emit path itself needs the `driver` feature to be reachable; `state_of`
// does not, and is exercised under a plain `cargo test`. See the `mod`
// declaration in `main.rs` for why the module compiles either way.
#![cfg_attr(not(feature = "driver"), allow(dead_code))]

//! The snapshot a driver build emits, and where it goes.
//!
//! `--features driver` only. The sink the row is rendered through is
//! [`crate::driver_anchors`], which a plain `cargo test` also uses; the shipping
//! build carries `Unanchored`, whose elements request layout identically, so
//! what the oracle measures is what ships.

use std::env;
use std::fs;
use std::io::{self, Write as _};
use std::path::PathBuf;

use crowbar_driver::{
    AnchorRegistry, Content, Flag, RawAnchor, Snapshot, SurfaceState, Theme as SnapshotTheme,
};
use crowbar_ui::Appearance;
use crowbar_ui::components::ContentLength;
use gpui::{Pixels, Size, px};

use crate::driver_anchors::fold_text_halves;
use crate::row_surface::{Cell, INSET_Y, RowSurface, StateFlag};

/// Where an emitted snapshot goes.
pub enum Destination {
    /// Write to this path.
    File(PathBuf),
    /// Write to stdout. Requested with `-`.
    Stdout,
}

impl Destination {
    /// Reads the request out of `CROWBAR_ROW_SNAPSHOT`, if it is set.
    ///
    /// A driver build without it is the ordinary app with the driver linked,
    /// which is the configuration the oracle measures, so it has to stay
    /// reachable.
    #[must_use]
    pub fn from_env() -> Option<Self> {
        let raw = env::var("CROWBAR_ROW_SNAPSHOT").ok()?;
        Some(if raw == "-" {
            Self::Stdout
        } else {
            Self::File(PathBuf::from(raw))
        })
    }
}

/// The matrix cell, in the snapshot's own vocabulary.
///
/// A second spelling of the same cell, which is exactly the kind of duplication
/// that drifts — so it is one function over the parsed [`Cell`] and nothing
/// reads the command line twice.
///
/// # `state.width` is the **viewport**, not the surface
///
/// `state` is the §8.3 matrix cell, and the differ *refuses* a comparison whose
/// cells differ — so whatever goes in here has to be the quantity that decides
/// what the two apps render. That is the viewport: it is what a CSS media query
/// asks about, and the badge's `sm:` variant follows it. The surface width is a
/// row parameter, and two runs at the same viewport with different surface
/// widths are legitimately different rows in the same cell.
///
/// Getting this backwards is not hypothetical. A reference taken in a 569px
/// webview and a native run drawing the ≥640px badge agreed on `state.width`,
/// compared happily, and produced four geometry deltas belonging to neither
/// side. Reporting the viewport is what makes that a refusal instead.
#[must_use]
pub fn state_of(cell: &Cell) -> SurfaceState {
    SurfaceState::new(
        u32::from(cell.viewport_width),
        match cell.appearance {
            Appearance::Light => SnapshotTheme::Light,
            Appearance::Dark => SnapshotTheme::Dark,
        },
        match cell.content {
            ContentLength::Short => Content::Short,
            ContentLength::Normal => Content::Normal,
            ContentLength::Overflow => Content::Overflow,
        },
        cell.flags.iter().copied().map(flag_of),
    )
}

fn flag_of(flag: StateFlag) -> Flag {
    match flag {
        StateFlag::Empty => Flag::Empty,
        StateFlag::Loading => Flag::Loading,
        StateFlag::Error => Flag::Error,
        StateFlag::Hover => Flag::Hover,
        StateFlag::Focus => Flag::Focus,
        StateFlag::Selected => Flag::Selected,
    }
}

/// How far past the window's edge an anchor may reach before the frame is
/// refused, in logical px.
///
/// Not zero, because both sides of the comparison are `f32` that have been
/// through a device-pixel snap, and a box whose bottom lands a hundredth of a
/// pixel outside a window sized to hold it is arithmetic rather than clipping.
/// Small enough that nothing a user could see hides inside it.
const EDGE_TOLERANCE: Pixels = px(0.5);

/// Why this frame cannot be emitted: the window cut the surface.
///
/// # Why this exists, and why it is here rather than in the parser
///
/// The driver's window now follows the surface it is asked to draw
/// ([`Cell::window_extent`]), so the height a cell asks for is no longer capped
/// at some number chosen to keep it inside a fixed window. What the caps were
/// protecting still has to hold: **a surface must never be silently clipped by
/// the driver's window**, because `visible` is a statement about what a user
/// sees, and an anchor cut by the window's own content mask reports a `visible`
/// that is a fact about the window size instead of about the port. A snapshot
/// full of those is exactly the fake convergence this project refuses.
///
/// It cannot be a parse-time check, because the bound that survives is not a
/// number anyone can write down: the platform decides. macOS constrains a titled
/// window's frame to its screen, so asking for a window taller than the
/// display's visible frame returns a shorter one, and the surface inside it is
/// cut. Whether *this* machine can hold *this* cell is knowable only once a
/// window exists — so the check is made against the drawable area the platform
/// actually granted, on the frame that was actually drawn, and the answer is to
/// refuse rather than to emit.
///
/// # Vertical only, and why that is not an oversight
///
/// Horizontally, an anchor outside the window is a picture some surfaces really
/// have: `sidebar-carousel` scrolls its track, and three of its four panels sit
/// entirely outside the scrollport — and therefore outside the window — on
/// purpose, with `visible: false` being the fact under measurement. Refusing
/// those would refuse the surface's whole point. The width axis has its own
/// guard anyway, one layer up: [`Cell::parse`] rejects a viewport too narrow for
/// the surface plus its insets.
///
/// Vertically nothing scrolls, and the window is now sized to the surface — so
/// an anchor below the window's bottom edge means the window did not follow, and
/// there is nothing else it could mean.
fn cut_by_the_window(records: &[RawAnchor], cell: &Cell, viewport: Size<Pixels>) -> Option<String> {
    let asked = RowSurface::window_size(cell).height;
    for record in records {
        let bottom = record.bounds.origin.y + record.bounds.size.height;
        if bottom <= viewport.height + EDGE_TOLERANCE {
            continue;
        }
        // The tallest surface this window could have held: the surface starts at
        // `INSET_Y`, so everything below that is what is left. Named because a
        // refusal that only says "too tall" leaves the reader to bisect, and
        // this number is the one they would have arrived at.
        let holds = f32::from(viewport.height) - INSET_Y;
        return Some(format!(
            "`{}` reaches {}px down the window but its drawable area is only {}px tall, so the \
             surface is cut at the window edge and every `visible` in this snapshot would be an \
             artefact of the window size rather than a fact about the port. This cell asked for a \
             {}px window and the platform granted {}px — on macOS a window is constrained to its \
             display, so this cell needs a taller one. This window holds a surface up to {holds}px \
             tall. Nothing was written.",
            record.id,
            f32::from(bottom),
            f32::from(viewport.height),
            f32::from(asked),
            f32::from(viewport.height),
        ));
    }
    None
}

/// Why this frame cannot be emitted: the platform did not grant the window
/// this cell asked for.
///
/// # Why `cut_by_the_window` does not already cover this
///
/// That guard refuses when the granted window is short enough to slice an
/// anchor off — a fact visible in the anchors themselves, and [`emit`] runs it
/// first so its more specific, anchor-naming message wins whenever something
/// really is cut. A granted viewport can differ from the requested one
/// **without** cutting anything: a surface whose own width derives from
/// `window.viewport_size()` (`command`'s root is `min(viewport -
/// 2*VIEWPORT_PADDING, max_width)`) simply lays out narrower in a narrower
/// window, with every anchor safely inside the new, smaller edge.
/// `cut_by_the_window` has nothing to find in that picture — and yet
/// [`state_of`] reports `cell.viewport_width`, the number this cell **asked**
/// for, while every box in the frame answers to the number the platform
/// **granted**. Unrefused, the snapshot would claim to be a measurement of one
/// cell while its geometry belongs to another, and the two disagree by a
/// constant that reads exactly like a port defect: `command`'s root landing
/// at 537 regardless of `--width` (576/700/1200) *and* regardless of
/// `--viewport-width` (800/1200/1714) is what proved that one environmental
/// rather than a port bug — invariance under both flags is not a coincidence
/// a real layout defect would produce.
///
/// # Refuse, never correct
///
/// The obvious alternative — rewrite `state.width` to the granted value — is
/// wrong. The cell the operator asked for is the cell they meant: silently
/// relabelling it would produce a snapshot that disagrees with the
/// *reference*, which was captured at the requested size, not the granted
/// one, and a relabelled snapshot would go on to fail the differ in a way
/// that reads as an ordinary port defect the next time someone diffs it.
///
/// # Both axes
///
/// [`RowSurface::window_size`] derives its width from `cell.viewport_width`
/// and its height from [`Cell::window_extent`], and a platform can shrink
/// either — a tiling window manager is what this item found it on, but
/// nothing here assumes which axis or which cause.
///
/// # Tolerance, and why not bit-for-bit equality
///
/// [`EDGE_TOLERANCE`] (half a logical pixel), the same constant
/// `cut_by_the_window` already uses and for the same reason: both sides of
/// this comparison are `f32`s that have passed through the device-pixel snap
/// gpui performs when it resolves a window's actual size, so demanding exact
/// equality would flag a difference the platform's own rounding introduced —
/// one no anchor bound would ever show — as if it were a real mismatch. A
/// whole pixel of difference is not rounding; it is the platform having
/// granted a window this cell did not ask for.
fn granted_matches_requested(cell: &Cell, granted: Size<Pixels>) -> Option<String> {
    let requested = RowSurface::window_size(cell);
    let width_off = (requested.width - granted.width).abs();
    let height_off = (requested.height - granted.height).abs();
    if width_off <= EDGE_TOLERANCE && height_off <= EDGE_TOLERANCE {
        return None;
    }

    let name = cell.surface.name;
    let requested_width = f32::from(requested.width);
    let requested_height = f32::from(requested.height);
    let granted_width = f32::from(granted.width);
    let granted_height = f32::from(granted.height);
    let asked_viewport = cell.viewport_width;
    Some(format!(
        "`{name}` asked for a {requested_width}×{requested_height}px window \
         (`--viewport-width {asked_viewport}`) but the platform granted \
         {granted_width}×{granted_height}px. Every anchor in this frame was laid out against the \
         {granted_width}×{granted_height}px window the platform actually gave it, not the \
         {requested_width}×{requested_height}px one this cell asked for — so a snapshot built \
         from it would report `state.width: {asked_viewport}` while its geometry belongs to a \
         {granted_width}px viewport instead, which is a snapshot labelled with a cell it was not \
         measured in. A tiling window manager resizing the window after it opened is a common \
         cause; free up space for a {requested_width}×{requested_height}px window (or move it \
         off the tiler) and re-run. Nothing was written.",
    ))
}

/// Serialises the recorded frame and writes it where it was asked for.
///
/// `viewport` is the drawable area the platform **granted**, not the one the
/// cell asked for: [`cut_by_the_window`] is the guard that keeps a clipped
/// surface out of the corpus, and it can only be answered by the window that
/// exists. It is a parameter of this function rather than a check at the call
/// site so that there is no way to write a snapshot without passing it.
///
/// # Errors
///
/// The window cut the surface, the platform did not grant the window this
/// cell asked for ([`granted_matches_requested`]), the snapshot could not be
/// built (no root anchor was recorded, or some anchor id was recorded more
/// than once), could not be serialised, or could not be written.
pub fn emit(
    anchors: &AnchorRegistry,
    cell: &Cell,
    destination: &Destination,
    viewport: Size<Pixels>,
) -> Result<PathBuf, String> {
    // Built from the folded records rather than through `AnchorRegistry::snapshot`,
    // because the fold has to happen between "what prepaint recorded" and "what
    // the contract describes". See `fold_text_halves`.
    // Both the name and the root come off the cell's own `--surface`, so a
    // snapshot cannot claim to be one surface while being anchored to the
    // other's root — which would offset every bound by a constant *and* pass
    // the differ's surface check.
    let records = fold_text_halves(anchors.records());

    // Before anything is built, let alone written: a frame the window cut is
    // not a measurement of the port.
    if let Some(complaint) = cut_by_the_window(&records, cell, viewport) {
        return Err(complaint);
    }

    // A frame that was not cut can still have been measured in the wrong
    // cell: see `granted_matches_requested`. This runs second because it is
    // the more general of the two checks — anything `cut_by_the_window`
    // would have caught is also a granted/requested mismatch, so letting the
    // more specific, anchor-naming refusal go first keeps its message intact
    // for the case both guards could explain.
    if let Some(complaint) = granted_matches_requested(cell, viewport) {
        return Err(complaint);
    }

    let snapshot = Snapshot::build(
        cell.surface.name,
        state_of(cell),
        cell.surface.root,
        &records,
    )
    .map_err(|err| err.to_string())?;
    let json = snapshot.to_json().map_err(|err| err.to_string())?;

    match destination {
        Destination::Stdout => {
            let mut out = io::stdout().lock();
            writeln!(out, "{json}").map_err(|err| err.to_string())?;
            out.flush().map_err(|err| err.to_string())?;
            Ok(PathBuf::from("-"))
        }
        Destination::File(path) => {
            fs::write(path, format!("{json}\n")).map_err(|err| err.to_string())?;
            Ok(path.clone())
        }
    }
}

/// Says on stderr what happened, so a failed emit is never a silent one.
pub fn report(outcome: &Result<PathBuf, String>) {
    match outcome {
        Ok(path) => eprintln!("crowbar-app: snapshot written to {}", path.display()),
        Err(err) => eprintln!("crowbar-app: no snapshot: {err}"),
    }
}

#[cfg(test)]
mod tests {
    use gpui::{px, size};

    use super::{granted_matches_requested, state_of};
    use crate::row_surface::{Cell, RowSurface};
    use crate::surface::Surface;

    fn a_cell(args: &[&str]) -> Cell {
        Cell::parse(args.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// Each surface's root is the contract's, not a local invention: an
    /// extractor that picked a different root would offset every anchor by a
    /// constant, and one that reused *another* surface's root would produce a
    /// snapshot with no root at all.
    #[test]
    fn each_surface_names_its_own_root_anchor() {
        let root = |name: &str| Surface::parse(name).expect("registered").root;

        assert_eq!(root("git-status-row"), "git-row-item");
        assert_eq!(root("file-tree-row"), "file-row-item");
        assert_eq!(root("dropdown-menu"), "menu-popup");

        // And the snapshot's `surface` field is the word the command line takes,
        // so a run cannot claim a name the differ has never seen.
        for name in Surface::names() {
            assert_eq!(Surface::parse(name).expect("registered").name, name);
        }
    }

    /// The name and the root travel together off the **same** cell, so a
    /// snapshot cannot claim to be one surface while being anchored to another's
    /// root — which would offset every bound by a constant *and* pass the
    /// differ's surface check.
    #[test]
    fn the_cells_surface_carries_both_halves() {
        let git = a_cell(&[]);
        let menu = a_cell(&["--surface", "dropdown-menu"]);

        assert_eq!(git.surface.name, "git-status-row");
        assert_eq!(git.surface.root, "git-row-item");
        assert_eq!(menu.surface.name, "dropdown-menu");
        assert_eq!(menu.surface.root, "menu-popup");
    }

    /// `SurfaceState` sorts and deduplicates, so two spellings of the same cell
    /// are the same cell — which is what stops the differ refusing a comparison
    /// it should have made.
    #[test]
    fn the_flag_order_on_the_command_line_does_not_change_the_cell() {
        let one = a_cell(&["--flags", "selected,hover"]);
        let other = a_cell(&["--flags", "hover,selected"]);

        assert_eq!(state_of(&one), state_of(&other));
    }

    /// `state.width` is the **viewport**, so the matrix can hold the sidebar at
    /// the reference's fixed 294px and still move the cell — and two runs that
    /// differ only in surface width are the same cell, which is what makes a
    /// comparison between them legitimate.
    #[test]
    fn the_cell_is_keyed_on_the_viewport_and_not_on_the_surface() {
        let narrow_surface = a_cell(&["--width", "294", "--viewport-width", "800"]);
        let wide_surface = a_cell(&["--width", "420", "--viewport-width", "800"]);
        assert_eq!(state_of(&narrow_surface), state_of(&wide_surface));

        let narrow_viewport = a_cell(&["--width", "294", "--viewport-width", "600"]);
        assert_ne!(
            state_of(&narrow_surface),
            state_of(&narrow_viewport),
            "crossing the breakpoint changes what is rendered, so it must change the cell",
        );

        // The number itself, because "different cells" would also be satisfied
        // by reporting the surface width on both sides.
        let rendered = format!("{:?}", state_of(&narrow_surface));
        assert!(rendered.contains("width: 800"), "{rendered}");
        assert!(!rendered.contains("294"), "{rendered}");
    }

    /// **The defect this item exists to close.** Before `granted_matches_requested`
    /// existed, nothing compared `cell.viewport_width` against the window the
    /// platform actually granted — `state_of` reports the former
    /// unconditionally (see its own doc comment) and `cut_by_the_window` only
    /// ever looks at anchor bounds, never at the window's own width. A
    /// platform that grants a narrower window than `--viewport-width` asked
    /// for — measured for real at 1714px requested against a 569px granted
    /// window — produced no error at all: `emit` ran straight through to
    /// `Snapshot::build` and wrote a snapshot whose `state.width` was 1714
    /// while its layout was the 569px one. Reverting `granted_matches_requested`
    /// to `|_, _| None` reproduces exactly that: this test fails with "a 569px
    /// granted viewport must not pass for a 1714px request" because the call
    /// returns `None` instead of `Some(_)`.
    #[test]
    fn a_granted_width_narrower_than_the_cell_requested_is_refused() {
        let cell = a_cell(&["--width", "294", "--viewport-width", "1714"]);
        let requested = RowSurface::window_size(&cell);
        // The exact numbers this item measured `command` at, so this test
        // pins the real failure rather than an invented one.
        let granted = size(px(569.0), requested.height);

        let complaint = granted_matches_requested(&cell, granted)
            .expect("a 569px granted viewport must not pass for a 1714px request");
        assert!(complaint.contains("git-status-row"), "{complaint}");
        assert!(complaint.contains("1714"), "{complaint}");
        assert!(complaint.contains("569"), "{complaint}");
    }

    /// The height counterpart: **not** an anchor-cut, so `cut_by_the_window`
    /// would not catch it either — a granted window taller than the surface
    /// needs cuts nothing, it just answers to a viewport this cell never
    /// asked for.
    #[test]
    fn a_granted_height_that_disagrees_with_the_request_is_refused() {
        let cell = a_cell(&[
            "--surface",
            "resizable",
            "--width",
            "600",
            "--viewport-width",
            "700",
            "--shell-height",
            "500",
        ]);
        let requested = RowSurface::window_size(&cell);
        // Taller, not shorter — so nothing could possibly be cut, which is
        // the point: this is not `cut_by_the_window`'s case.
        let granted = size(requested.width, requested.height + px(50.0));

        let complaint = granted_matches_requested(&cell, granted)
            .expect("a granted height 50px taller than requested must not pass");
        assert!(complaint.contains("resizable"), "{complaint}");
        assert!(complaint.contains("700"), "{complaint}");
    }

    /// **The control.** Without it, "the guard fires" would also pass on a
    /// guard that refuses unconditionally — which is the shape of vacuous
    /// test this project has already shipped eight of. The identical cell,
    /// granted exactly the window it asked for, passes.
    #[test]
    fn a_granted_viewport_matching_the_request_is_not_refused() {
        let cell = a_cell(&["--width", "294", "--viewport-width", "800"]);
        let requested = RowSurface::window_size(&cell);

        assert_eq!(granted_matches_requested(&cell, requested), None);
    }

    /// **Mutation evidence for the ±0.5px tolerance**, not just its shape:
    /// exactly the tolerance passes, one whole pixel past it does not. Both
    /// sides are stated so that a guard which quietly widened or narrowed the
    /// tolerance would fail one assertion or the other.
    #[test]
    fn the_tolerance_absorbs_half_a_pixel_but_not_a_whole_one() {
        let cell = a_cell(&["--width", "294", "--viewport-width", "800"]);
        let requested = RowSurface::window_size(&cell);

        let half_pixel_over = size(requested.width + px(0.5), requested.height);
        assert_eq!(
            granted_matches_requested(&cell, half_pixel_over),
            None,
            "half a logical pixel is `ANCHORS.md` §5's own bounds tolerance",
        );

        let one_pixel_over = size(requested.width + px(1.0), requested.height);
        assert!(
            granted_matches_requested(&cell, one_pixel_over).is_some(),
            "a whole pixel is a real disagreement, not device-pixel rounding",
        );
    }
}
