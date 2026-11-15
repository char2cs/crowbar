/// What the picker asks of its host.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PickerEvent {
    /// The user dismissed the picker.
    Cancelled,
    /// The user chose the folder at this absolute path of the host's disk.
    Chosen(String),
}
