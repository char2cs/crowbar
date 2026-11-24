use editor::Vertical;

#[derive(Clone, Copy)]
pub(super) enum LineCommand {
    Move(Vertical),
    Copy(Vertical),
    Delete,
}
