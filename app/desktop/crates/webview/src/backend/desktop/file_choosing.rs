use std::path::PathBuf;

use crate::chooser::FileChoice;
use crate::error::Result;
use crate::view::ParentWindow;

pub type ChosenFiles = Box<dyn FnOnce(Vec<PathBuf>) + Send + 'static>;

pub trait FileChoosing {
    fn choose_files(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()>;
}
