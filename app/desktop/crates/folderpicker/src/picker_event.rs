#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PickerEvent {
    Cancelled,
    Chosen(String),
}
