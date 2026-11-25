pub(crate) mod byte_source;
pub(crate) mod clock;
pub(crate) mod preferences_store;

use std::sync::Arc;

use crate::shell::ports::byte_source::ByteSource;
use crate::shell::ports::clock::Clock;
use crate::shell::ports::preferences_store::PreferencesStore;

pub(crate) struct Ports {
    pub(crate) clock: Arc<dyn Clock>,
    pub(crate) bytes: Arc<dyn ByteSource>,
    pub(crate) preferences: Arc<dyn PreferencesStore>,
}
