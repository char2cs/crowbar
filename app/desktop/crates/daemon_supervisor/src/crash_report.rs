use std::io::{Read, Seek, SeekFrom};
use std::path::Path;
use std::time::Duration;

const LOG_TAIL_BYTES: u64 = 64 * 1024;
const LOG_TAIL_LINES: usize = 40;
const SPAWN_MARKER: &str = "=== daemon spawned";

pub(crate) fn last_run_tail(
    log: &str,
    max_lines: usize,
) -> String {
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

pub(crate) fn crash_summary(
    deaths: usize,
    window: Duration,
) -> String {
    format!(
        "The backend exited {deaths} times within {} minutes.",
        window.as_secs() / 60
    )
}

pub(crate) fn crash_report(
    summary: &str,
    log: &Path,
    version: &str,
) -> String {
    let tail = match read_tail(log) {
        Ok(text) => last_run_tail(&text, LOG_TAIL_LINES),
        Err(e) => format!("could not read {}: {e}", log.display()),
    };
    format!(
        "Crowbar {version} on {} {}\n{summary}\n\n{tail}",
        std::env::consts::OS,
        std::env::consts::ARCH
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_crash_report_is_the_last_run_of_the_log() {
        let log = "=== daemon spawned (epoch 1)\n[err] old run\n=== daemon terminated (code=Some(2))\n\
               === daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n\
               === daemon terminated (code=Some(2))\n";
        assert_eq!(
            last_run_tail(log, 10),
            "=== daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n=== daemon terminated (code=Some(2))"
        );
        assert_eq!(
            last_run_tail(log, 2),
            "[err] goroutine 1\n=== daemon terminated (code=Some(2))"
        );
        assert_eq!(last_run_tail("no marker\nat all", 5), "no marker\nat all");
    }

    #[test]
    fn the_summary_names_the_deaths_counted_and_the_window() {
        assert_eq!(
            crash_summary(4, Duration::from_secs(600)),
            "The backend exited 4 times within 10 minutes."
        );
    }

    #[test]
    fn an_unreadable_log_is_said_so_in_the_report() {
        let report = crash_report("It died.", Path::new("/definitely/not/here.log"), "1.2.3");
        assert!(report.contains("It died."));
        assert!(report.contains("could not read /definitely/not/here.log"));
    }

    #[test]
    fn the_report_names_the_app_version_and_the_platform() {
        let report = crash_report("It died.", Path::new("/definitely/not/here.log"), "1.2.3");
        let first = report.lines().next().unwrap_or_default();
        assert_eq!(
            first,
            format!(
                "Crowbar 1.2.3 on {} {}",
                std::env::consts::OS,
                std::env::consts::ARCH
            )
        );
    }
}
