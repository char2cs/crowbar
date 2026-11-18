/// A modifier key a [`Keystroke`](crate::Keystroke) can hold down.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Modifier {
    /// The platform primary modifier: Command on macOS, Control elsewhere.
    Primary,
    /// Shift.
    Shift,
    /// Alt or Option.
    Alt,
}
