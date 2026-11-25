//! The desktop's message dialog, asked over a window of the shell.

use crate::alert::AlertRequest;
use crate::error::{Error, Result};
use crate::view::ParentWindow;

/// Receives the pressed button's index once, on the platform thread; `None` when the dialog is
/// dismissed.
pub type ChosenButton = Box<dyn FnOnce(Option<usize>) + Send + 'static>;

/// The native message dialog a backend asks; a backend without one refuses, and the shell draws
/// its own.
pub trait AlertAsking {
    /// Asks `request` over `parent` and hands the pressed button to `answer`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped, cannot hold a dialog over
    /// `parent` or has no dialog of its own.
    fn ask_alert(
        &self,
        _parent: ParentWindow,
        _request: AlertRequest,
        _answer: ChosenButton,
    ) -> Result<()> {
        Err(Error::UnsupportedParent(
            "this desktop's dialogs are drawn by the shell",
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Drawn;

    impl AlertAsking for Drawn {}

    #[test]
    fn a_backend_without_its_own_dialog_leaves_it_to_the_shell() {
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![],
        };
        let asked = Drawn.ask_alert(ParentWindow::AppKit(1), request, Box::new(|_| ()));
        assert!(matches!(asked, Err(Error::UnsupportedParent(_))));
    }
}
