#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    PreferencesChanged,
    Held(Vec<sidebar::RowId>),
}
