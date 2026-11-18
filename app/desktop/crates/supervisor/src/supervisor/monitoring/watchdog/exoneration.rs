#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Exoneration {
    Suspended,
    Answering,
    Busy,
}
