//! The shell's context menus on macOS, which it draws itself.

use crate::backend::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::{Error, Result};
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

// PLATFORM: macos the shell keeps its drawn context menus until NSMenu is wired, so the web view backend pops none.
fn refused() -> Error {
    Error::UnsupportedParent("macOS context menus are drawn by the shell")
}

impl MenuPopping for AppkitPlatform {
    fn popup_menu(
        &self,
        _parent: ParentWindow,
        _request: MenuRequest,
        _answer: ChosenEntry,
    ) -> Result<()> {
        Err(refused())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_backend_leaves_context_menus_to_the_shell() {
        assert!(matches!(refused(), Error::UnsupportedParent(_)));
    }
}
