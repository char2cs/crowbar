#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum MergeWork {
    #[default]
    Idle,
    Committing,
    Merging,
}
