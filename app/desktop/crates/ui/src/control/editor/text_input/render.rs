use gpui::prelude::FluentBuilder as _;
use gpui::{
    Context, CursorStyle, InteractiveElement, IntoElement, MouseButton, ParentElement, Render,
    Styled, Window, div, px,
};

use crate::control::editor::field::chrome;
use crate::control::editor::text_input::TextInput;
use crate::control::editor::text_input::line_element::LineElement;
use crate::control::editor::text_input_style::TextInputStyle;
use crate::foundation::ControlScale;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::Theme;
use crate::foundation::tokens::text_size;

const ICON_SIZE: f32 = 14.0;
const ICON_INSET: f32 = 8.0;
const ICON_CLEARANCE: f32 = 28.0;

impl Render for TextInput {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let frame = chrome(
            palette,
            theme.appearance(),
            self.focus_handle.is_focused(window).into(),
        );
        let (fill, border, shadows) = self.finish(palette, frame, window);
        let field = self.field(cx);
        let (height, text) = self.style.sized(theme, ControlScale::of(window));
        let left = self
            .leading
            .map_or(self.style.padding(), |_| px(ICON_CLEARANCE));
        Self::bind_actions(field, cx)
            .relative()
            .flex()
            .items_center()
            .w_full()
            .h(height)
            .pl(left)
            .pr(self.style.padding())
            .rounded(self.style.radius())
            .border_1()
            .border_color(border)
            .bg(fill)
            .text_color(palette.foreground())
            .text_size(text)
            .line_height(text_size::LINE_HEIGHT)
            .font_family(theme.ui_family())
            .when(self.style == TextInputStyle::Bare, |d| {
                d.font_weight(gpui::FontWeight::MEDIUM)
            })
            .shadow(shadows)
            .children(self.leading.map(|icon| leading(icon, palette)))
            .child(LineElement::new(cx.entity(), palette))
    }
}

impl TextInput {
    fn field(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> gpui::Stateful<gpui::Div> {
        div()
            .id("ui-text-input")
            .key_context(crate::control::editor::text_input::actions::CONTEXT)
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .cursor(CursorStyle::IBeam)
    }

    fn stepper_border(
        &self,
        palette: &crate::foundation::Palette,
        border: gpui::Hsla,
        window: &Window,
    ) -> gpui::Hsla {
        if self.focus_handle.is_focused(window) {
            return border;
        }
        palette.border()
    }

    fn finish(
        &self,
        palette: &crate::foundation::Palette,
        frame: crate::control::editor::field::Chrome,
        window: &Window,
    ) -> (gpui::Hsla, gpui::Hsla, Vec<gpui::BoxShadow>) {
        if self.style == TextInputStyle::Bare {
            let clear = gpui::hsla(0.0, 0.0, 0.0, 0.0);
            return (clear, clear, Vec::new());
        }
        if self.style != TextInputStyle::Stepper {
            return (frame.fill, frame.border, frame.shadows);
        }
        (
            palette.muted(),
            self.stepper_border(palette, frame.border, window),
            Vec::new(),
        )
    }
}

fn leading(
    icon: IconName,
    palette: &crate::foundation::Palette,
) -> impl IntoElement {
    div().absolute().left(px(ICON_INSET)).child(
        Icon::new(icon)
            .size(px(ICON_SIZE))
            .color(palette.muted_foreground()),
    )
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::control::editor::test_support::field::field;

    #[gpui::test]
    fn the_field_renders_and_keeps_focus_after_typing(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("x");
        cx.run_until_parked();
        let focused = input.read_with(cx, |input, _| input.focus_handle.clone());
        assert!(cx.update(|window, _| focused.is_focused(window)), "focused");
    }
}
