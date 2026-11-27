#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Landing {
    Shown,
    Refused(editor::OpenFailure),
    Quiet,
}
