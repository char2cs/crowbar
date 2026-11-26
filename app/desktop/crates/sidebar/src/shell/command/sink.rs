//! Where the sidebar sends its commands.

use crate::error::Error;
use crate::shell::command::Command;

/// Implemented by the shell. A refusal comes back as [`Error::Refused`] so the view can keep an
/// inline input open with the reason.
pub trait Sink {
    /// Performs `command`.
    ///
    /// # Errors
    ///
    /// [`Error::Refused`] when the shell rejects the command; the sidebar keeps the input open and
    /// shows the reason.
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error>;
}
