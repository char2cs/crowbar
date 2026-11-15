use std::io;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct CrashReport {
    pub(crate) version: String,
    pub(crate) message: String,
    pub(crate) location: String,
    pub(crate) backtrace: String,
}

impl CrashReport {
    pub(crate) fn render(&self) -> String {
        format!(
            "Crowbar {} on {} {}\npanic at {}\n{}\n\n{}\n",
            self.version,
            std::env::consts::OS,
            std::env::consts::ARCH,
            self.location,
            self.message,
            self.backtrace,
        )
    }

    pub(crate) fn write(
        &self,
        dir: &Path,
    ) -> io::Result<PathBuf> {
        let seconds = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_or(0, |elapsed| elapsed.as_secs());
        let path = dir.join(format!("crash-{seconds}.txt"));
        std::fs::write(&path, self.render())?;
        Ok(path)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn report() -> CrashReport {
        CrashReport {
            version: "1.2.3".into(),
            message: "index out of bounds".into(),
            location: "src/main.rs:7:5".into(),
            backtrace: "0: main".into(),
        }
    }

    #[test]
    fn the_report_names_version_location_and_message() {
        let text = report().render();
        for part in [
            "Crowbar 1.2.3",
            "src/main.rs:7:5",
            "index out of bounds",
            "0: main",
        ] {
            assert!(text.contains(part), "{part} missing from {text}");
        }
    }

    #[test]
    fn the_report_file_is_written_into_the_directory() {
        let dir = tempfile::tempdir().unwrap();
        let path = report().write(dir.path()).unwrap();
        assert_eq!(path.parent(), Some(dir.path()));
        assert!(
            std::fs::read_to_string(path)
                .unwrap()
                .contains("index out of bounds")
        );
    }
}
