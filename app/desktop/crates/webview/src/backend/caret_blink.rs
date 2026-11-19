//! How the desktop asks text carets to blink, and the callback that hears it.

use std::sync::Arc;
use std::time::Duration;

/// How the desktop asks text carets to blink.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaretBlink {
    /// The caret stays shown.
    Steady,
    /// The caret shows, then hides, holding each phase this long.
    Every(Duration),
}

/// Receives the desktop's [`CaretBlink`] once when followed and again on every change, on the
/// platform thread.
pub type CaretReport = Arc<dyn Fn(CaretBlink) + Send + Sync + 'static>;
