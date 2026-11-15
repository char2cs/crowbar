use std::io::ErrorKind;
use std::path::{Path, PathBuf};

use crate::error::Error;
use crate::window_state::WindowState;

/// The file the window state is kept in.
#[derive(Debug, Clone)]
pub struct WindowStore {
    path: PathBuf,
}

impl WindowStore {
    /// A store backed by `path`; the file is created on the first save.
    #[must_use]
    pub fn new(path: &Path) -> Self {
        Self {
            path: path.to_path_buf(),
        }
    }

    /// The remembered state, `None` on a first run.
    ///
    /// # Errors
    /// [`Error::Io`] when the file cannot be read, [`Error::Corrupt`] when it is not valid.
    pub fn load(&self) -> Result<Option<WindowState>, Error> {
        let text = match std::fs::read_to_string(&self.path) {
            Ok(text) => text,
            Err(error) if error.kind() == ErrorKind::NotFound => return Ok(None),
            Err(source) => return Err(self.io(source)),
        };
        serde_json::from_str(&text)
            .map(Some)
            .map_err(|source| Error::Corrupt {
                path: self.path.clone(),
                source,
            })
    }

    /// Saves `state` atomically (write beside, then rename), creating the directory.
    ///
    /// # Errors
    /// [`Error::Io`] when the file cannot be written.
    pub fn save(
        &self,
        state: &WindowState,
    ) -> Result<(), Error> {
        let text = serde_json::to_string(state).map_err(|source| Error::Corrupt {
            path: self.path.clone(),
            source,
        })?;
        if let Some(dir) = self.path.parent() {
            std::fs::create_dir_all(dir).map_err(|source| self.io(source))?;
        }
        let staging = self.path.with_extension("json.tmp");
        std::fs::write(&staging, text).map_err(|source| self.io(source))?;
        std::fs::rename(&staging, &self.path).map_err(|source| self.io(source))
    }

    fn io(
        &self,
        source: std::io::Error,
    ) -> Error {
        Error::Io {
            path: self.path.clone(),
            source,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rect::Rect;

    const STATE: WindowState = WindowState {
        bounds: Rect {
            x: 10.0,
            y: 20.0,
            width: 900.0,
            height: 600.0,
        },
        maximized: true,
    };

    #[test]
    fn a_first_run_has_no_state_and_a_saved_state_comes_back() {
        let dir = tempfile::tempdir().unwrap();
        let store = WindowStore::new(&dir.path().join("nested/window.json"));
        assert!(store.load().unwrap().is_none());
        store.save(&STATE).unwrap();
        assert_eq!(store.load().unwrap(), Some(STATE));
    }

    #[test]
    fn a_damaged_file_is_reported() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("window.json");
        std::fs::write(&path, "{nope").unwrap();
        assert!(matches!(
            WindowStore::new(&path).load(),
            Err(Error::Corrupt { .. })
        ));
    }
}
