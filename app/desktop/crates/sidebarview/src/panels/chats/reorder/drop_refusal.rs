//! Saying so when a row is released over one that refuses it, as develop's `toastRefusal` does,
//! so a refused drop does not look like one that silently failed to stick. A release back on the
//! dragged row is a cancel, not a refusal.

use gpui::Context;
use sidebar::{DropModes, RowId};
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(super) fn say_refused(
        &mut self,
        (subject, target): (&RowId, &RowId),
        cx: &mut Context<'_, Self>,
    ) {
        let Some(label) = self.refused_label(subject, target) else {
            return;
        };
        let content = ToastContent::new(ToastKind::Error, format!("Can't move {label} there"));
        self.toast(content, (None, ToastLifetime::Auto), cx);
    }

    fn refused_label(
        &self,
        subject: &RowId,
        target: &RowId,
    ) -> Option<String> {
        let project = self.data.active_project()?;
        let tree = self.data.tree(&project);
        let index = |id: &RowId| tree.rows.iter().position(|row| &row.id == id);
        let (dragged, over) = (index(subject)?, index(target)?);
        let refused =
            dragged != over && DropModes::allowed(&tree.rows, dragged, over) == DropModes::NONE;
        let label = Some(tree.rows.get(dragged)?.label.clone()).filter(|label| !label.is_empty());
        refused.then(|| label.unwrap_or_else(|| "this row".to_owned()))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};

    use crate::sidebar::{DragPoint, DragPress};
    use crate::test_support::prelude::*;

    fn dragged_onto(
        sidebar: &gpui::Entity<crate::sidebar::Sidebar>,
        (subject, target): (&str, &str),
        cx: &mut gpui::VisualTestContext,
    ) {
        let (subject, target) = (RowId::from(subject), RowId::from(target));
        let press = DragPress {
            row: subject.clone(),
            at: point(px(0.0), px(200.0)),
        };
        let over = DragPoint {
            subject: subject.clone(),
            target: target.clone(),
            bounds: Bounds::new(point(px(0.0), px(200.0)), size(px(200.0), px(36.0))),
            at: point(px(20.0), px(218.0)),
        };
        sidebar.update(cx, |this, cx| {
            this.press_row(press);
            this.drag_over(&over, cx);
            this.drop_on((subject, target), cx);
        });
    }

    fn toasts(
        sidebar: &gpui::Entity<crate::sidebar::Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> usize {
        let queue = sidebar.read_with(cx, |this, _| this.toasts.clone());
        queue.read_with(cx, |queue, _| queue.len())
    }

    #[gpui::test]
    fn a_release_over_a_row_that_refuses_the_drag_says_so_and_moves_nothing(
        cx: &mut TestAppContext
    ) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        dragged_onto(&sidebar, ("demo", "folder"), cx);
        assert_eq!(recorder.sent(), vec![], "nothing moves");
        assert_eq!(toasts(&sidebar, cx), 1, "a repo cannot go into a folder");
        dragged_onto(&sidebar, ("working", "folder"), cx);
        assert_eq!(toasts(&sidebar, cx), 1, "an allowed drop says nothing");
    }
}
