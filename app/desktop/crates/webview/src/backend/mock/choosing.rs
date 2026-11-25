use crate::backend::file_choosing::{ChosenFiles, FileChoosing};
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::chooser::FileChoice;
use crate::error::Result;
use crate::view::ParentWindow;

impl FileChoosing for MockPlatform {
    fn choose_files(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()> {
        self.record(Call::ChooseFiles { parent, choice });
        answer(self.chosen.borrow().clone());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::chooser::ChoiceKind;

    const FOLDER: FileChoice = FileChoice {
        title: "Select Folder",
        kind: ChoiceKind::Folder,
        filter: None,
    };

    #[test]
    fn a_chooser_is_recorded_and_answers_the_paths_it_was_given() {
        let mock = MockPlatform::default();
        mock.choose_paths(vec![PathBuf::from("/w/app")]);
        let heard = Arc::new(Mutex::new(Vec::new()));
        let sink = Arc::clone(&heard);
        let answer: ChosenFiles = Box::new(move |paths| sink.lock().unwrap().extend(paths));
        mock.choose_files(ParentWindow::X11(9), FOLDER, answer)
            .unwrap();
        let recorded = Call::ChooseFiles {
            parent: ParentWindow::X11(9),
            choice: FOLDER,
        };
        assert_eq!(mock.calls(), [recorded]);
        assert_eq!(*heard.lock().unwrap(), [PathBuf::from("/w/app")]);
    }
}
