//! The shell's context menus on macOS, which it pops as `NSMenu`s from GPUI.

use crate::backend::desktop::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::{Error, Result};
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

// PLATFORM: macos the shell pops its NSMenus through gpui-component's native menu, so the web view backend pops none.
fn refused() -> Error {
    Error::UnsupportedParent("macOS context menus are popped by the shell")
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
