//! Why a terminal stream stopped.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum StreamEnd {
    ViewGone,
    Exited,
    Lost,
}
