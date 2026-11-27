#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Lock {
    #[default]
    Unlocked,
    Locked,
}
