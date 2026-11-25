/// What a message dialog's button does, which the desktop shows in its own way.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AlertRole {
    /// Leaves things as they are; `Escape` answers the same.
    Cancel,
    /// Goes ahead, the dialog's default.
    Accept,
    /// Goes ahead and loses something, styled as dangerous.
    Destructive,
}
