use std::io;
use std::os::unix::fs::MetadataExt;
use std::path::Path;

pub(crate) fn file_owner(path: &Path) -> io::Result<u32> {
    Ok(std::fs::symlink_metadata(path)?.uid())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_filesystem_root_belongs_to_the_superuser() {
        assert_eq!(file_owner(Path::new("/")).unwrap(), 0);
    }

    #[test]
    fn a_missing_file_is_a_not_found_error() {
        let error = file_owner(Path::new("/nonexistent/crowbar.sock")).unwrap_err();
        assert_eq!(error.kind(), io::ErrorKind::NotFound);
    }
}
