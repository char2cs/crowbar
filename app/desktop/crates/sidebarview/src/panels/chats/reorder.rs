mod drag_end;
mod drag_line;
mod drag_state;
mod drop_hint;
mod drop_line;
mod drop_refusal;
mod line_box;
mod line_slide;
mod row_drag;
mod row_draggable;
mod row_ghost;

pub(crate) use drag_state::DragState;
pub(crate) use row_drag::{RowDrag, tracked};
pub(crate) use row_draggable::draggable;
pub(crate) use row_ghost::{GhostBuild, RowGhost};

use gpui::{Context, Window, px};
use sidebar::{Command, DragArm, DropMode, RowId};

use crate::panels::chats::reorder::drop_hint::DropHint;
use crate::sidebar::DragPoint;
use crate::sidebar::DragPress;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn press_row(
        &mut self,
        press: DragPress,
    ) {
        self.drag.press = Some(press);
    }

    pub fn preview_drag(
        &mut self,
        (subject, target, mode): (RowId, RowId, DropMode),
        cx: &mut Context<'_, Self>,
    ) {
        self.drag.lifted = Some(subject);
        self.drag.hint = Some(DropHint { row: target, mode });
        self.aim_line(cx);
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
            self.aim_line(cx);
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
        let pressed = self
            .drag
            .press
            .as_ref()
            .is_some_and(|press| press.row == subject);
        self.end_drag(cx);
        let clicked_in_place = !lifted && pressed && subject == target;
        if clicked_in_place {
            return self.open_row(subject, cx);
        }
        if !lifted {
            return;
        }
        let Some(hint) = hint else {
            return self.say_refused((&subject, &target), cx);
        };
        let command = Command::Drop {
            row: subject,
            target,
            mode: hint.mode,
        };
        self.send(command, cx);
    }

    pub(crate) fn pull_edge(
        &mut self,
        pull: f32,
        cx: &mut Context<'_, Self>,
    ) {
        if (pull - self.drag.pull).abs() < f32::EPSILON {
            return;
        }
        self.drag.pull = pull;
        cx.notify();
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
        let handle = self.scrolls.of_space(&project);
        let mut offset = handle.offset();
        offset.y -= px(self.drag.pull);
        handle.set_offset(offset);
        window.request_animation_frame();
    }

    pub(crate) fn release_pointer(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.end_resize(cx);
        self.release_drag(cx);
    }

    fn release_drag(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.drag.press.is_some() || self.drag.lifted.is_some() {
            self.end_drag(cx);
        }
        if self.file_drag.is_live() {
            self.end_file_drag(cx);
        }
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
        let moved = DragArm::at((press.at.x.into(), press.at.y.into()))
            .started((point.at.x.into(), point.at.y.into()));
        if moved {
            self.drag.lifted = Some(point.subject.clone());
        }
        moved
    }

    fn hint_for(
        &mut self,
        point: &DragPoint,
    ) -> Option<DropHint> {
        let project = self.data.active_project()?;
        let viewport = self.scrolls.of_space(&project).bounds();
        if !viewport.contains(&point.at) {
            return None;
        }
        let folded = |row: &RowId| self.tree.folded().any(|id| id == row);
        DropHint::resolve(&self.data.tree(&project).rows, point, folded)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};

    use super::*;
    use crate::test_support::prelude::*;

    const ROW_TOP: f32 = 200.0;
    const BELOW_THE_TREE: f32 = 790.0;

    fn over_at(
        rows: (&str, &str),
        top: f32,
        at: (f32, f32),
    ) -> DragPoint {
        let ((subject, target), (x, y)) = (rows, at);
        DragPoint {
            subject: RowId::from(subject),
            target: RowId::from(target),
            bounds: Bounds::new(point(px(0.0), px(top)), size(px(200.0), px(36.0))),
            at: point(px(x), px(top + y)),
        }
    }

    fn over(
        subject: &str,
        target: &str,
        at: (f32, f32),
    ) -> DragPoint {
        over_at((subject, target), ROW_TOP, at)
    }

    fn press(row: &str) -> DragPress {
        DragPress {
            row: RowId::from(row),
            at: point(px(0.0), px(ROW_TOP)),
        }
    }

    #[gpui::test]
    fn a_drop_after_the_threshold_sends_the_placement(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "folder", (20.0, 18.0)), cx);
            this.drop_on((RowId::from("chat"), RowId::from("folder")), cx);
        });
        let want = Command::Drop {
            row: RowId::from("chat"),
            target: RowId::from("folder"),
            mode: DropMode::Into,
        };
        assert_eq!(recorder.sent(), vec![want]);
    }

    #[gpui::test]
    fn a_release_below_the_threshold_is_not_a_drop(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "folder", (2.0, 2.0)), cx);
            this.drop_on((RowId::from("chat"), RowId::from("folder")), cx);
        });
        assert_eq!(recorder.sent(), vec![]);
    }

    #[gpui::test]
    fn a_release_below_the_threshold_on_the_pressed_row_is_a_click_that_opens_it(
        cx: &mut TestAppContext
    ) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "chat", (5.0, 0.0)), cx);
            this.drop_on((RowId::from("chat"), RowId::from("chat")), cx);
        });
        assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("chat"))]);
    }

    #[gpui::test]
    fn the_gallery_preview_marks_the_source_and_the_target(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let (subject, target) = (RowId::from("chat"), RowId::from("folder"));
        sidebar.update(cx, |this, cx| {
            this.preview_drag((subject.clone(), target.clone(), DropMode::After), cx);
            assert!(this.drag.is_lifted(&subject), "source");
            assert_eq!(this.drag.mode_on(&target), Some(DropMode::After), "target");
        });
    }

    #[gpui::test]
    fn a_release_anywhere_ends_a_drag_that_never_reached_a_row(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "folder", (20.0, 18.0)), cx);
            assert!(this.drag.lifted.is_some(), "lifted");
            this.release_drag(cx);
            assert!(this.drag.lifted.is_none(), "dropped elsewhere");
            assert!(this.drag.press.is_none(), "press forgotten");
        });
    }

    #[gpui::test]
    fn a_row_scrolled_out_of_the_tree_is_no_drop_target(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "folder", (20.0, 18.0)), cx);
            let hidden = over_at(("chat", "folder"), BELOW_THE_TREE, (20.0, 4.0));
            this.drag_over(&hidden, cx);
            assert_eq!(this.drag.hint, None);
        });
    }

    #[gpui::test]
    fn hovering_selects_a_hint_and_ending_clears_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.press_row(press("chat"));
            this.drag_over(&over("chat", "folder", (20.0, 18.0)), cx);
            let target = RowId::from("folder");
            assert_eq!(this.drag.mode_on(&target), Some(DropMode::Into), "hint");
            this.end_drag(cx);
            assert_eq!(this.drag.mode_on(&target), None, "cleared");
        });
    }
}
