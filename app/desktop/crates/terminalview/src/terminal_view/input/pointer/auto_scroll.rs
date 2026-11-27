use gpui::Task;
use terminal::DragEdge;

pub(crate) struct AutoScroll {
    pub(crate) edge: DragEdge,
    pub(crate) _task: Task<()>,
}
