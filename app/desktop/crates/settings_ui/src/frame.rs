//! What every drawing function of the view needs.

use gpui::Entity;

use crate::fields::Fields;
use crate::record_backend::RecordBackend;
use crate::settings_modal::SettingsModal;
use crate::settings_state::SettingsState;

#[derive(Clone, Copy)]
pub(crate) struct Frame<'a> {
    pub(crate) state: &'a SettingsState,
    pub(crate) owner: &'a Entity<SettingsModal>,
    pub(crate) fields: &'a Fields,
    pub(crate) records: &'a dyn RecordBackend,
}
