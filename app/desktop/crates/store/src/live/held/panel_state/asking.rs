#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) enum Asking {
    #[default]
    NotYet,
    Sent,
}
