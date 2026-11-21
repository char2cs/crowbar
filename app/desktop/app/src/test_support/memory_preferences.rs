use std::sync::{Mutex, PoisonError};

use sidebar::Preferences;

use crate::preferences_store::PreferencesStore;

#[derive(Debug, Default)]
pub(crate) struct MemoryPreferences {
    stored: Mutex<Preferences>,
}

impl PreferencesStore for MemoryPreferences {
    fn load(&self) -> Result<Preferences, sidebar::Error> {
        Ok(self
            .stored
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone())
    }

    fn save(
        &self,
        preferences: &Preferences,
    ) -> Result<(), sidebar::Error> {
        *self.stored.lock().unwrap_or_else(PoisonError::into_inner) = preferences.clone();
        Ok(())
    }
}
