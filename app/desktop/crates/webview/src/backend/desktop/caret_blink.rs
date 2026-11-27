use std::sync::Arc;
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaretBlink {
    Steady,
    Every(Duration),
}

pub type CaretReport = Arc<dyn Fn(CaretBlink) + Send + Sync + 'static>;
