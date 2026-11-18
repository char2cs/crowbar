//! Whether a remote branch may be brought in.

/// A remote branch is protected, already a workspace, or free to import.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Availability {
    /// The branch can be selected.
    Importable,
    /// The branch already has a workspace.
    Imported,
    /// The branch is protected and never imported.
    Protected,
}
