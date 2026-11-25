#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Landing {
    /// The document shows; the pane's open-time marks can go.
    Shown,
    /// The user opened it just now and it could not be read: its tab closes and a toast says why.
    Refused(editor::OpenFailure),
    /// Nothing to do: the pane says why itself, or it closed meanwhile.
    Quiet,
}
