use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::logging::log_store::LogStore;

pub(crate) struct RotatingLog {
    store: Arc<dyn LogStore>,
    path: PathBuf,
    writer: Box<dyn Write + Send>,
    written: u64,
    max_bytes: u64,
    generations: u32,
}

impl RotatingLog {
    pub(crate) fn open(
        store: Arc<dyn LogStore>,
        path: &Path,
        limits: (u64, u32),
    ) -> io::Result<Self> {
        let file = store.open_append(path)?;
        Ok(Self {
            store,
            path: path.to_path_buf(),
            writer: file.writer,
            written: file.length,
            max_bytes: limits.0,
            generations: limits.1,
        })
    }

    fn rotate(&mut self) -> io::Result<()> {
        self.writer.flush()?;
        for generation in (1..=self.generations).rev() {
            let target = generation_path(&self.path, generation);
            let source = match generation {
                1 => self.path.clone(),
                older => generation_path(&self.path, older - 1),
            };
            self.store.rename_over(&source, &target)?;
        }
        self.writer = self.store.open_append(&self.path)?.writer;
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
        let count = self.writer.write(buf)?;
        self.written = self
            .written
            .saturating_add(u64::try_from(count).unwrap_or(u64::MAX));
        Ok(count)
    }

    fn flush(&mut self) -> io::Result<()> {
        self.writer.flush()
    }
}

fn generation_path(
    path: &Path,
    generation: u32,
) -> PathBuf {
    let mut name = path.as_os_str().to_owned();
    name.push(format!(".{generation}"));
    PathBuf::from(name)
}

#[cfg(test)]
mod tests {
    use std::io::Write;

    use super::*;
    use crate::logging::test_support::InMemoryLogStore;

    fn log_in(
        store: &Arc<InMemoryLogStore>,
        name: &str,
        limits: (u64, u32),
    ) -> RotatingLog {
        let shared: Arc<dyn LogStore> = Arc::<InMemoryLogStore>::clone(store);
        RotatingLog::open(shared, Path::new(name), limits).unwrap()
    }

    fn text(
        store: &InMemoryLogStore,
        path: &Path,
    ) -> String {
        store.text(path).unwrap_or_default()
    }

    #[test]
    fn a_full_file_moves_to_the_next_generation() {
        let store = Arc::new(InMemoryLogStore::default());
        let path = Path::new("crowbar.log");
        let mut log = log_in(&store, "crowbar.log", (10, 2));
        for line in ["aaaaaa\n", "bbbbbb\n", "cccccc\n", "dddddd\n"] {
            log.write_all(line.as_bytes()).unwrap();
        }
        log.flush().unwrap();
        assert_eq!(text(&store, path), "dddddd\n");
        assert_eq!(text(&store, &generation_path(path, 1)), "cccccc\n");
        assert_eq!(text(&store, &generation_path(path, 2)), "bbbbbb\n");
        assert!(
            store.text(&generation_path(path, 3)).is_none(),
            "only two generations are kept"
        );
    }

    #[test]
    fn a_reopened_log_appends_and_one_long_line_is_written_whole() {
        let store = Arc::new(InMemoryLogStore::default());
        let path = Path::new("crowbar.log");
        log_in(&store, "crowbar.log", (10, 1))
            .write_all(b"aaaaaa\n")
            .unwrap();
        let mut log = log_in(&store, "crowbar.log", (10, 1));
        log.write_all(b"bbbbbb\n").unwrap();
        assert_eq!(text(&store, &generation_path(path, 1)), "aaaaaa\n");
        assert_eq!(text(&store, path), "bbbbbb\n");
        let mut tiny = log_in(&store, "tiny.log", (4, 1));
        tiny.write_all(b"much longer than four\n").unwrap();
        assert_eq!(
            text(&store, Path::new("tiny.log")),
            "much longer than four\n"
        );
    }
}
