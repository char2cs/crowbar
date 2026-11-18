//! What every drawing function of the view needs.

mod element_id;
mod fields;
mod icon_stem;
pub(crate) mod spacing;

pub(crate) use element_id::element_id;
pub(crate) use fields::Fields;
pub(crate) use icon_stem::icon_named;

use gpui::{FocusHandle, ScrollHandle};
use settings::{RecordBackend, SettingsState};

use crate::intent::IntentSink;

#[derive(Clone, Copy)]
pub(crate) struct Frame<'a> {
    pub(crate) state: &'a SettingsState,
    pub(crate) sink: &'a IntentSink,
    pub(crate) fields: &'a Fields,
    pub(crate) records: &'a dyn RecordBackend,
    pub(crate) focus: &'a FocusHandle,
    pub(crate) scroll: &'a ScrollHandle,
}
