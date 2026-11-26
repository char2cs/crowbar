//! The sliding drop line, drawn above the whole window while a row is dragged.

use gpui::{
    AnchoredPositionMode, AnyElement, Context, IntoElement, ParentElement, Window, anchored,
    deferred, point, px,
};
use ui::{Motion, Surface, Theme, layer};

use crate::panels::chats::reorder::drop_line::drop_line;
use crate::panels::chats::reorder::line_box::LineBox;
use crate::panels::chats::reorder::line_slide::LineSlide;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn drop_line_layer(
        &self,
        window: &mut Window,
        cx: &Context<'_, Self>,
    ) -> Option<AnyElement> {
        let target = self.line_target()?;
        let clock = (self.clock.now(), Motion::of(cx));
        let scrolling = self.drag.pull != 0.0;
        let aimed = |slide: &LineSlide| !scrolling && slide.target() == target;
        let slide = self.drag.slide.filter(aimed);
        if slide.is_some_and(|slide| slide.is_running(clock)) {
            window.request_animation_frame();
        }
        let at = slide.map_or(target, |slide| slide.at(clock));
        let color = Theme::of(cx).palette(Surface::Chrome).sidebar_drop_line();
        let line = anchored()
            .position_mode(AnchoredPositionMode::Local)
            .position(point(px(at.left), px(at.top)))
            .child(ui::Shaped::seen(drop_line(at, color), px(0.0)));
        Some(
            deferred(line)
                .with_priority(layer::DROP_LINE)
                .into_any_element(),
        )
    }

    pub(crate) fn aim_line(
        &mut self,
        cx: &Context<'_, Self>,
    ) {
        let Some(target) = self.line_target() else {
            self.drag.slide = None;
            return;
        };
        let clock = (self.clock.now(), Motion::of(cx));
        let slide = self
            .drag
            .slide
            .get_or_insert_with(|| LineSlide::resting(target));
        if self.drag.pull != 0.0 {
            slide.jump_to(target);
        }
        slide.slide_to(target, clock);
    }

    pub(crate) fn line_target(&self) -> Option<LineBox> {
        let hint = self.drag.hint.as_ref()?;
        LineBox::of(self.row_bounds.of(&hint.row)?, hint.mode)
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{DropMode, RowId};

    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_line_sits_on_the_laid_out_target_row(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let (subject, target) = (RowId::from("working"), RowId::from("folder"));
        sidebar.update(cx, |this, cx| {
            this.preview_drag((subject, target, DropMode::Before), cx);
        });
        cx.run_until_parked();
        let line = sidebar.read_with(cx, |this, _| this.line_target());
        assert!(line.is_some_and(|at| at.width > 0.0), "{line:?}");
    }

    #[gpui::test]
    fn the_drop_line_aims_at_a_new_target_when_the_drag_moves(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let subject = RowId::from("working");
        sidebar.update(cx, |this, cx| {
            this.preview_drag(
                (subject.clone(), RowId::from("folder"), DropMode::Before),
                cx,
            );
        });
        cx.run_until_parked();
        let aimed = sidebar.update(cx, |this, cx| {
            this.preview_drag((subject, RowId::from("folder"), DropMode::After), cx);
            let aim = this.drag.slide.map(|slide| slide.target());
            (aim, this.line_target())
        });
        assert_eq!(aimed.0, aimed.1, "aimed on the move, before any paint");
    }
}
