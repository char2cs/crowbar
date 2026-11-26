/// What the first-run screens ask of the shell.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    /// Open the folder picker for the first space.
    ChooseFolder,
    /// Open the Add Host dialog.
    AddHost,
    /// Move the window with the pointer, from a press on the top strip.
    MoveWindow,
}
