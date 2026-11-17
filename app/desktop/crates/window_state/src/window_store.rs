use std::path::Path;
use std::sync::Arc;

use crate::adapters::disk_file::DiskFile;
use crate::error::Error;
use crate::state_file::StateFile;
use crate::window_state::WindowState;

/// The file the window state is kept in.
#[derive(Debug, Clone)]
pub struct WindowStore {
    file: Arc<dyn StateFile>,
}

impl WindowStore {
    /// A store backed by `path`; the file is created on the first save.
    #[must_use]
    pub fn new(path: &Path) -> Self {
        Self::over(Arc::new(DiskFile::at(path)))
    }

    fn over(file: Arc<dyn StateFile>) -> Self {
        Self { file }
    }

    /// The remembered state, `None` on a first run.
    ///
    /// # Errors
    /// [`Error::Io`] when the file cannot be read, [`Error::Corrupt`] when it is not valid.
    pub fn load(&self) -> Result<Option<WindowState>, Error> {
        let text = self.file.read().map_err(|source| self.io(source))?;
        text.map(|text| self.parse(&text)).transpose()
    }

    fn parse(
        &self,
        text: &str,
    ) -> Result<WindowState, Error> {
        let state: WindowState = serde_json::from_str(text).map_err(|source| Error::Corrupt {
            path: self.file.path().to_path_buf(),
            source,
        })?;
        let path = self.file.path().to_path_buf();
        state
            .bounds
            .validated()
            .map(|_| state)
            .ok_or(Error::OutOfRange { path })
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
            path: self.file.path().to_path_buf(),
            source,
        })?;
        self.file.write(&text).map_err(|source| self.io(source))
    }

    fn io(
        &self,
        source: std::io::Error,
    ) -> Error {
        Error::Io {
            path: self.file.path().to_path_buf(),
            source,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rect::Rect;
    use crate::test_support::MemoryFile;

    const STATE: WindowState = WindowState {
        bounds: Rect {
            x: 10.0,
            y: 20.0,
            width: 900.0,
            height: 600.0,
        },
        maximized: true,
    };

    fn store(text: Option<&str>) -> WindowStore {
        WindowStore::over(Arc::new(MemoryFile::with(text)))
    }

    #[test]
    fn a_first_run_has_no_state_and_a_saved_state_comes_back() {
        let store = store(None);
        assert!(store.load().unwrap().is_none());
        store.save(&STATE).unwrap();
        assert_eq!(store.load().unwrap(), Some(STATE));
    }

    #[test]
    fn damaged_or_out_of_range_text_is_reported() {
        const NOT_FINITE: &str =
            r#"{"bounds":{"x":0.0,"y":0.0,"width":1e39,"height":600.0},"maximized":false}"#;
        let store = store(None);
        assert!(matches!(store.parse("{nope"), Err(Error::Corrupt { .. })));
        assert!(matches!(
            store.parse(NOT_FINITE),
            Err(Error::OutOfRange { .. })
        ));
    }
}
