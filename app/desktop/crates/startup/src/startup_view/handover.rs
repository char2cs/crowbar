#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Handover {
    Waiting,
    Fading,
    Done,
}
