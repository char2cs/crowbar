use std::rc::Rc;

use crate::alert::AlertRequest;
use crate::backend::{ChosenButton, Platform};
use crate::error::Result;
use crate::view::ParentWindow;

/// Asks the desktop's message dialog over a shell window, on the platform thread of a
/// [`Host`](crate::Host).
#[derive(Clone)]
pub struct NativeAlert {
    platform: Rc<dyn Platform>,
}

impl std::fmt::Debug for NativeAlert {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("NativeAlert").finish_non_exhaustive()
    }
}

impl NativeAlert {
    pub(crate) fn new(platform: Rc<dyn Platform>) -> Self {
        Self { platform }
    }

    /// Asks `request` over `parent`; `answer` hears the pressed button.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a dialog over
    /// `parent`.
    pub fn ask(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()> {
        self.platform.ask_alert(parent, request, answer)
    }
}

#[cfg(test)]
mod tests {
    use crate::backend::Call;
    use crate::backend::MockPlatform;
    use crate::host::Host;

    use super::*;

    #[test]
    fn a_host_s_dialog_is_asked_on_its_platform() {
        let mock = Rc::new(MockPlatform::default());
        let host = Host::with_platform(Rc::<MockPlatform>::clone(&mock));
        let request = AlertRequest {
            title: Some("Unsaved Changes".into()),
            message: "a.rs has unsaved changes.".into(),
            buttons: vec![],
        };
        let parent = ParentWindow::X11(4);
        host.native_alert()
            .ask(parent, request.clone(), Box::new(|_| ()))
            .unwrap();
        assert_eq!(mock.calls(), [Call::AskAlert { parent, request }]);
    }
}
