use std::io::{Read, Seek, SeekFrom};
use std::path::Path;

const LOG_TAIL_BYTES: u64 = 64 * 1024;

pub(crate) fn read_tail(path: &Path) -> std::io::Result<String> {
    let mut file = std::fs::File::open(path)?;
    let len = file.metadata()?.len();
    file.seek(SeekFrom::Start(len.saturating_sub(LOG_TAIL_BYTES)))?;
    let mut bytes = Vec::new();
    file.read_to_end(&mut bytes)?;
    Ok(String::from_utf8_lossy(&bytes).into_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_small_file_is_read_whole() {
        let manifest = Path::new(env!("CARGO_MANIFEST_DIR")).join("Cargo.toml");
        let text = read_tail(&manifest).unwrap();
        assert!(text.contains("supervisor"), "{text}");
    }

    #[test]
    fn a_missing_file_is_an_error() {
        assert!(read_tail(Path::new("/nonexistent/daemon.log")).is_err());
    }
}
