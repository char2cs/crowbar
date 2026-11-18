//! Why a terminal stream stopped.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum End {
    ViewGone,
    Exited,
    Lost,
}
