use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::UNIX_EPOCH;

use layout::Layout;

use crate::adapters::disk_files::DiskFiles;
use crate::adapters::system_clock::SystemClock;
use crate::error::{Error, Result};
use crate::shell::ports::clock::Clock;
use crate::shell::shell_state::persistence::layout_files::LayoutFiles;
use crate::shell::shell_state::persistence::loaded_layout::LoadedLayout;

#[derive(Debug, Clone)]
pub(crate) struct LayoutStore {
    path: PathBuf,
    files: Arc<dyn LayoutFiles>,
    clock: Arc<dyn Clock>,
}

impl LayoutStore {
    pub(crate) fn new(
        path: PathBuf,
        files: Arc<dyn LayoutFiles>,
        clock: Arc<dyn Clock>,
    ) -> Self {
        Self { path, files, clock }
    }

    pub(crate) fn on_disk(path: PathBuf) -> Self {
        Self::new(
            path,
            Arc::new(DiskFiles::default()),
            Arc::new(SystemClock::default()),
        )
    }

    pub(crate) fn load(&self) -> Result<Layout> {
        let text = match self.files.read(&self.path) {
            Ok(text) => text,
            Err(source) if source.kind() == std::io::ErrorKind::NotFound => {
                return Ok(Layout::default());
            },
            Err(source) => return Err(Error::Read(self.path.clone(), source)),
        };
        serde_json::from_str(&text).map_err(|source| Error::Parse(self.path.clone(), source))
    }

    pub(crate) fn load_or_quarantine(&self) -> Result<LoadedLayout> {
        match self.load() {
            Ok(layout) => Ok(LoadedLayout {
                layout,
                quarantined: None,
            }),
            Err(Error::Parse(path, error)) => Ok(self.start_fresh(&path, &error)),
            Err(other) => Err(other),
        }
    }

    fn start_fresh(
        &self,
        path: &Path,
        error: &serde_json::Error,
    ) -> LoadedLayout {
        tracing::error!(%error, path = %path.display(), "the saved layout is unreadable");
        LoadedLayout {
            layout: Layout::default(),
            quarantined: self.quarantine(),
        }
    }

    fn quarantine(&self) -> Option<PathBuf> {
        let stamp = self
            .clock
            .wall()
            .duration_since(UNIX_EPOCH)
            .map_or(0, |elapsed| elapsed.as_secs());
        let mut aside = self.path.clone().into_os_string();
        aside.push(format!(".bad-{stamp}"));
        let aside = PathBuf::from(aside);
        self.files
            .rename(&self.path, &aside)
            .inspect_err(|error| {
                tracing::error!(%error, "could not set the unreadable layout aside");
            })
            .ok()
            .map(|()| aside)
    }

    pub(crate) fn save(
        &self,
        layout: &Layout,
    ) -> Result<()> {
        let text = serde_json::to_string(layout)
            .map_err(|source| Error::Parse(self.path.clone(), source))?;
        self.files
            .write(&self.path, text.as_bytes())
            .map_err(|source| Error::Write(self.path.clone(), source))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FixedClock, MemoryFiles, add_tab};

    const PATH: &str = "/state/layout.json";
    const CORRUPT: &str = "{ not json";
    const ASIDE: &str = "/state/layout.json.bad-42";

    fn layout() -> Layout {
        let mut layout = Layout::default();
        add_tab(&mut layout, 1);
        layout
    }

    fn store_over(files: &Arc<MemoryFiles>) -> LayoutStore {
        LayoutStore::new(
            PathBuf::from(PATH),
            Arc::<MemoryFiles>::clone(files),
            Arc::new(FixedClock::at(42)),
        )
    }

    #[test]
    fn a_saved_layout_loads_back_as_is_and_a_refused_write_is_a_write_error() {
        let files = Arc::new(MemoryFiles::default());
        let store = store_over(&files);
        store.save(&layout()).unwrap();
        assert_eq!(store.load().unwrap(), layout(), "round trip");
        let loaded = store.load_or_quarantine().unwrap();
        assert_eq!(
            (loaded.layout, loaded.quarantined),
            (layout(), None),
            "at startup"
        );
        assert_eq!(files.names(), [PathBuf::from(PATH)], "only the document");
        let refusing = Arc::new(MemoryFiles::refusing_writes());
        let saved = store_over(&refusing).save(&layout());
        assert!(matches!(saved, Err(Error::Write(..))), "{saved:?}");
    }

    #[test]
    fn a_missing_or_corrupt_file_starts_empty_and_a_corrupt_one_is_set_aside_intact() {
        let empty = store_over(&Arc::new(MemoryFiles::default()));
        let first = empty.load_or_quarantine().unwrap();
        assert_eq!(
            (first.layout, first.quarantined),
            (Layout::default(), None),
            "first run"
        );
        let files = Arc::new(MemoryFiles::default());
        files.seed(Path::new(PATH), CORRUPT);
        let refused = store_over(&files).load();
        assert!(
            matches!(refused, Err(Error::Parse(..))),
            "a plain load refuses it"
        );
        let loaded = store_over(&files).load_or_quarantine().unwrap();
        let aside = Some(PathBuf::from(ASIDE));
        assert_eq!(
            (loaded.layout, loaded.quarantined),
            (Layout::default(), aside)
        );
        let kept = files.text(Path::new(ASIDE));
        assert_eq!(kept.as_deref(), Some(CORRUPT), "contents kept");
    }
}
