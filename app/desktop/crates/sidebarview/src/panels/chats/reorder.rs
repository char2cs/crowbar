//! The sidebar's side of a row drag: arming, hovering, dropping.

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
use sidebar::{Command, DragArm, RowId};

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
    use super::*;
    use crate::test_support::drag::{drag, over_at};
    use crate::test_support::prelude::*;
    use sidebar::DropMode;

    const BELOW_THE_TREE: f32 = 790.0;

    fn dropped(
        cx: &mut TestAppContext,
        rows: (&str, &str),
        at: (f32, f32),
    ) -> Vec<Command> {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            drag(this, rows, at, cx);
            this.drop_on((RowId::from(rows.0), RowId::from(rows.1)), cx);
        });
        recorder.sent()
    }

    #[gpui::test]
    fn a_release_drops_after_the_threshold_and_clicks_before_it(cx: &mut TestAppContext) {
        let into = Command::Drop {
            row: RowId::from("working"),
            target: RowId::from("folder"),
            mode: DropMode::Into,
        };
        let cases = [
            (("working", "folder"), (20.0, 18.0), vec![into]),
            (("working", "folder"), (2.0, 2.0), Vec::new()),
            (
                ("working", "working"),
                (5.0, 0.0),
                vec![Command::OpenRow(RowId::from("working"))],
            ),
        ];
        for (rows, at, want) in cases {
            assert_eq!(dropped(cx, rows, at), want, "{rows:?} at {at:?}");
        }
    }

    #[gpui::test]
    fn a_release_anywhere_ends_a_drag_that_never_reached_a_row(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            drag(this, ("working", "folder"), (20.0, 18.0), cx);
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
            drag(this, ("working", "folder"), (20.0, 18.0), cx);
            let hidden = over_at(("working", "folder"), BELOW_THE_TREE, (20.0, 4.0));
            this.drag_over(&hidden, cx);
            assert_eq!(this.drag.hint, None);
        });
    }
}
