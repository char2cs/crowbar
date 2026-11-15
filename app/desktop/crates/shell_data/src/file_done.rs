//! How an action on the workspace's files reports back to the shell.

/// Called once, on a runtime thread, with what the action produced or why it failed in words fit
/// to show.
pub type FileDone<T> = Box<dyn FnOnce(Result<T, String>) + Send>;
