//! The desktop settings the engine's own pages follow, offered to the shell's widgets too.

use crate::backend::desktop::caret_blink::CaretReport;
use crate::error::Result;

/// The desktop settings a backend reads and follows.
pub trait DesktopSettings {
    /// Reports the desktop's caret blink now and again whenever it changes.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    fn follow_caret(
        &self,
        report: CaretReport,
    ) -> Result<()>;
}
