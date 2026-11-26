//! The desktop's context menu, popped up over a window of the shell.

use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

/// Receives the chosen entry once, on the platform thread: its index in the menu, then in each
/// submenu it lies in; `None` when the menu is dismissed.
pub type ChosenEntry = Box<dyn FnOnce(Option<Vec<usize>>) + Send + 'static>;

/// The native context menu a backend pops up.
pub trait MenuPopping {
    /// Pops `request` up over `parent` and hands the chosen entry to `answer`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a menu over
    /// `parent`.
    fn popup_menu(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()>;
}
