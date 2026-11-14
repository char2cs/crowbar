use std::io::{Read, Seek, SeekFrom};
use std::path::Path;
use std::time::Duration;

const LOG_TAIL_BYTES: u64 = 64 * 1024;
const LOG_TAIL_LINES: usize = 40;
const SPAWN_MARKER: &str = "=== daemon spawned";

pub(crate) fn last_run_tail(log: &str, max_lines: usize) -> String {
    let run = log
        .rfind(SPAWN_MARKER)
        .map_or(log, |at| log.get(at..).unwrap_or(log));
    let lines: Vec<&str> = run.lines().collect();
    lines
        .get(lines.len().saturating_sub(max_lines)..)
        .unwrap_or_default()
        .join("\n")
}

fn read_tail(path: &Path) -> std::io::Result<String> {
    let mut file = std::fs::File::open(path)?;
    let len = file.metadata()?.len();
    file.seek(SeekFrom::Start(len.saturating_sub(LOG_TAIL_BYTES)))?;
    let mut bytes = Vec::new();
    file.read_to_end(&mut bytes)?;
    Ok(String::from_utf8_lossy(&bytes).into_owned())
}

pub(crate) fn crash_summary(deaths: usize, window: Duration) -> String {
    format!(
        "The backend exited {deaths} times within {} minutes.",
        window.as_secs() / 60
    )
}

pub(crate) fn crash_report(summary: &str, log: &Path) -> String {
    let tail = match read_tail(log) {
        Ok(text) => last_run_tail(&text, LOG_TAIL_LINES),
        Err(e) => format!("could not read {}: {e}", log.display()),
    };
    format!(
        "Crowbar daemon on {} {}\n{summary}\n\n{tail}",
        std::env::consts::OS,
        std::env::consts::ARCH
    )
}

#[cfg(test)]
#[path = "crash_report_tests.rs"]
mod tests;
