//! The sidebar's shell-local preferences, stored in one JSON file.

use std::collections::BTreeMap;
use std::path::Path;

use serde::{Deserialize, Serialize};

use crate::dock_side::DockSide;
use crate::error::Error;
use crate::mode::Mode;
use crate::panel::Panel;
use crate::row_id::RowId;
use crate::tree_state::TreeState;
use crate::visibility::Visibility;

/// Width, visibility, dock side, chosen mode, folded rows, host filter and the last space per
/// host. None of it goes to the daemon.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Preferences {
    /// Open or closed.
    pub visibility: Visibility,
    /// The remembered width.
    pub width: f32,
    /// The docked edge.
    pub side: DockSide,
    /// The chosen mode, which may be unavailable right now.
    pub mode: Mode,
    /// Ids of folded rows.
    pub folded: Vec<String>,
    /// The host whose spaces are listed, when more than one host exists.
    pub host_filter: Option<String>,
    /// The last space opened per host id.
    pub last_project_by_host: BTreeMap<String, String>,
}

impl Default for Preferences {
    fn default() -> Self {
        let panel = Panel::default();
        Self {
            visibility: panel.visibility(),
            width: panel.preferred(),
            side: panel.side(),
            mode: Mode::Chats,
            folded: Vec::new(),
            host_filter: None,
            last_project_by_host: BTreeMap::new(),
        }
    }
}

fn unreadable(
    path: &Path,
    source: std::io::Error,
) -> Error {
    Error::ReadPreferences {
        path: path.to_owned(),
        source,
    }
}

impl Preferences {
    /// Reads the file; a missing file yields the defaults.
    ///
    /// # Errors
    ///
    /// [`Error::ReadPreferences`] when the file exists but cannot be read, and
    /// [`Error::ParsePreferences`] when it is not valid JSON of this shape.
    pub fn load(path: &Path) -> Result<Self, Error> {
        let text = match std::fs::read_to_string(path) {
            Ok(text) => text,
            Err(source) if source.kind() == std::io::ErrorKind::NotFound => {
                return Ok(Self::default());
            },
            Err(source) => return Err(unreadable(path, source)),
        };
        serde_json::from_str(&text).map_err(|source| Error::ParsePreferences {
            path: path.to_owned(),
            source,
        })
    }

    /// Writes the file, replacing it.
    ///
    /// # Errors
    ///
    /// [`Error::WritePreferences`] when the file cannot be written.
    pub fn save(
        &self,
        path: &Path,
    ) -> Result<(), Error> {
        let write = |source| Error::WritePreferences {
            path: path.to_owned(),
            source,
        };
        let json = serde_json::to_string_pretty(self)
            .map_err(|error| write(std::io::Error::other(error)))?;
        atomic_file::write(path, json.as_bytes()).map_err(write)
    }

    pub(crate) fn panel(&self) -> Panel {
        Panel::restore(self.visibility, self.width, self.side)
    }

    pub(crate) fn tree(&self) -> TreeState {
        TreeState::with_folded(self.folded.iter().map(|id| RowId::from(id.as_str())))
    }

    #[must_use]
    pub(crate) fn updated(
        &self,
        panel: Panel,
        mode: Mode,
        tree: &TreeState,
    ) -> Self {
        Self {
            visibility: panel.visibility(),
            width: panel.preferred(),
            side: panel.side(),
            mode,
            folded: tree.folded().map(|id| id.as_str().to_owned()).collect(),
            ..self.clone()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn scratch(name: &str) -> std::path::PathBuf {
        std::env::temp_dir().join(format!("sidebar-prefs-{}-{name}.json", std::process::id()))
    }

    type Check = fn(&Result<Preferences, Error>) -> bool;

    fn is_default(loaded: &Result<Preferences, Error>) -> bool {
        matches!(loaded, Ok(prefs) if *prefs == Preferences::default())
    }

    fn is_parse_error(loaded: &Result<Preferences, Error>) -> bool {
        matches!(loaded, Err(Error::ParsePreferences { .. }))
    }

    fn is_clamped(loaded: &Result<Preferences, Error>) -> bool {
        matches!(loaded, Ok(prefs) if prefs.mode == Mode::Chats
            && (prefs.panel().preferred() - 640.0).abs() < 0.01)
    }

    type LoadCase = (&'static str, Option<&'static str>, Check);

    const LOAD_CASES: &[LoadCase] = &[
        ("a missing file yields the defaults", None, is_default),
        (
            "a corrupt file is an error, not a reset",
            Some("not json"),
            is_parse_error,
        ),
        (
            "partial files fall back per field and widths are clamped",
            Some(r#"{"width": 9000.0}"#),
            is_clamped,
        ),
    ];

    fn load_file(
        name: &str,
        content: Option<&str>,
    ) -> Result<Preferences, Error> {
        let path = scratch(name);
        if let Some(text) = content {
            std::fs::write(&path, text).expect("write");
        }
        let loaded = Preferences::load(&path);
        std::fs::remove_file(&path).ok();
        loaded
    }

    #[test]
    fn loading_handles_missing_corrupt_and_partial_files() {
        for (name, content, check) in LOAD_CASES {
            let loaded = load_file(name, *content);
            assert!(check(&loaded), "{name}");
        }
    }

    #[test]
    fn saving_and_loading_round_trips_every_field() {
        let path = scratch("roundtrip");
        let mut tree = TreeState::default();
        tree.toggle_fold(&RowId::from("row"));
        let mut panel = Panel::default();
        panel.commit(400.0);
        panel.dock_to(DockSide::Right);
        let mut prefs = Preferences::default().updated(panel, Mode::Files, &tree);
        prefs
            .last_project_by_host
            .insert("local".into(), "p1".into());
        prefs.save(&path).expect("save");
        let loaded = Preferences::load(&path).expect("load");
        std::fs::remove_file(&path).expect("cleanup");
        assert_eq!(loaded, prefs, "round trip");
        assert_eq!(loaded.panel().side(), DockSide::Right, "side");
        assert!((loaded.panel().preferred() - 400.0).abs() < 0.01, "width");
        assert_eq!(loaded.tree().folded().count(), 1, "folds");
    }
}
