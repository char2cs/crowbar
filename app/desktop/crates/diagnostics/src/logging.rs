//! The shell's rotating log, its sinks and the crash report left beside it.

pub(crate) mod adapters;
mod crash_report;
mod log_file;
mod log_paths;
mod log_sink;
mod log_store;
mod rotating_log;
mod sink_writer;
#[cfg(test)]
mod test_support;

pub(crate) use adapters::DiskLogStore;
pub(crate) use crash_report::CrashReport;
pub use log_paths::LogPaths;
pub(crate) use log_sink::LogSink;
pub(crate) use rotating_log::RotatingLog;
