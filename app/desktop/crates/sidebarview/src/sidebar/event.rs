//! What the sidebar tells its owner.

/// Emitted by [`crate::Sidebar`] when something the owner persists or reacts to changed.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    /// Width, visibility, dock side, mode or folds changed: save the preferences.
    PreferencesChanged,
}
