//! The desktop's file chooser, opened over a window of the shell.

use std::path::PathBuf;

use crate::chooser::FileChoice;
use crate::error::Result;
use crate::view::ParentWindow;

/// Receives the chosen paths once, on the platform thread; empty when the chooser is cancelled.
pub type ChosenFiles = Box<dyn FnOnce(Vec<PathBuf>) + Send + 'static>;

/// The native file chooser a backend opens.
pub trait FileChoosing {
    /// Opens a chooser for `choice` over `parent` and hands its answer to `answer`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a chooser over
    /// `parent`.
    fn choose_files(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()>;
}
