use std::fs::DirBuilder;
use std::io;
use std::os::unix::fs::DirBuilderExt;
use std::path::Path;

const PRIVATE_DIR_MODE: u32 = 0o700;

pub(crate) fn private_dir(dir: &Path) -> io::Result<()> {
    DirBuilder::new()
        .recursive(true)
        .mode(PRIVATE_DIR_MODE)
        .create(dir)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_directory_that_exists_is_left_alone() {
        private_dir(Path::new(env!("CARGO_MANIFEST_DIR"))).unwrap();
    }

    #[test]
    fn a_directory_that_cannot_be_made_is_an_error() {
        assert!(private_dir(Path::new("/proc/crowbar/run")).is_err());
    }
}
