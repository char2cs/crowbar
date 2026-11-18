//! What the emulator tells its owner.

/// Something the program in the terminal asked the shell to do.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TerminalEvent {
    /// The program set the window title (raw, unsanitised).
    Title(String),
    /// The program asked to drop its title.
    ResetTitle,
    /// The bell rang.
    Bell,
    /// The program stored text in the clipboard (OSC 52).
    ClipboardStore(String),
    /// The session ended with this exit code.
    Exit(i32),
}
