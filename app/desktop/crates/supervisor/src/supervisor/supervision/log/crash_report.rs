use std::path::Path;
use std::time::Duration;

use crate::supervisor::supervision::log::adapters::read_tail;

const LOG_TAIL_LINES: usize = 40;
const SECONDS_PER_MINUTE: u64 = 60;
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

pub(crate) fn crash_summary(
    deaths: usize,
    window: Duration,
) -> String {
    format!(
        "The backend exited {deaths} times within {} minutes.",
        window.as_secs() / SECONDS_PER_MINUTE
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
        let cases = [
            (
                log,
                10,
                "=== daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n=== daemon terminated (code=Some(2))",
            ),
            (
                log,
                2,
                "[err] goroutine 1\n=== daemon terminated (code=Some(2))",
            ),
            ("no marker\nat all", 5, "no marker\nat all"),
        ];
        for (text, lines, expected) in cases {
            assert_eq!(last_run_tail(text, lines), expected, "{lines} lines");
        }
    }

    #[test]
    fn the_report_names_the_app_platform_and_deaths_and_says_when_the_log_is_unreadable() {
        let summary = crash_summary(4, Duration::from_secs(600));
        let report = crash_report(&summary, Path::new("/definitely/not/here.log"), "1.2.3");
        let (os, arch) = (std::env::consts::OS, std::env::consts::ARCH);
        let expected = format!(
            "Crowbar 1.2.3 on {os} {arch}\nThe backend exited 4 times within 10 minutes.\n\ncould not read /definitely/not/here.log"
        );
        assert!(report.starts_with(&expected), "{report}");
    }
}
