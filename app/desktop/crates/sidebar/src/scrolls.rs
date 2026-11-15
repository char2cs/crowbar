//! The scroll positions the sidebar keeps: one per space, plus the Files and Changes panels.

use std::collections::HashMap;

use gpui::ScrollHandle;

use crate::project_id::ProjectId;

#[derive(Debug, Default)]
pub(crate) struct Scrolls {
    spaces: HashMap<ProjectId, ScrollHandle>,
    pub(crate) files: ScrollHandle,
    pub(crate) changes: ScrollHandle,
}

impl Scrolls {
    pub(crate) fn of_space(
        &mut self,
        project: &ProjectId,
    ) -> ScrollHandle {
        self.spaces.entry(project.clone()).or_default().clone()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px};

    use super::*;

    #[test]
    fn each_space_keeps_its_own_scroll_position() {
        let mut scrolls = Scrolls::default();
        let a = scrolls.of_space(&ProjectId::from("inv"));
        let b = scrolls.of_space(&ProjectId::from("other"));
        a.set_offset(point(px(0.0), px(-10.0)));
        assert_ne!(a.offset(), b.offset(), "separate");
        let again = scrolls.of_space(&ProjectId::from("inv"));
        assert_eq!(again.offset(), a.offset(), "remembered");
    }
}
