//! The shell settings once they have opened, for readers built before that.

use std::sync::{Arc, OnceLock};

use settings_ui::{Key, Value};

type Reader = Arc<dyn Fn(&Key) -> Option<Value> + Send + Sync>;

#[derive(Clone, Default)]
pub(crate) struct SettingsSlot {
    reader: Arc<OnceLock<Reader>>,
}

impl std::fmt::Debug for SettingsSlot {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("SettingsSlot")
            .field("installed", &self.reader.get().is_some())
            .finish()
    }
}

impl SettingsSlot {
    pub(crate) fn install(
        &self,
        reader: impl Fn(&Key) -> Option<Value> + Send + Sync + 'static,
    ) {
        self.reader.get_or_init(|| Arc::new(reader));
    }

    pub(crate) fn get(
        &self,
        key: &str,
    ) -> Option<Value> {
        self.reader.get()?(&Key::new(key))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nothing_is_read_before_the_settings_open() {
        assert_eq!(SettingsSlot::default().get("fileTreeDensity"), None);
    }
}
