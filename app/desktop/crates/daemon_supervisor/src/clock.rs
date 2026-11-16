use std::time::SystemTime;

use tokio::time::Instant;

#[derive(Clone, Copy)]
pub(crate) struct Clock {
    pub(crate) instant: fn() -> Instant,
    pub(crate) wall: fn() -> SystemTime,
}
