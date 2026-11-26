//! The desktop's message dialog, asked over a window of the shell.

use crate::alert::AlertRequest;
use crate::error::Result;
use crate::view::ParentWindow;

/// Receives the pressed button's index once, on the platform thread; `None` when the dialog is
/// dismissed.
pub type ChosenButton = Box<dyn FnOnce(Option<usize>) + Send + 'static>;

/// The native message dialog a backend asks; the shell has no other way to confirm.
pub trait AlertAsking {
    /// Asks `request` over `parent` and hands the pressed button to `answer`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a dialog over
    /// `parent`; `answer` is then dropped unanswered.
    fn ask_alert(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()>;
}
