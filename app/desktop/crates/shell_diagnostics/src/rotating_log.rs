use std::fs::{File, OpenOptions};
use std::io::{self, Write};
use std::path::{Path, PathBuf};

#[derive(Debug)]
pub(crate) struct RotatingLog {
    path: PathBuf,
    file: File,
    written: u64,
    max_bytes: u64,
    generations: u32,
}

impl RotatingLog {
    pub(crate) fn open(
        path: &Path,
        max_bytes: u64,
        generations: u32,
    ) -> io::Result<Self> {
        let file = open_append(path)?;
        let written = file.metadata()?.len();
        Ok(Self {
            path: path.to_path_buf(),
            file,
            written,
            max_bytes,
            generations,
        })
    }

    fn rotate(&mut self) -> io::Result<()> {
        self.file.flush()?;
        for generation in (1..=self.generations).rev() {
            let target = generation_path(&self.path, generation);
            let source = match generation {
                1 => self.path.clone(),
                older => generation_path(&self.path, older - 1),
            };
            move_over(&source, &target)?;
        }
        self.file = open_append(&self.path)?;
        self.written = 0;
        Ok(())
    }
}

impl Write for RotatingLog {
    fn write(
        &mut self,
        buf: &[u8],
    ) -> io::Result<usize> {
        let incoming = u64::try_from(buf.len()).unwrap_or(u64::MAX);
        if self.written > 0 && self.written.saturating_add(incoming) > self.max_bytes {
            self.rotate()?;
        }
        let count = self.file.write(buf)?;
        self.written = self
            .written
            .saturating_add(u64::try_from(count).unwrap_or(u64::MAX));
        Ok(count)
    }

    fn flush(&mut self) -> io::Result<()> {
        self.file.flush()
    }
}

fn open_append(path: &Path) -> io::Result<File> {
    OpenOptions::new().create(true).append(true).open(path)
}

fn generation_path(
    path: &Path,
    generation: u32,
) -> PathBuf {
    let mut name = path.as_os_str().to_owned();
    name.push(format!(".{generation}"));
    PathBuf::from(name)
}

fn move_over(
    source: &Path,
    target: &Path,
) -> io::Result<()> {
    match std::fs::rename(source, target) {
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(()),
        other => other,
    }
}

#[cfg(test)]
mod tests {
    use std::io::Write;

    use super::*;

    fn read(path: &Path) -> String {
        std::fs::read_to_string(path).unwrap_or_default()
    }

    #[test]
    fn a_full_file_moves_to_the_next_generation() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("crowbar.log");
        let mut log = RotatingLog::open(&path, 10, 2).unwrap();
        for line in ["aaaaaa\n", "bbbbbb\n", "cccccc\n", "dddddd\n"] {
            log.write_all(line.as_bytes()).unwrap();
        }
        log.flush().unwrap();
        assert_eq!(read(&path), "dddddd\n");
        assert_eq!(read(&generation_path(&path, 1)), "cccccc\n");
        assert_eq!(read(&generation_path(&path, 2)), "bbbbbb\n");
        assert!(
            !generation_path(&path, 3).exists(),
            "only two generations are kept"
        );
    }

    #[test]
    fn a_reopened_log_appends_and_one_long_line_is_written_whole() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("crowbar.log");
        RotatingLog::open(&path, 10, 1)
            .unwrap()
            .write_all(b"aaaaaa\n")
            .unwrap();
        let mut log = RotatingLog::open(&path, 10, 1).unwrap();
        log.write_all(b"bbbbbb\n").unwrap();
        assert_eq!(read(&generation_path(&path, 1)), "aaaaaa\n");
        assert_eq!(read(&path), "bbbbbb\n");
        let mut tiny = RotatingLog::open(&dir.path().join("tiny.log"), 4, 1).unwrap();
        tiny.write_all(b"much longer than four\n").unwrap();
        assert_eq!(
            read(&dir.path().join("tiny.log")),
            "much longer than four\n"
        );
    }
}
