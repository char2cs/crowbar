//! The shell's file choosers on macOS, which gpui opens itself.

use crate::backend::file_choosing::{ChosenFiles, FileChoosing};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::chooser::FileChoice;
use crate::error::{Error, Result};
use crate::view::ParentWindow;

// PLATFORM: macos the shell keeps gpui's own NSOpenPanel, so the web view backend opens no chooser.
fn refused() -> Error {
    Error::UnsupportedParent("macOS files are chosen through gpui's NSOpenPanel")
}

impl FileChoosing for AppkitPlatform {
    fn choose_files(
        &self,
        _parent: ParentWindow,
        _choice: FileChoice,
        _answer: ChosenFiles,
    ) -> Result<()> {
        Err(refused())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_backend_leaves_choosing_files_to_gpui() {
        assert!(matches!(refused(), Error::UnsupportedParent(_)));
    }
}
