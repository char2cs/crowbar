#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Event {
    Title(String),
    ResetTitle,
    Bell,
    ClipboardStore(String),
    Exit(i32),
}
