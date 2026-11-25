use std::rc::Rc;

use crate::backend::{ChosenFiles, Platform};
use crate::chooser::FileChoice;
use crate::error::Result;
use crate::view::ParentWindow;

/// Opens the desktop's file chooser over a shell window, on the platform thread of a
/// [`Host`](crate::Host).
#[derive(Clone)]
pub struct FileChooser {
    platform: Rc<dyn Platform>,
}

impl std::fmt::Debug for FileChooser {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("FileChooser").finish_non_exhaustive()
    }
}

impl FileChooser {
    pub(crate) fn new(platform: Rc<dyn Platform>) -> Self {
        Self { platform }
    }

    /// Opens a chooser for `choice` over `parent`; `answer` hears the chosen paths.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a chooser over
    /// `parent`.
    pub fn choose(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()> {
        self.platform.choose_files(parent, choice, answer)
    }
}

#[cfg(test)]
mod tests {
    use crate::backend::Call;
    use crate::backend::MockPlatform;
    use crate::chooser::ChoiceKind;
    use crate::host::Host;

    use super::*;

    #[test]
    fn a_host_s_chooser_opens_on_its_platform() {
        let mock = Rc::new(MockPlatform::default());
        let host = Host::with_platform(Rc::<MockPlatform>::clone(&mock));
        let choice = FileChoice {
            title: "Select Folder",
            kind: ChoiceKind::Folder,
            filter: None,
        };
        let parent = ParentWindow::X11(4);
        host.file_chooser()
            .choose(parent, choice, Box::new(|_| ()))
            .unwrap();
        assert_eq!(mock.calls(), [Call::ChooseFiles { parent, choice }]);
    }
}
