use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::UNIX_EPOCH;

use model::Layout;

use crate::adapters::disk_layout_files::DiskLayoutFiles;
use crate::adapters::system_clock::SystemClock;
use crate::clock::Clock;
use crate::error::{Error, Result};
use crate::layout_files::LayoutFiles;
use crate::loaded_layout::LoadedLayout;

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
            Arc::new(DiskLayoutFiles::default()),
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
    use model::{Command, TabId};

    use super::*;
    use crate::test_support::{FixedClock, MemoryFiles};

    const PATH: &str = "/state/layout.json";
    const CORRUPT: &str = "{ not json";
    const ASIDE: &str = "/state/layout.json.bad-42";

    fn layout() -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: "Chat".into(),
            })
            .unwrap();
        layout
    }

    fn store_over(files: &Arc<MemoryFiles>) -> LayoutStore {
        LayoutStore::new(
            PathBuf::from(PATH),
            Arc::<MemoryFiles>::clone(files),
            Arc::new(FixedClock::at(42)),
        )
    }

    enum Disk {
        Absent,
        Saved,
        Corrupt,
    }

    struct Case {
        name: &'static str,
        disk: Disk,
        quarantined: Option<&'static str>,
    }

    const CASES: &[Case] = &[
        Case {
            name: "a first run starts with an empty layout",
            disk: Disk::Absent,
            quarantined: None,
        },
        Case {
            name: "a healthy file is loaded as is",
            disk: Disk::Saved,
            quarantined: None,
        },
        Case {
            name: "a corrupt file is set aside and the app starts empty",
            disk: Disk::Corrupt,
            quarantined: Some(ASIDE),
        },
    ];

    fn files_holding(disk: &Disk) -> Arc<MemoryFiles> {
        let files = Arc::new(MemoryFiles::default());
        match disk {
            Disk::Absent => {},
            Disk::Saved => store_over(&files).save(&layout()).unwrap(),
            Disk::Corrupt => files.seed(Path::new(PATH), CORRUPT),
        }
        files
    }

    fn expected_layout(disk: &Disk) -> Layout {
        match disk {
            Disk::Saved => layout(),
            Disk::Absent | Disk::Corrupt => Layout::default(),
        }
    }

    #[test]
    fn loading_at_startup_never_loses_the_user_a_layout() {
        for case in CASES {
            let files = files_holding(&case.disk);
            let loaded = store_over(&files).load_or_quarantine().unwrap();
            assert_eq!(loaded.layout, expected_layout(&case.disk), "{}", case.name);
            let aside = case.quarantined.map(PathBuf::from);
            assert_eq!(loaded.quarantined, aside, "{}: set aside", case.name);
            let kept = aside.and_then(|path| files.text(&path));
            let expected = case.quarantined.map(|_| CORRUPT);
            assert_eq!(kept.as_deref(), expected, "{}: contents kept", case.name);
        }
    }

    #[test]
    fn a_saved_layout_loads_back() {
        let files = Arc::new(MemoryFiles::default());
        let store = store_over(&files);
        store.save(&layout()).unwrap();
        assert_eq!(store.load().unwrap(), layout(), "round trip");
        assert_eq!(files.names(), [PathBuf::from(PATH)], "only the document");
    }

    #[test]
    fn a_corrupt_file_is_an_error_not_an_empty_layout() {
        let files = Arc::new(MemoryFiles::default());
        files.seed(Path::new(PATH), CORRUPT);
        assert!(
            matches!(store_over(&files).load(), Err(Error::Parse(..))),
            "refused"
        );
    }

    #[test]
    fn a_refused_write_is_a_write_error() {
        let files = Arc::new(MemoryFiles::refusing_writes());
        let saved = store_over(&files).save(&layout());
        assert!(matches!(saved, Err(Error::Write(..))), "{saved:?}");
    }
}
