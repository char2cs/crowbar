//! How loud an event is, as tracing's levels.

/// The level an [`crate::Event`] is logged at.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Level {
    /// Something failed that the user will notice.
    Error,
    /// Something failed that was recovered from or worked around.
    Warn,
    /// A step worth keeping in the log.
    Info,
    /// Detail for diagnosing a problem.
    Debug,
    /// Detail for following the code step by step.
    Trace,
}
