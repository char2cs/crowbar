use std::sync::Arc;

use crate::byte_source::ByteSource;
use crate::clock::Clock;

pub(crate) struct Ports {
    pub(crate) clock: Arc<dyn Clock>,
    pub(crate) bytes: Arc<dyn ByteSource>,
}
