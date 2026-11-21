//! What `Enter` does in a text area.

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum EnterKey {
    #[default]
    Newline,
    Submit,
}
