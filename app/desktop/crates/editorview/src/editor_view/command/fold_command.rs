#[derive(Clone, Copy, PartialEq, Eq)]
pub(in crate::editor_view) enum FoldCommand {
    Fold,
    Unfold,
    FoldAll,
    UnfoldAll,
}
