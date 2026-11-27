use terminal::MouseButton;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum DragState {
    Idle,
    Selecting,
    ReportingToProgram(MouseButton),
    ScrollbarThumb { grab: f32 },
}
