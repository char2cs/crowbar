mod adapters;
mod storage;
#[cfg(test)]
mod test_support;

use std::collections::BTreeMap;
use std::path::Path;

use serde::{Deserialize, Serialize};

use crate::error::Error;
use crate::frame::dock_side::DockSide;
use crate::frame::mode::Mode;
use crate::frame::panel::Panel;
use crate::frame::preferences::adapters::disk_storage::DiskStorage;
use crate::frame::preferences::storage::Storage;
use crate::frame::visibility::Visibility;
use crate::shell::RowId;
use crate::tree::Tree;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Preferences {
    pub visibility: Visibility,
    pub width: f32,
    pub side: DockSide,
    pub mode: Mode,
    pub folded: Vec<String>,
    pub host_filter: Option<String>,
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
    pub fn load(path: &Path) -> Result<Self, Error> {
        Self::load_from(&DiskStorage::at(path))
    }

    pub fn save(
        &self,
        path: &Path,
    ) -> Result<(), Error> {
        self.save_to(&DiskStorage::at(path))
    }

    fn load_from(file: &dyn Storage) -> Result<Self, Error> {
        let path = file.path();
        let stored = file.read().map_err(|source| unreadable(path, source))?;
        let Some(text) = stored else {
            return Ok(Self::default());
        };
        serde_json::from_str(&text).map_err(|source| Error::ParsePreferences {
            path: path.to_owned(),
            source,
        })
    }

    fn save_to(
        &self,
        file: &dyn Storage,
    ) -> Result<(), Error> {
        let write = |source| Error::WritePreferences {
            path: file.path().to_owned(),
            source,
        };
        let json = serde_json::to_string_pretty(self)
            .map_err(|error| write(std::io::Error::other(error)))?;
        file.write(&json).map_err(write)
    }

    pub fn panel(&self) -> Panel {
        Panel::restore(self.visibility, self.width, self.side)
    }

    pub fn tree(&self) -> Tree {
        Tree::with_folded(self.folded.iter().map(|id| RowId::from(id.as_str())))
    }

    #[must_use]
    pub fn updated(
        &self,
        panel: Panel,
        mode: Mode,
        tree: &Tree,
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
    use crate::frame::preferences::test_support::MemoryStorage;

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

    fn load_file(content: Option<&str>) -> Result<Preferences, Error> {
        Preferences::load_from(&MemoryStorage::with(content))
    }

    #[test]
    fn loading_handles_missing_corrupt_and_partial_files() {
        for (name, content, check) in LOAD_CASES {
            let loaded = load_file(*content);
            assert!(check(&loaded), "{name}");
        }
    }

    #[test]
    fn saving_and_loading_round_trips_every_field() {
        let file = MemoryStorage::with(None);
        let mut tree = Tree::default();
        tree.toggle_fold(&RowId::from("row"));
        let mut panel = Panel::default();
        panel.commit(400.0);
        panel.dock_to(DockSide::Right);
        let mut prefs = Preferences::default().updated(panel, Mode::Files, &tree);
        prefs
            .last_project_by_host
            .insert("local".into(), "p1".into());
        prefs.save_to(&file).expect("save");
        let loaded = Preferences::load_from(&file).expect("load");
        assert_eq!(loaded, prefs, "round trip");
        assert_eq!(loaded.panel().side(), DockSide::Right, "side");
        assert!((loaded.panel().preferred() - 400.0).abs() < 0.01, "width");
        assert_eq!(loaded.tree().folded().count(), 1, "folds");
    }
}
