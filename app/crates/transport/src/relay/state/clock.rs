use std::sync::Arc;

use tokio::time::Instant;

pub(crate) type Clock = Arc<dyn Fn() -> Instant + Send + Sync>;
