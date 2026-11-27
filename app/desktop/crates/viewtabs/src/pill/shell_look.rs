#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ShellLook {
    Idle,
    Active,
    Lifted,
    MergeTarget,
}
