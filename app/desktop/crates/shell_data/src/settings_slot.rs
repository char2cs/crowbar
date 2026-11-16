//! The shell settings once they have opened, for readers built before that.

use std::sync::{Arc, OnceLock};

use settings_ui::{Key, Value};

use crate::settings_core::SettingsCore;

#[derive(Debug, Clone, Default)]
pub(crate) struct SettingsSlot {
    core: Arc<OnceLock<Arc<SettingsCore>>>,
}

impl SettingsSlot {
    pub(crate) fn install(
        &self,
        core: &Arc<SettingsCore>,
    ) {
        self.core.get_or_init(|| Arc::clone(core));
    }

    pub(crate) fn get(
        &self,
        key: &str,
    ) -> Option<Value> {
        self.core.get()?.get(&Key::new(key))
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
