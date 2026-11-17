use gpui::prelude::FluentBuilder as _;
use gpui::{
    Context, CursorStyle, InteractiveElement, IntoElement, MouseButton, ParentElement, Render,
    Styled, Window, div,
};

use crate::pose::Pose;
use crate::radius;
use crate::text_area::TextArea;
use crate::text_area_actions::CONTEXT;
use crate::text_area_chrome::text_area_chrome;
use crate::text_area_element::TextAreaElement;
use crate::text_size;
use crate::theme::Theme;

const DISABLED_OPACITY: f32 = 0.64;

impl Render for TextArea {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let focus = Pose::from(self.focus_handle.is_focused(window) && self.accepts_input());
        let frame = text_area_chrome(palette, theme.appearance(), focus, self.condition);
        let limit = self.limit.map(|height| height - self.size.frame_height());
        let field = div().id("ui-text-area").key_context(CONTEXT);
        let field = self.interactive(field, cx);
        Self::bind_actions(field, cx)
            .relative()
            .w_full()
            .min_h(self.size.min_height())
            .px(self.size.across())
            .py(self.size.down())
            .rounded(radius::LG)
            .border_1()
            .border_color(frame.border)
            .bg(frame.fill)
            .shadow(frame.shadows)
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .line_height(text_size::LINE_HEIGHT)
            .font_family(theme.ui_family())
            .when(!self.accepts_input(), |field| {
                field.opacity(DISABLED_OPACITY)
            })
            .child(TextAreaElement::new(cx.entity(), palette, limit))
    }
}

impl TextArea {
    fn interactive(
        &self,
        field: gpui::Stateful<gpui::Div>,
        cx: &mut Context<'_, Self>,
    ) -> gpui::Stateful<gpui::Div> {
        if !self.accepts_input() {
            return field;
        }
        field
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_scroll_wheel(cx.listener(Self::on_scroll))
            .cursor(CursorStyle::IBeam)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::area;

    #[gpui::test]
    fn an_enabled_area_accepts_input(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        let accepts = area.read_with(cx, |area, _| area.accepts_input());
        assert!(accepts, "enabled by default");
    }
}
