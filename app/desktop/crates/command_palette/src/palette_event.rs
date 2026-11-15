/// What the palette asks of the shell.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum PaletteEvent {
    /// The user chose the command with this id.
    Run(String),
    /// The user dismissed the palette.
    Close,
}
