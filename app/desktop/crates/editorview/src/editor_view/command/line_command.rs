use editor::Vertical;

#[derive(Clone, Copy)]
pub(in crate::editor_view) enum LineCommand {
    Move(Vertical),
    Copy(Vertical),
    Delete,
}
