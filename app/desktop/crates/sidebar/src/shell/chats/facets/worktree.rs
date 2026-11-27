#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Worktree {
    #[default]
    Shared,
    Owned,
}
