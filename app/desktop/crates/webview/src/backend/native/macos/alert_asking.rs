//! The shell's message dialogs on macOS, which it draws itself.

use crate::alert::AlertRequest;
use crate::backend::alert_asking::{AlertAsking, ChosenButton};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::{Error, Result};
use crate::view::ParentWindow;

// PLATFORM: macos the shell keeps its drawn dialogs until NSAlert is wired, so the web view backend asks none.
fn refused() -> Error {
    Error::UnsupportedParent("macOS dialogs are drawn by the shell")
}

impl AlertAsking for AppkitPlatform {
    fn ask_alert(
        &self,
        _parent: ParentWindow,
        _request: AlertRequest,
        _answer: ChosenButton,
    ) -> Result<()> {
        Err(refused())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_backend_leaves_dialogs_to_the_shell() {
        assert!(matches!(refused(), Error::UnsupportedParent(_)));
    }
}
