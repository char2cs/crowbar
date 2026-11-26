use gpui::EntityId;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct TabDrag {
    pub(crate) strip: EntityId,
}
