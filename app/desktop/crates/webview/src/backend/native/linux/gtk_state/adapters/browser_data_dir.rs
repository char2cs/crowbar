use std::io::ErrorKind;
use std::path::Path;

use crate::error::{Error, Result};

pub(crate) fn remove(dir: &Path) -> Result<()> {
    match std::fs::remove_dir_all(dir) {
        Err(error) if error.kind() != ErrorKind::NotFound => {
            Err(Error::PlatformThread(error.to_string()))
        },
        _ => Ok(()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_directory_that_is_not_there_is_already_clean() {
        assert!(remove(Path::new("/nonexistent/crowbar-browser-data")).is_ok());
    }
}
