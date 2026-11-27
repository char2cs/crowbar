#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PaneLifecycle {
    KeepAlive,
    Parkable,
    DestroyOnHide,
}
