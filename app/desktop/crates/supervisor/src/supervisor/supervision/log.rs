//! The daemon's captured log: its sinks, messages and the crash report.

mod adapters;
mod crash_report;
mod daemon_log;
mod log_message;
pub(crate) mod log_sink;

pub(crate) use crash_report::{crash_report, crash_summary};
pub use daemon_log::DaemonLog;
pub(crate) use daemon_log::MAX_LINE_BYTES;
