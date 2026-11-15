use std::io::Write;
use std::path::PathBuf;

use model::Layout;

use crate::error::{Error, Result};

/// The layout's state file: one JSON document, replaced atomically on every save.
#[derive(Debug, Clone)]
pub(crate) struct LayoutStore {
    path: PathBuf,
}

impl LayoutStore {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self { path }
    }

    /// The saved layout, or an empty one on the first run.
    pub(crate) fn load(&self) -> Result<Layout> {
        let text = match std::fs::read_to_string(&self.path) {
            Ok(text) => text,
            Err(source) if source.kind() == std::io::ErrorKind::NotFound => {
                return Ok(Layout::default());
            },
            Err(source) => return Err(Error::Read(self.path.clone(), source)),
        };
        serde_json::from_str(&text).map_err(|source| Error::Parse(self.path.clone(), source))
    }

    /// Writes `layout` next to the file, then renames it over, so a crash leaves either the
    /// old or the new document and never half of one.
    pub(crate) fn save(&self, layout: &Layout) -> Result<()> {
        let text = serde_json::to_string(layout)
            .map_err(|source| Error::Parse(self.path.clone(), source))?;
        let staged = self.path.with_extension("json.tmp");
        if let Some(dir) = self.path.parent() {
            std::fs::create_dir_all(dir)
                .map_err(|source| Error::Write(dir.to_path_buf(), source))?;
        }
        let write = |path: &PathBuf| -> std::io::Result<()> {
            let mut file = std::fs::File::create(path)?;
            file.write_all(text.as_bytes())?;
            file.sync_all()
        };
        write(&staged).map_err(|source| Error::Write(staged.clone(), source))?;
        std::fs::rename(&staged, &self.path)
            .map_err(|source| Error::Write(self.path.clone(), source))
    }
}

#[cfg(test)]
#[path = "layout_store_tests.rs"]
mod tests;
