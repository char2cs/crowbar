#[derive(Clone, Copy, PartialEq, Eq)]
pub(super) enum FoldCommand {
    Fold,
    Unfold,
    FoldAll,
    UnfoldAll,
}
