mod entry;

use std::io::{Cursor, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use zip::write::SimpleFileOptions;
use zip::{CompressionMethod, ZipWriter};

use crate::bundle::entry::Entry;
use crate::error::Error;
use crate::file_store::DiskFiles;
use crate::file_store::FileStore;

const META_NAME: &str = "meta.txt";

/// A diagnostics zip under construction: files and dumps go in, the manifest `meta.txt` says
/// what is missing and why, so an incomplete bundle is still an honest one.
#[derive(Debug)]
pub struct Bundle {
    files: Arc<dyn FileStore>,
    version: String,
    entries: Vec<Entry>,
    notes: Vec<String>,
}

impl Bundle {
    /// An empty bundle for the app `version`.
    #[must_use]
    pub fn new(version: &str) -> Self {
        Self::on(Arc::new(DiskFiles::default()), version)
    }

    fn on(
        files: Arc<dyn FileStore>,
        version: &str,
    ) -> Self {
        Self {
            files,
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
        match self.files.read(path) {
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
        self.entries.push(Entry {
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
        let files = match self.files.list_files(dir) {
            Ok(files) => files,
            Err(error) => return self.add_missing(prefix, &format!("{}: {error}", dir.display())),
        };
        for path in &files {
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
        let archive = self.zip_into(seconds).map_err(&failed)?;
        self.files
            .write(&path, &archive)
            .map_err(|error| failed(Box::new(error)))?;
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
        seconds: u64,
    ) -> Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>> {
        let mut zip = ZipWriter::new(Cursor::new(Vec::new()));
        let options = SimpleFileOptions::default().compression_method(CompressionMethod::Deflated);
        let manifest = Entry {
            name: META_NAME.to_owned(),
            bytes: self.manifest(seconds).into_bytes(),
        };
        for entry in self.entries.iter().chain(std::iter::once(&manifest)) {
            zip.start_file(entry.name.as_str(), options)?;
            zip.write_all(&entry.bytes)?;
        }
        Ok(zip.finish()?.into_inner())
    }
}

#[cfg(test)]
mod tests {
    use std::io::Read;
    use std::time::{Duration, UNIX_EPOCH};

    use super::*;
    use crate::test_support::InMemoryFiles;

    fn read_entry(
        files: &InMemoryFiles,
        path: &Path,
        name: &str,
    ) -> Option<String> {
        let bytes = files.read(path).unwrap();
        let mut archive = zip::ZipArchive::new(Cursor::new(bytes)).unwrap();
        let mut entry = archive.by_name(name).ok()?;
        let mut text = String::new();
        entry.read_to_string(&mut text).unwrap();
        Some(text)
    }

    fn bundle_on(files: &Arc<InMemoryFiles>) -> Bundle {
        let shared: Arc<dyn FileStore> = Arc::<InMemoryFiles>::clone(files);
        Bundle::on(shared, "1.2.3")
    }

    #[test]
    fn files_dumps_and_the_manifest_round_trip_and_a_missing_directory_is_noted_not_fatal() {
        let files = Arc::new(InMemoryFiles::default().with("logs/crowbar.log", b"[INFO] hi\n"));
        let mut bundle = bundle_on(&files);
        bundle.add_directory("app", Path::new("logs"));
        bundle.add_directory("extra", Path::new("none"));
        bundle.add_bytes("goroutine-dump.txt", b"goroutine 1".to_vec());
        bundle.add_file("daemon.log", Path::new("absent.log"));
        bundle.add_missing("heap-profile.txt", "timed out");
        let now = UNIX_EPOCH + Duration::from_secs(42);
        let path = bundle.write(Path::new("out"), now).unwrap();
        assert!(path.ends_with("crowbar-diagnostics-42.zip"));
        let entry = |name| read_entry(&files, &path, name);
        assert_eq!(entry("app/crowbar.log").as_deref(), Some("[INFO] hi\n"));
        assert_eq!(entry("goroutine-dump.txt").as_deref(), Some("goroutine 1"));
        let meta = entry("meta.txt").unwrap();
        for part in [
            "app: crowbar 1.2.3",
            "exported_epoch: 42",
            "daemon.log not included",
            "heap-profile.txt not included: timed out",
            "extra not included",
        ] {
            assert!(meta.contains(part), "{part} missing from {meta}");
        }
    }
}
