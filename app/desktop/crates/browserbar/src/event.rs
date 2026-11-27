#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    Go(String),
    Back,
    Forward,
    Reload,
    TogglePick,
}
