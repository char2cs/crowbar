use crate::shell::listing::folder_phase::FolderPhase;

pub trait FolderSource {
    fn folder_phase(&self) -> FolderPhase;
}
