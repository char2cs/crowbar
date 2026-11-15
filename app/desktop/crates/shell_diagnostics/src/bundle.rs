use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use zip::write::SimpleFileOptions;
use zip::{CompressionMethod, ZipWriter};

use crate::bundle_entry::BundleEntry;
use crate::error::Error;

const META_NAME: &str = "meta.txt";

/// A diagnostics zip under construction: files and dumps go in, the manifest `meta.txt` says
/// what is missing and why, so an incomplete bundle is still an honest one.
#[derive(Debug)]
pub struct Bundle {
    version: String,
    entries: Vec<BundleEntry>,
    notes: Vec<String>,
}

impl Bundle {
    /// An empty bundle for the app `version`.
    #[must_use]
    pub fn new(version: &str) -> Self {
        Self {
            version: version.to_owned(),
            entries: Vec::new(),
            notes: Vec::new(),
        }
    }

    /// Adds the file at `path` as `name`, or notes why it is absent.
    pub fn add_file(
        &mut self,
        name: &str,
        path: &Path,
    ) {
        match std::fs::read(path) {
            Ok(bytes) => self.add_bytes(name, bytes),
            Err(error) => self.add_missing(name, &format!("{}: {error}", path.display())),
        }
    }

    /// Adds `bytes` as `name`.
    pub fn add_bytes(
        &mut self,
        name: &str,
        bytes: Vec<u8>,
    ) {
        self.entries.push(BundleEntry {
            name: name.to_owned(),
            bytes,
        });
    }

    /// Records in the manifest that `name` could not be included, and why.
    pub fn add_missing(
        &mut self,
        name: &str,
        reason: &str,
    ) {
        self.notes.push(format!("{name} not included: {reason}"));
    }

    /// Adds every regular file of `dir` as `<prefix>/<file name>`, skipping nothing silently.
    pub fn add_directory(
        &mut self,
        prefix: &str,
        dir: &Path,
    ) {
        let listing = match std::fs::read_dir(dir) {
            Ok(listing) => listing,
            Err(error) => return self.add_missing(prefix, &format!("{}: {error}", dir.display())),
        };
        let mut files: Vec<PathBuf> = listing.flatten().map(|item| item.path()).collect();
        files.sort();
        for path in files.iter().filter(|path| path.is_file()) {
            let name = path
                .file_name()
                .map_or_else(String::new, |n| n.to_string_lossy().into_owned());
            self.add_file(&format!("{prefix}/{name}"), path);
        }
    }

    /// Writes `crowbar-diagnostics-<unix seconds>.zip` into `dir` and returns its path.
    ///
    /// # Errors
    /// [`Error::Bundle`] when the directory or the zip cannot be written.
    pub fn write(
        &self,
        dir: &Path,
        now: SystemTime,
    ) -> Result<PathBuf, Error> {
        let seconds = now
            .duration_since(UNIX_EPOCH)
            .map_or(0, |elapsed| elapsed.as_secs());
        let path = dir.join(format!("crowbar-diagnostics-{seconds}.zip"));
        let failed = |source: Box<dyn std::error::Error + Send + Sync>| Error::Bundle {
            path: path.clone(),
            source,
        };
        std::fs::create_dir_all(dir).map_err(|e| failed(Box::new(e)))?;
        let file = std::fs::File::create(&path).map_err(|e| failed(Box::new(e)))?;
        self.zip_into(file, seconds).map_err(failed)?;
        Ok(path)
    }

    fn manifest(
        &self,
        seconds: u64,
    ) -> String {
        let mut text = format!(
            "app: crowbar {}\nos: {} {}\nexported_epoch: {seconds}\n",
            self.version,
            std::env::consts::OS,
            std::env::consts::ARCH,
        );
        for note in &self.notes {
            text.push_str(note);
            text.push('\n');
        }
        text
    }

    fn zip_into(
        &self,
        file: std::fs::File,
        seconds: u64,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let mut zip = ZipWriter::new(file);
        let options = SimpleFileOptions::default().compression_method(CompressionMethod::Deflated);
        let manifest = BundleEntry {
            name: META_NAME.to_owned(),
            bytes: self.manifest(seconds).into_bytes(),
        };
        for entry in self.entries.iter().chain(std::iter::once(&manifest)) {
            zip.start_file(entry.name.as_str(), options)?;
            zip.write_all(&entry.bytes)?;
        }
        zip.finish()?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::io::Read;
    use std::time::Duration;

    use super::*;

    fn read_entry(
        path: &Path,
        name: &str,
    ) -> Option<String> {
        let mut archive = zip::ZipArchive::new(std::fs::File::open(path).unwrap()).unwrap();
        let mut entry = archive.by_name(name).ok()?;
        let mut text = String::new();
        entry.read_to_string(&mut text).unwrap();
        Some(text)
    }

    #[test]
    fn files_dumps_and_the_manifest_round_trip() {
        let dir = tempfile::tempdir().unwrap();
        let logs = dir.path().join("logs");
        std::fs::create_dir(&logs).unwrap();
        std::fs::write(logs.join("crowbar.log"), "[INFO] hi\n").unwrap();
        let mut bundle = Bundle::new("1.2.3");
        bundle.add_directory("app", &logs);
        bundle.add_bytes("goroutine-dump.txt", b"goroutine 1".to_vec());
        bundle.add_file("daemon.log", &dir.path().join("absent.log"));
        bundle.add_missing("heap-profile.txt", "timed out");
        let path = bundle
            .write(
                &dir.path().join("out"),
                UNIX_EPOCH + Duration::from_secs(42),
            )
            .unwrap();
        assert!(path.ends_with("crowbar-diagnostics-42.zip"));
        assert_eq!(
            read_entry(&path, "app/crowbar.log").as_deref(),
            Some("[INFO] hi\n")
        );
        assert_eq!(
            read_entry(&path, "goroutine-dump.txt").as_deref(),
            Some("goroutine 1")
        );
        let meta = read_entry(&path, "meta.txt").unwrap();
        for part in [
            "app: crowbar 1.2.3",
            "exported_epoch: 42",
            "daemon.log not included",
            "heap-profile.txt not included: timed out",
        ] {
            assert!(meta.contains(part), "{part} missing from {meta}");
        }
    }

    #[test]
    fn a_missing_directory_is_noted_not_fatal() {
        let dir = tempfile::tempdir().unwrap();
        let mut bundle = Bundle::new("1");
        bundle.add_directory("app", &dir.path().join("none"));
        let path = bundle.write(dir.path(), UNIX_EPOCH).unwrap();
        assert!(
            read_entry(&path, "meta.txt")
                .unwrap()
                .contains("app not included")
        );
    }
}
