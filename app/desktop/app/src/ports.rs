use std::sync::Arc;

use crate::byte_source::ByteSource;
use crate::clock::Clock;
use crate::preferences_store::PreferencesStore;

pub(crate) struct Ports {
    pub(crate) clock: Arc<dyn Clock>,
    pub(crate) bytes: Arc<dyn ByteSource>,
    pub(crate) preferences: Arc<dyn PreferencesStore>,
}
