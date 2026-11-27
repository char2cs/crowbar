use gpui::{Context, Pixels};

use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::render::text_area_condition::TextAreaCondition;
use crate::control::editor::text_area::render::text_area_size::TextAreaSize;

impl TextArea {
    #[must_use]
    pub fn size(
        mut self,
        size: TextAreaSize,
    ) -> Self {
        self.size = size;
        self
    }

    #[must_use]
    pub fn max_height(
        mut self,
        height: Pixels,
    ) -> Self {
        self.limit = Some(height);
        self
    }

    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.condition = TextAreaCondition::Disabled;
        self
    }

    #[must_use]
    pub fn invalid(mut self) -> Self {
        self.condition = TextAreaCondition::Invalid;
        self
    }

    pub fn set_condition(
        &mut self,
        condition: TextAreaCondition,
        cx: &mut Context<'_, Self>,
    ) {
        self.condition = condition;
        self.selecting = false;
        self.autoscroll = None;
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use crate::control::editor::test_support::area::area_with;
    use crate::control::editor::text_area::TextArea;
    use crate::control::editor::text_area::render::text_area_condition::TextAreaCondition;

    #[gpui::test]
    fn a_disabled_area_refuses_input_until_the_condition_changes(cx: &mut TestAppContext) {
        let (area, cx) = area_with(cx, |area| area.disabled().max_height(px(120.0)));
        assert!(
            !area.read_with(cx, |area, _| area.accepts_input()),
            "disabled"
        );
        area.update(cx, |area: &mut TextArea, cx| {
            area.set_condition(TextAreaCondition::Normal, cx);
        });
        assert!(
            area.read_with(cx, |area, _| area.accepts_input()),
            "enabled"
        );
    }
}
