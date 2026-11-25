/// What a file chooser selects.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ChoiceKind {
    /// One file.
    File,
    /// One or more files.
    Files,
    /// One folder.
    Folder,
}
