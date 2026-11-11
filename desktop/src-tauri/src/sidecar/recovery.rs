//! The way out of a crash loop: once the supervisor stops respawning a daemon that keeps
//! dying, the window shows why and lets the user restart it, read its log or copy a report.

use std::io::{Read, Seek, SeekFrom};
use std::sync::atomic::Ordering;

use tauri::{AppHandle, Emitter, Manager, Runtime};

use super::{daemon_log_path, socket_path, spawn, supervisor, SidecarHandle};

/// How much of the daemon log's end is read for the report: its last run's final words.
const LOG_TAIL_BYTES: u64 = 64 * 1024;
const LOG_TAIL_LINES: usize = 40;

/// The end of the daemon log, or why it could not be read.
fn read_log_tail() -> String {
    let Some(path) = daemon_log_path() else {
        return "the daemon log's location is unknown".into();
    };
    let read = || -> std::io::Result<String> {
        let mut file = std::fs::File::open(&path)?;
        let len = file.metadata()?.len();
        file.seek(SeekFrom::Start(len.saturating_sub(LOG_TAIL_BYTES)))?;
        let mut bytes = Vec::new();
        file.read_to_end(&mut bytes)?;
        Ok(String::from_utf8_lossy(&bytes).into_owned())
    };
    match read() {
        Ok(text) => supervisor::last_run_tail(&text, LOG_TAIL_LINES),
        Err(e) => format!("could not read {}: {e}", path.display()),
    }
}

/// The crash panel's headline: how many times the daemon exited inside the restart window.
pub(super) fn crash_summary(deaths: usize, window: std::time::Duration) -> String {
    format!(
        "The backend exited {deaths} times within {} minutes.",
        window.as_secs() / 60
    )
}

/// What the window shows and copies for a crash loop: versions, then the last run's log.
pub(super) fn crash_report<R: Runtime>(app: &AppHandle<R>, summary: &str) -> String {
    let pkg = app.package_info();
    format!(
        "Crowbar {} on {} {}\n{summary}\n\n{}",
        pkg.version,
        std::env::consts::OS,
        std::env::consts::ARCH,
        read_log_tail()
    )
}

/// The crash-loop report, while the supervisor has given up on the daemon.
#[tauri::command]
pub fn daemon_crash_report<R: Runtime>(app: AppHandle<R>) -> Option<String> {
    app.state::<SidecarHandle>()
        .crash_report
        .lock()
        .unwrap()
        .clone()
}

/// Starts the daemon again on the user's word, with a fresh restart budget.
#[tauri::command]
pub async fn daemon_restart<R: Runtime>(app: AppHandle<R>) -> Result<(), String> {
    let state = app.state::<SidecarHandle>();
    if state.shutting_down.load(Ordering::SeqCst) {
        return Err("Crowbar is quitting".into());
    }
    if state.child.lock().unwrap().is_some() {
        return Err("the backend is already running".into());
    }
    let socket = socket_path().ok_or("the backend was never started")?;
    state.restart_budget.lock().unwrap().reset();
    state.crash_report.lock().unwrap().take();
    log::warn!("restarting crowbar daemon at the user's request");
    spawn(&app, socket).await.map_err(|e| e.to_string())?;
    let _ = app.emit("daemon:restarted", ());
    Ok(())
}

/// Shows the daemon log in the OS file manager.
#[tauri::command]
pub async fn reveal_daemon_log() -> Result<(), String> {
    let path = daemon_log_path().ok_or("the daemon log's location is unknown")?;
    crate::file_manager::reveal_in_finder(path.to_string_lossy().into_owned()).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_summary_names_the_deaths_counted_and_the_window() {
        assert_eq!(
            crash_summary(4, std::time::Duration::from_secs(600)),
            "The backend exited 4 times within 10 minutes."
        );
    }
}
