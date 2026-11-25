//! The desktop settings the engine's own pages follow, offered to the shell's widgets too.

use crate::backend::desktop::caret_blink::CaretReport;
use crate::backend::desktop::color_scheme::ColorScheme;
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

    /// Draws the desktop's own menus, dialogs and choosers in `scheme` from now on.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()>;
}
