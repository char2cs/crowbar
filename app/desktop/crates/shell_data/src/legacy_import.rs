//! The one-time copy of the previous desktop app's settings into the shell's settings file.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::adapters::DiskFiles;
use crate::error::{Error, Result};
use crate::file_store::FileStore;
use crate::legacy_locate;
use crate::legacy_merge::merge;
use crate::legacy_sqlite::LegacySqlite;
use crate::legacy_store::LegacyStore;
use crate::setting_file::SettingFile;

const MARKER: &str = "legacy-imported";

/// Where the old data may be and where the shell keeps its settings.
#[derive(Debug, Clone)]
pub struct LegacyImport {
    /// The old database, when one was found.
    pub database: Option<PathBuf>,
    /// The shell's `settings.json`.
    pub settings: PathBuf,
}

impl LegacyImport {
    /// The import for a user whose home folder is `user_home`, looking where the previous app
    /// kept its data on this system.
    #[must_use]
    pub fn discover(
        user_home: &Path,
        settings: PathBuf,
    ) -> Self {
        Self {
            database: legacy_locate::under(&DiskFiles::default(), user_home),
            settings,
        }
    }

    /// Copies the settings once: a later start finds the marker and does nothing. Returns how
    /// many settings were taken.
    ///
    /// # Errors
    /// [`Error::Legacy`] when the old database exists and cannot be read, and [`Error::File`] or
    /// [`Error::Json`] when the settings file cannot be read or written.
    pub fn run(&self) -> Result<usize> {
        let files: Arc<dyn FileStore> = Arc::new(DiskFiles::default());
        self.run_on(&files)
    }

    fn run_on(
        &self,
        files: &Arc<dyn FileStore>,
    ) -> Result<usize> {
        let marker = marker_of(&self.settings);
        if files.read(&marker).is_ok() {
            return Ok(0);
        }
        let Some(database) = &self.database else {
            return Ok(0);
        };
        let taken = import(
            &SettingFile::new(self.settings.clone(), Arc::clone(files)),
            &LegacySqlite::new(database.clone()),
        )?;
        files.write(&marker, b"").map_err(|source| Error::File {
            path: marker,
            source,
        })?;
        Ok(taken)
    }
}

pub(crate) fn import(
    file: &SettingFile,
    store: &dyn LegacyStore,
) -> Result<usize> {
    let mut current = file.load()?;
    let taken = merge(&mut current, &store.entries()?);
    if taken > 0 {
        file.save(&current)?;
    }
    Ok(taken)
}

fn marker_of(settings: &Path) -> PathBuf {
    settings.with_file_name(MARKER)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::legacy_store::Entries;
    use crate::test_support::FakeFiles;

    struct Fixed(Entries);

    impl LegacyStore for Fixed {
        fn entries(&self) -> Result<Entries> {
            Ok(self.0.clone())
        }
    }

    const SETTINGS: &str = "/state/settings.json";

    #[test]
    fn imported_settings_reach_the_file_and_existing_ones_stay() {
        let file = SettingFile::new(SETTINGS.into(), Arc::new(FakeFiles::default()));
        let entries = Entries::from([
            (
                "crowbar:settings:themeMode".to_owned(),
                "\"dark\"".to_owned(),
            ),
            ("crowbar:settings:tabSize".to_owned(), "8".to_owned()),
        ]);
        let first = import(&file, &Fixed(entries.clone())).unwrap();
        let again = import(&file, &Fixed(entries)).unwrap();
        assert_eq!((first, again, file.load().unwrap().len()), (2, 0, 2));
    }

    #[test]
    fn the_marker_stops_a_second_import_and_no_database_leaves_no_marker() {
        let files: Arc<FakeFiles> = Arc::default();
        let store: Arc<dyn FileStore> = Arc::<FakeFiles>::clone(&files);
        let settings = PathBuf::from(SETTINGS);
        let none = LegacyImport {
            database: None,
            settings: settings.clone(),
        };
        assert_eq!(none.run_on(&store).unwrap(), 0);
        assert!(store.read(&marker_of(&settings)).is_err());
        let missing = LegacyImport {
            database: Some("/absent.localstorage".into()),
            settings: settings.clone(),
        };
        assert!(missing.run_on(&store).is_err());
        assert!(store.read(&marker_of(&settings)).is_err());
        files.put("/state/legacy-imported", b"");
        assert_eq!(missing.run_on(&store).unwrap(), 0);
    }
}
