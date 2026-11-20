//! What the sidebar tells its owner.

/// Emitted by [`crate::Sidebar`] when something the owner persists or reacts to changed.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    /// Width, visibility, dock side, mode or folds changed: save the preferences.
    PreferencesChanged,
    /// A row was held for removal: it and every row beneath it, whose views should close.
    Held(Vec<sidebar::RowId>),
}
