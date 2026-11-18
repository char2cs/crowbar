//! Logging and crash reports of the Crowbar shell.
//!
//! [`start`] installs the `tracing` subscriber, which writes to a size-rotated log file in the
//! shell data directory (and to stderr), and a panic hook that leaves a report file next to
//! it. [`LogPaths`] names those files so the shell can offer "Open log". [`Bundle`] zips the
//! logs, the daemon's log and live daemon dumps into one file the user can attach to a report.

mod bundle;
mod diagnostics;
mod error;
mod file_store;
mod logging;
#[cfg(test)]
mod test_support;

pub use bundle::Bundle;
pub use diagnostics::start;
pub use error::Error;
pub use logging::LogPaths;
