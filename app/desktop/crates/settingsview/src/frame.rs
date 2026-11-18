//! What every drawing function of the view needs.

use gpui::{FocusHandle, ScrollHandle};
use settings::{RecordBackend, SettingsState};

use crate::fields::Fields;
use crate::intent_sink::IntentSink;

#[derive(Clone, Copy)]
pub(crate) struct Frame<'a> {
    pub(crate) state: &'a SettingsState,
    pub(crate) sink: &'a IntentSink,
    pub(crate) fields: &'a Fields,
    pub(crate) records: &'a dyn RecordBackend,
    pub(crate) focus: &'a FocusHandle,
    pub(crate) scroll: &'a ScrollHandle,
}
