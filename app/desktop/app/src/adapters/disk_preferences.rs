use std::path::PathBuf;

use sidebar::Preferences;

use crate::shell::ports::preferences_store::PreferencesStore;

#[derive(Debug)]
pub(crate) struct DiskPreferences {
    pub(crate) path: PathBuf,
}

impl PreferencesStore for DiskPreferences {
    fn load(&self) -> Result<Preferences, sidebar::Error> {
        Preferences::load(&self.path)
    }

    fn save(
        &self,
        preferences: &Preferences,
    ) -> Result<(), sidebar::Error> {
        preferences.save(&self.path)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const MANIFEST: &str = concat!(env!("CARGO_MANIFEST_DIR"), "/Cargo.toml");

    #[test]
    fn a_missing_file_loads_the_defaults_and_a_save_beneath_a_regular_file_fails() {
        let missing = DiskPreferences {
            path: PathBuf::from("/nonexistent-crowbar/sidebar.json"),
        };
        assert_eq!(missing.load().unwrap(), Preferences::default());
        let beneath_a_file = DiskPreferences {
            path: PathBuf::from(MANIFEST).join("sidebar.json"),
        };
        assert!(beneath_a_file.save(&Preferences::default()).is_err());
    }
}
