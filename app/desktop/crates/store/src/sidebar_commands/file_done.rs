//! How an action on the workspace's files reports back to the shell.

use crate::sidebar_commands::file_error::FileError;

/// Called once, on a runtime thread, with what the action produced or why it failed.
pub type FileDone<T> = Box<dyn FnOnce(Result<T, FileError>) + Send>;
