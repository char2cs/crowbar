/// Whether a tab's file can be written, as develop's `useWorkspaceReadOnly`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum TabAccess {
    /// An ordinary tab.
    #[default]
    Writable,
    /// The file's branch is locked: a lock follows the label and the tooltip says why.
    ReadOnly,
}
