//! The sidebar's side of a row drag: arming, hovering, dropping.

use gpui::{Context, Window, px};

use crate::command::Command;
use crate::drag_arm::drag_started;
use crate::drag_point::DragPoint;
use crate::drag_press::DragPress;
use crate::drop_hint::DropHint;
use crate::drop_mode::DropMode;
use crate::row_id::RowId;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn press_row(
        &mut self,
        press: DragPress,
    ) {
        self.drag.press = Some(press);
    }

    /// Shows the indicators of `subject` being dragged over `target`, as the gallery's drag
    /// state; the pointer is not involved.
    pub fn preview_drag(
        &mut self,
        (subject, target, mode): (RowId, RowId, DropMode),
        cx: &mut Context<'_, Self>,
    ) {
        self.drag.lifted = Some(subject);
        self.drag.hint = Some(DropHint { row: target, mode });
        cx.notify();
    }

    pub(crate) fn drag_over(
        &mut self,
        point: &DragPoint,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.arm_lift(point) {
            return;
        }
        let hint = self.hint_for(point);
        if hint != self.drag.hint {
            self.drag.hint = hint;
            cx.notify();
        }
    }

    pub(crate) fn drop_on(
        &mut self,
        (subject, target): (RowId, RowId),
        cx: &mut Context<'_, Self>,
    ) {
        let hint = self.drag.hint.clone().filter(|hint| hint.row == target);
        let lifted = self.drag.is_lifted(&subject);
        self.end_drag(cx);
        let Some(hint) = hint.filter(|_| lifted) else {
            return;
        };
        let command = Command::Drop {
            row: subject,
            target,
            mode: hint.mode,
        };
        self.send(command, cx);
    }

    pub(crate) fn scroll_edge(
        &mut self,
        window: &mut Window,
    ) {
        if self.drag.pull == 0.0 || self.drag.lifted.is_none() {
            return;
        }
        let Some(project) = self.data.active_project() else {
            return;
        };
        let handle = self.scroll_for(&project);
        let mut offset = handle.offset();
        offset.y -= px(self.drag.pull);
        handle.set_offset(offset);
        window.request_animation_frame();
    }

    pub(crate) fn end_drag(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.drag.end();
        cx.notify();
    }

    fn arm_lift(
        &mut self,
        point: &DragPoint,
    ) -> bool {
        if self.drag.lifted.is_some() {
            return true;
        }
        let Some(press) = self.drag.press.as_ref().filter(|p| p.row == point.subject) else {
            return false;
        };
        let moved = drag_started(
            (press.at.x.into(), press.at.y.into()),
            (point.at.x.into(), point.at.y.into()),
        );
        if moved {
            self.drag.lifted = Some(point.subject.clone());
        }
        moved
    }

    fn hint_for(
        &self,
        point: &DragPoint,
    ) -> Option<DropHint> {
        let project = self.data.active_project()?;
        DropHint::resolve(&self.data.tree(&project).rows, point)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};

    use super::*;
    use crate::test_support::prelude::*;

    fn over(
        subject: &str,
        target: &str,
        (x, y): (f32, f32),
    ) -> DragPoint {
        DragPoint {
            subject: RowId::from(subject),
            target: RowId::from(target),
            bounds: Bounds::new(point(px(0.0), px(0.0)), size(px(200.0), px(36.0))),
            at: point(px(x), px(y)),
        }
    }

    fn press(row: &str) -> DragPress {
        DragPress {
            row: RowId::from(row),
            at: point(px(0.0), px(0.0)),
        }
    }

    #[gpui::test]
    fn a_drop_after_the_threshold_sends_the_placement(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("working"));
            this.drag_over(&over("working", "folder", (20.0, 18.0)), cx);
            this.drop_on((RowId::from("working"), RowId::from("folder")), cx);
        });
        let want = Command::Drop {
            row: RowId::from("working"),
            target: RowId::from("folder"),
            mode: DropMode::Into,
        };
        assert_eq!(recorder.sent(), vec![want]);
    }

    #[gpui::test]
    fn a_release_below_the_threshold_is_not_a_drop(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("working"));
            this.drag_over(&over("working", "folder", (2.0, 2.0)), cx);
            this.drop_on((RowId::from("working"), RowId::from("folder")), cx);
        });
        assert_eq!(recorder.sent(), vec![]);
    }

    #[gpui::test]
    fn the_gallery_preview_marks_the_source_and_the_target(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        let (subject, target) = (RowId::from("working"), RowId::from("folder"));
        sidebar.update(cx, |this, cx| {
            this.preview_drag((subject.clone(), target.clone(), DropMode::After), cx);
            assert!(this.drag.is_lifted(&subject), "source");
            assert_eq!(this.drag.mode_on(&target), Some(DropMode::After), "target");
        });
    }

    #[gpui::test]
    fn hovering_selects_a_hint_and_ending_clears_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("working"));
            this.drag_over(&over("working", "folder", (20.0, 18.0)), cx);
            let target = RowId::from("folder");
            assert_eq!(this.drag.mode_on(&target), Some(DropMode::Into), "hint");
            this.end_drag(cx);
            assert_eq!(this.drag.mode_on(&target), None, "cleared");
        });
    }
}
