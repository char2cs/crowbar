//! Diagnostics-bundle export: everything a bug report needs, in one zip.
//!
//! Offline-first means there is no telemetry backend — the user is the
//! transport. This command gathers the daemon log (where panics and watchdog
//! goroutine dumps land), the app log, fresh goroutine/heap dumps from the
//! live daemon, and version metadata into a single zip in the user's
//! Downloads folder, and returns its path for the UI to show.

use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::Duration;

use http_body_util::BodyExt;
use tauri::{AppHandle, Manager, Runtime};

use crate::sidecar;

/// Per-dump budget: a wedged daemon must not stall the export — missing
/// dumps are recorded in meta.txt instead.
const DUMP_TIMEOUT: Duration = Duration::from_secs(10);

/// Writes `entries` (name → contents) as a deflate zip at `zip_path`.
pub fn write_bundle(zip_path: &Path, entries: &[(String, Vec<u8>)]) -> Result<(), String> {
    let file = std::fs::File::create(zip_path).map_err(|e| e.to_string())?;
    let mut zip = zip::ZipWriter::new(file);
    let options = zip::write::SimpleFileOptions::default()
        .compression_method(zip::CompressionMethod::Deflated);
    for (name, contents) in entries {
        zip.start_file(name.as_str(), options)
            .map_err(|e| e.to_string())?;
        zip.write_all(contents).map_err(|e| e.to_string())?;
    }
    zip.finish().map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn diagnostics_export<R: Runtime>(app: AppHandle<R>) -> Result<String, String> {
    let mut entries: Vec<(String, Vec<u8>)> = Vec::new();
    let mut meta = String::new();

    let pkg = app.package_info();
    meta.push_str(&format!(
        "app: {} {}\nos: {} {}\nexported_epoch: {}\n",
        pkg.name,
        pkg.version,
        std::env::consts::OS,
        std::env::consts::ARCH,
        epoch_secs(),
    ));

    // Daemon log generations — where panic traces, exit codes, and watchdog
    // goroutine dumps live.
    let daemon_log = sidecar::daemon_log_path();
    add_file(&mut entries, &mut meta, &daemon_log, "daemon.log");
    let mut rotated = daemon_log.as_os_str().to_owned();
    rotated.push(".1");
    add_file(
        &mut entries,
        &mut meta,
        &PathBuf::from(rotated),
        "daemon.log.1",
    );

    // The Rust host's own log(s).
    match app.path().app_log_dir() {
        Ok(dir) => match std::fs::read_dir(&dir) {
            Ok(items) => {
                for item in items.flatten() {
                    let path = item.path();
                    if path.extension().and_then(|e| e.to_str()) == Some("log") {
                        let name = format!(
                            "app/{}",
                            path.file_name().unwrap_or_default().to_string_lossy()
                        );
                        add_file(&mut entries, &mut meta, &path, &name);
                    }
                }
            }
            Err(e) => meta.push_str(&format!("app log dir unreadable: {e}\n")),
        },
        Err(e) => meta.push_str(&format!("app log dir unresolved: {e}\n")),
    }

    // Live-daemon state, best effort: version, then fresh goroutine + heap
    // dumps — the "what is it doing right now" half of the bundle.
    if let Some(socket) = app.state::<sidecar::SidecarHandle>().socket_path() {
        fetch_into(
            &mut entries,
            &mut meta,
            &socket,
            "/v0/health",
            "daemon-health.json",
        )
        .await;
        fetch_into(
            &mut entries,
            &mut meta,
            &socket,
            "/debug/pprof/goroutine?debug=2",
            "goroutine-dump.txt",
        )
        .await;
        fetch_into(
            &mut entries,
            &mut meta,
            &socket,
            "/debug/pprof/heap?debug=1",
            "heap-profile.txt",
        )
        .await;
    } else {
        meta.push_str("daemon socket unknown: live dumps skipped\n");
    }

    entries.push(("meta.txt".into(), meta.into_bytes()));

    let dest_dir = app
        .path()
        .download_dir()
        .unwrap_or_else(|_| std::env::temp_dir());
    let zip_path = dest_dir.join(format!("crowbar-diagnostics-{}.zip", epoch_secs()));
    write_bundle(&zip_path, &entries)?;
    Ok(zip_path.display().to_string())
}

fn epoch_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// Adds a file to the bundle, or records why it is absent — an incomplete
/// bundle with an honest manifest beats a failed export.
fn add_file(entries: &mut Vec<(String, Vec<u8>)>, meta: &mut String, path: &Path, name: &str) {
    match std::fs::read(path) {
        Ok(bytes) => entries.push((name.to_string(), bytes)),
        Err(e) => meta.push_str(&format!("{name} not included ({}): {e}\n", path.display())),
    }
}

async fn fetch_into(
    entries: &mut Vec<(String, Vec<u8>)>,
    meta: &mut String,
    socket: &PathBuf,
    request_path: &str,
    name: &str,
) {
    let fetched = tokio::time::timeout(DUMP_TIMEOUT, async {
        let resp = sidecar::http_get(socket, request_path).await?;
        if !resp.status().is_success() {
            return Err(format!("{} returned {}", request_path, resp.status()).into());
        }
        let body = resp.into_body().collect().await?.to_bytes();
        Ok::<Vec<u8>, Box<dyn std::error::Error + Send + Sync>>(body.to_vec())
    })
    .await;
    match fetched {
        Ok(Ok(bytes)) => entries.push((name.to_string(), bytes)),
        Ok(Err(e)) => meta.push_str(&format!("{name} not included: {e}\n")),
        Err(_) => meta.push_str(&format!(
            "{name} not included: timed out after {DUMP_TIMEOUT:?}\n"
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::write_bundle;
    use std::io::Read;

    #[test]
    fn write_bundle_roundtrips_all_entries() {
        let dir = std::env::temp_dir().join(format!("crowbar-diag-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let zip_path = dir.join("bundle.zip");

        let entries = vec![
            ("meta.txt".to_string(), b"app: crowbar 0.1.0\n".to_vec()),
            (
                "daemon.log".to_string(),
                b"=== daemon spawned\npanic: boom\n".to_vec(),
            ),
            ("app/Crowbar.log".to_string(), b"[INFO] hi\n".to_vec()),
        ];
        write_bundle(&zip_path, &entries).unwrap();

        let file = std::fs::File::open(&zip_path).unwrap();
        let mut archive = zip::ZipArchive::new(file).unwrap();
        assert_eq!(archive.len(), 3);
        for (name, expected) in &entries {
            let mut entry = archive.by_name(name).unwrap();
            let mut contents = Vec::new();
            entry.read_to_end(&mut contents).unwrap();
            assert_eq!(&contents, expected, "entry {name} must roundtrip");
        }

        std::fs::remove_dir_all(&dir).unwrap();
    }
}
