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
            Err(source) => {
                return Err(Error::ReadPreferences {
                    path: path.to_owned(),
                    source,
                });
            },
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
    pub fn save(&self, path: &Path) -> Result<(), Error> {
        let write = |source| Error::WritePreferences {
            path: path.to_owned(),
            source,
        };
        let json = serde_json::to_string_pretty(self)
            .map_err(|error| write(std::io::Error::other(error)))?;
        std::fs::write(path, json).map_err(write)
    }

    /// The panel geometry these preferences describe; the width is clamped.
    pub fn panel(&self) -> Panel {
        Panel::restore(self.visibility, self.width, self.side)
    }

    /// The tree state with the stored folds applied.
    pub fn tree(&self) -> TreeState {
        TreeState::with_folded(self.folded.iter().map(|id| RowId::from(id.as_str())))
    }

    /// These preferences after the panel, mode and tree changed.
    #[must_use]
    pub fn updated(&self, panel: &Panel, mode: Mode, tree: &TreeState) -> Self {
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
#[path = "preferences_tests.rs"]
mod tests;
