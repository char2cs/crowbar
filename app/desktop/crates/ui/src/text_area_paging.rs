//! Page Up and Page Down of [`TextArea`]: move the caret a viewport height, keeping its column.

use gpui::{Context, point};

use crate::direction::Direction;
use crate::text_area::TextArea;

impl TextArea {
    fn page_target(
        &mut self,
        direction: Direction,
    ) -> Option<usize> {
        let bounds = self.bounds?;
        let cursor = self.buffer.cursor();
        let goal = self.goal_x();
        let top = self.layout.point_for_index(cursor).y;
        let span = match direction {
            Direction::Backward => -bounds.size.height,
            Direction::Forward => bounds.size.height,
        };
        let target = self.layout.index_for_point(point(goal, top + span));
        self.goal = Some((target, goal));
        Some(target)
    }

    pub(crate) fn page(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(target) = self.page_target(direction) {
            self.buffer.move_to(target);
            cx.notify();
        }
    }

    pub(crate) fn extend_page(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(target) = self.page_target(direction) {
            self.buffer.select_to(target);
            cx.notify();
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::{area_text, area_with};
    use crate::text_area_size::TextAreaSize;

    const ROWS: &str = "r0\nr1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\nr9";

    #[gpui::test]
    fn page_keys_move_a_viewport_and_shift_selects(cx: &mut TestAppContext) {
        let (area, cx) = area_with(cx, |area| {
            area.size(TextAreaSize::Small).max_height(gpui::px(62.0))
        });
        area.update(cx, |area, cx| area.set_text(ROWS, cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("ctrl-home pagedown");
        cx.simulate_input("X");
        let paged = area_text(&area, cx);
        let landed = paged.lines().position(|line| line.starts_with('X'));
        assert!(landed.is_some_and(|row| row > 1), "{paged}");
        cx.simulate_keystrokes("pageup");
        cx.simulate_input("Y");
        assert!(area_text(&area, cx).starts_with("rY0"));
        cx.simulate_keystrokes("shift-pagedown");
        cx.simulate_input("Z");
        let after = area_text(&area, cx);
        assert!(after.starts_with("rYZ"), "{after}");
        assert!(after.lines().count() < paged.lines().count());
    }
}
