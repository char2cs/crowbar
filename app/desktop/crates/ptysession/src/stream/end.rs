#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum End {
    ViewGone,
    Exited,
    Lost,
    Gone,
}
