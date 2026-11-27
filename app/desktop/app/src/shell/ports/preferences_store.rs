use std::fmt::Debug;

use sidebar::Preferences;

pub(crate) trait PreferencesStore: Debug + Send + Sync {
    fn load(&self) -> Result<Preferences, sidebar::Error>;

    fn save(
        &self,
        preferences: &Preferences,
    ) -> Result<(), sidebar::Error>;
}
