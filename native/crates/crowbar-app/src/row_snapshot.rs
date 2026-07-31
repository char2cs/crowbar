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

use crowbar_driver::{AnchorRegistry, Content, Flag, Snapshot, SurfaceState, Theme as SnapshotTheme};
use crowbar_ui::Appearance;
use crowbar_ui::components::ContentLength;

use crate::driver_anchors::fold_text_halves;
use crate::row_surface::{Cell, SURFACE, StateFlag};

/// The anchor id everything else is reported relative to.
const ROOT: &str = crowbar_ui::components::ID_ITEM;

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

/// Serialises the recorded frame and writes it where it was asked for.
///
/// # Errors
///
/// The snapshot could not be built (no root anchor was recorded), could not be
/// serialised, or could not be written.
pub fn emit(
    anchors: &AnchorRegistry,
    cell: &Cell,
    destination: &Destination,
) -> Result<PathBuf, String> {
    // Built from the folded records rather than through `AnchorRegistry::snapshot`,
    // because the fold has to happen between "what prepaint recorded" and "what
    // the contract describes". See `fold_text_halves`.
    let snapshot = Snapshot::build(
        SURFACE,
        state_of(cell),
        ROOT,
        &fold_text_halves(anchors.records()),
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
    use super::{ROOT, state_of};
    use crate::row_surface::Cell;

    fn a_cell(args: &[&str]) -> Cell {
        Cell::parse(args.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// The root is the contract's, not a local invention: an extractor that
    /// picked a different root would offset every anchor by a constant.
    #[test]
    fn the_root_is_the_contracts_root_anchor() {
        assert_eq!(ROOT, "git-row-item");
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
}
