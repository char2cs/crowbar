#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    ChooseFolder,
    AddHost,
    MoveWindow,
}
