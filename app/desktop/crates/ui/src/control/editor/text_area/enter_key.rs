#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum EnterKey {
    #[default]
    Newline,
    Submit,
}
