use std::panic::{self, PanicHookInfo};
use std::path::Path;

use tracing_subscriber::EnvFilter;

use crate::crash_report::CrashReport;
use crate::error::Error;
use crate::log_paths::LogPaths;
use crate::log_sink::LogSink;
use crate::rotating_log::RotatingLog;

const MAX_LOG_BYTES: u64 = 5 * 1024 * 1024;
const KEPT_GENERATIONS: u32 = 3;
const DEFAULT_FILTER: &str = "info";

/// Installs logging and the crash hook; the log goes to `paths` and to stderr, filtered by
/// `RUST_LOG` (default `info`). A panic anywhere writes `crash-<time>.txt` beside the log and
/// then runs the previous hook.
///
/// # Errors
/// [`Error::LogFile`] when the log cannot be opened, [`Error::AlreadyInstalled`] on a second call.
pub fn start(
    paths: &LogPaths,
    version: &str,
) -> Result<(), Error> {
    std::fs::create_dir_all(paths.dir()).map_err(|source| log_error(paths.dir(), source))?;
    let file = RotatingLog::open(&paths.current(), MAX_LOG_BYTES, KEPT_GENERATIONS)
        .map_err(|source| log_error(paths.dir(), source))?;
    let filter =
        EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new(DEFAULT_FILTER));
    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_ansi(false)
        .with_writer(LogSink::new(file))
        .try_init()
        .map_err(|_| Error::AlreadyInstalled)?;
    install_crash_hook(paths.dir(), version);
    Ok(())
}

fn log_error(
    dir: &Path,
    source: std::io::Error,
) -> Error {
    Error::LogFile {
        dir: dir.to_path_buf(),
        source,
    }
}

fn install_crash_hook(
    dir: &Path,
    version: &str,
) {
    let dir = dir.to_path_buf();
    let version = version.to_owned();
    let previous = panic::take_hook();
    panic::set_hook(Box::new(move |info| {
        record_panic(&report_for(info, &version), &dir);
        previous(info);
    }));
}

fn record_panic(
    report: &CrashReport,
    dir: &Path,
) {
    match report.write(dir) {
        Ok(path) => tracing::error!(report = %path.display(), "the shell panicked"),
        Err(error) => {
            tracing::error!(%error, "the shell panicked and the report could not be written");
        },
    }
}

fn report_for(
    info: &PanicHookInfo<'_>,
    version: &str,
) -> CrashReport {
    let message = info
        .payload()
        .downcast_ref::<&str>()
        .map(|text| (*text).to_owned())
        .or_else(|| info.payload().downcast_ref::<String>().cloned())
        .unwrap_or_else(|| "non-text panic payload".to_owned());
    CrashReport {
        version: version.to_owned(),
        message,
        location: info
            .location()
            .map_or_else(|| "unknown".to_owned(), ToString::to_string),
        backtrace: std::backtrace::Backtrace::force_capture().to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_log_file_error_names_the_directory() {
        let error = log_error(
            Path::new("/data/logs"),
            std::io::Error::from(std::io::ErrorKind::PermissionDenied),
        );
        assert!(error.to_string().contains("/data/logs"), "{error}");
    }
}
