//! The sliding drop line, drawn above the whole window while a row is dragged.

use gpui::{
    AnyElement, Context, IntoElement, ParentElement, Window, anchored, deferred, point, px,
};
use ui::{Motion, Surface, Theme, layer};

use crate::drop_line::drop_line;
use crate::line_box::LineBox;
use crate::line_slide::LineSlide;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn drop_line_layer(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let target = self.line_target();
        let Some(target) = target else {
            self.drag.slide = None;
            return None;
        };
        let clock = (self.clock.now(), Motion::of(cx));
        let scrolling = self.drag.pull != 0.0;
        let slide = self
            .drag
            .slide
            .get_or_insert_with(|| LineSlide::resting(target));
        if scrolling {
            slide.jump_to(target);
        }
        slide.slide_to(target, clock);
        if slide.is_running(clock) {
            window.request_animation_frame();
        }
        let at = slide.at(clock);
        let color = Theme::of(cx).palette(Surface::Chrome).sidebar_drop_line();
        let line = anchored()
            .position(point(px(at.left), px(at.top)))
            .child(drop_line(at, color));
        Some(
            deferred(line)
                .with_priority(layer::DROP_LINE)
                .into_any_element(),
        )
    }

    fn line_target(&self) -> Option<LineBox> {
        let hint = self.drag.hint.as_ref()?;
        LineBox::of(self.row_bounds.of(&hint.row)?, hint.mode)
    }
}

#[cfg(test)]
mod tests {
    use crate::drop_mode::DropMode;
    use crate::row_id::RowId;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_line_sits_on_the_laid_out_target_row(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        let (subject, target) = (RowId::from("working"), RowId::from("folder"));
        sidebar.update(cx, |this, cx| {
            this.preview_drag((subject, target, DropMode::Before), cx);
        });
        cx.run_until_parked();
        let line = sidebar.read_with(cx, |this, _| this.line_target());
        assert!(line.is_some_and(|at| at.width > 0.0), "{line:?}");
    }
}
