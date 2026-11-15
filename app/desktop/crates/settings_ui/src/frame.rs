//! What every drawing function of the view needs.

use crate::fields::Fields;
use crate::intent_sink::IntentSink;
use crate::record_backend::RecordBackend;
use crate::settings_state::SettingsState;

#[derive(Clone, Copy)]
pub(crate) struct Frame<'a> {
    pub(crate) state: &'a SettingsState,
    pub(crate) sink: &'a IntentSink,
    pub(crate) fields: &'a Fields,
    pub(crate) records: &'a dyn RecordBackend,
}
