//! Logging and crash reports of the Crowbar shell.
//!
//! [`start`] installs the `tracing` subscriber, which writes to a size-rotated log file in the
//! shell data directory (and to stderr), and a panic hook that leaves a report file next to
//! it. [`LogPaths`] names those files so the shell can offer "Open log". [`Bundle`] zips the
//! logs, the daemon's log and live daemon dumps into one file the user can attach to a report.

mod bundle;
mod bundle_entry;
mod crash_report;
mod error;
mod log_paths;
mod log_sink;
mod rotating_log;
mod shell_diagnostics;
mod sink_writer;

pub use bundle::Bundle;
pub use error::Error;
pub use log_paths::LogPaths;
pub use shell_diagnostics::start;
