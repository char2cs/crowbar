use std::io::Write;
use std::path::{Path, PathBuf};

use model::Layout;

use crate::error::{Error, Result};
use crate::loaded_layout::LoadedLayout;

#[derive(Debug, Clone)]
pub(crate) struct LayoutStore {
    path: PathBuf,
}

impl LayoutStore {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self { path }
    }

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
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |elapsed| elapsed.as_secs());
        let mut aside = self.path.clone().into_os_string();
        aside.push(format!(".bad-{stamp}"));
        let aside = PathBuf::from(aside);
        std::fs::rename(&self.path, &aside)
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
mod tests {
    use model::{Command, TabId};

    use super::*;

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

    enum Disk {
        Absent,
        Saved,
        Corrupt,
    }

    fn load_or_quarantine_on(disk: &Disk) -> (PathBuf, LoadedLayout, tempfile::TempDir) {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        match disk {
            Disk::Absent => {},
            Disk::Saved => LayoutStore::new(path.clone()).save(&layout()).unwrap(),
            Disk::Corrupt => std::fs::write(&path, "{ not json").unwrap(),
        }
        let loaded = LayoutStore::new(path.clone()).load_or_quarantine().unwrap();
        (path, loaded, dir)
    }

    const CASES: &[(&str, Disk)] = &[
        ("a first run starts with an empty layout", Disk::Absent),
        ("a healthy file is loaded as is", Disk::Saved),
        (
            "a corrupt file is set aside and the app starts empty",
            Disk::Corrupt,
        ),
    ];

    fn assert_set_aside(
        name: &str,
        path: &Path,
        loaded: &LoadedLayout,
    ) {
        let aside = loaded.quarantined.as_ref().unwrap();
        assert_eq!(loaded.layout, Layout::default(), "{name}: empty layout");
        assert!(
            !path.exists() && aside.exists(),
            "{name}: kept under another name"
        );
        assert_eq!(
            std::fs::read_to_string(aside).unwrap(),
            "{ not json",
            "{name}: contents kept"
        );
    }

    #[test]
    fn loading_at_startup_never_loses_the_user_a_layout() {
        for (name, disk) in CASES {
            let (path, loaded, _dir) = load_or_quarantine_on(disk);
            match disk {
                Disk::Absent => assert_eq!(
                    (&loaded.layout, &loaded.quarantined),
                    (&Layout::default(), &None),
                    "{name}"
                ),
                Disk::Saved => assert_eq!(
                    (&loaded.layout, &loaded.quarantined),
                    (&layout(), &None),
                    "{name}"
                ),
                Disk::Corrupt => assert_set_aside(name, &path, &loaded),
            }
        }
    }

    #[test]
    fn a_saved_layout_loads_back_and_leaves_no_staging_file() {
        let dir = tempfile::tempdir().unwrap();
        let store = LayoutStore::new(dir.path().join("state").join("layout.json"));
        store.save(&layout()).unwrap();
        assert_eq!(store.load().unwrap(), layout(), "round trip");
        let leftovers: Vec<_> = std::fs::read_dir(dir.path().join("state"))
            .unwrap()
            .map(|entry| entry.unwrap().file_name())
            .collect();
        assert_eq!(leftovers, ["layout.json"], "only the document remains");
    }

    #[test]
    fn a_corrupt_file_is_an_error_not_an_empty_layout() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        std::fs::write(&path, "{ not json").unwrap();
        assert!(
            matches!(LayoutStore::new(path).load(), Err(Error::Parse(..))),
            "refused"
        );
    }
}
