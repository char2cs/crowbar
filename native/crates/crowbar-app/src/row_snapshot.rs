//! The driver-backed [`AnchorSink`] and the snapshot it emits.
//!
//! `--features driver` only. The shipping build carries `Unanchored`, whose
//! elements request layout identically, so what the oracle measures is what
//! ships.

use std::env;
use std::fs;
use std::io::{self, Write as _};
use std::path::PathBuf;

use crowbar_driver::{AnchorRegistry, Content, Flag, SurfaceState, Theme as SnapshotTheme};
use crowbar_ui::Appearance;
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, Div, IntoElement as _, SharedString};

use crate::row_surface::{Cell, SURFACE, StateFlag};

/// The anchor id everything else is reported relative to.
const ROOT: &str = crowbar_ui::components::ID_ITEM;

/// Wraps the row's elements in the driver's anchor elements.
///
/// The wrappers contribute no taffy node and return `None` from
/// `Element::id()`, so the layout tree with them is the layout tree without
/// them — see `crowbar-driver`'s `src/element.rs`.
pub struct DriverAnchors;

impl AnchorSink for DriverAnchors {
    fn root(&self, id: SharedString, element: Div) -> AnyElement {
        crowbar_driver::anchor_root(id, element).into_any_element()
    }

    fn boxed(&self, id: SharedString, element: Div) -> AnyElement {
        crowbar_driver::anchor(id, element).into_any_element()
    }

    fn text(&self, id: SharedString, content: SharedString) -> AnyElement {
        crowbar_driver::anchor_text(id, content).into_any_element()
    }
}

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
#[must_use]
pub fn state_of(cell: &Cell) -> SurfaceState {
    SurfaceState::new(
        u32::from(cell.width),
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
    let snapshot = anchors
        .snapshot(SURFACE, state_of(cell), ROOT)
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
        let one = Cell::parse(["--flags", "selected,hover"].into_iter().map(str::to_owned))
            .expect("well-formed");
        let other = Cell::parse(["--flags", "hover,selected"].into_iter().map(str::to_owned))
            .expect("well-formed");

        assert_eq!(state_of(&one), state_of(&other));
    }
}
