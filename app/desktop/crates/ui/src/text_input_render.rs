use gpui::prelude::FluentBuilder as _;
use gpui::{
    Context, CursorStyle, InteractiveElement, IntoElement, MouseButton, ParentElement, Render,
    Styled, Window, div, px,
};

use crate::chrome::chrome;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::text_input::TextInput;
use crate::text_input_element::TextInputElement;
use crate::text_input_style::TextInputStyle;
use crate::text_size;
use crate::theme::Theme;

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
        let left = self
            .leading
            .map_or(self.style.padding(), |_| px(ICON_CLEARANCE));
        Self::bind_actions(field, cx)
            .relative()
            .flex()
            .items_center()
            .w_full()
            .h(self.style.height())
            .pl(left)
            .pr(self.style.padding())
            .rounded(self.style.radius())
            .border_1()
            .border_color(border)
            .bg(fill)
            .text_color(palette.foreground())
            .text_size(self.style.text(theme))
            .line_height(text_size::LINE_HEIGHT)
            .font_family(theme.ui_family())
            .when(self.style == TextInputStyle::Bare, |d| {
                d.font_weight(gpui::FontWeight::MEDIUM)
            })
            .shadow(shadows)
            .children(self.leading.map(|icon| leading(icon, palette)))
            .child(TextInputElement::new(cx.entity(), palette))
    }
}

impl TextInput {
    fn field(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> gpui::Stateful<gpui::Div> {
        div()
            .id("ui-text-input")
            .key_context(crate::text_input_actions::CONTEXT)
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .cursor(CursorStyle::IBeam)
    }

    fn stepper_border(
        &self,
        palette: &crate::palette::Palette,
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
        palette: &crate::palette::Palette,
        frame: crate::chrome::Chrome,
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
    palette: &crate::palette::Palette,
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

    use crate::test_support::field;

    #[gpui::test]
    fn the_field_renders_and_keeps_focus_after_typing(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("x");
        cx.run_until_parked();
        let focused = input.read_with(cx, |input, _| input.focus_handle.clone());
        assert!(cx.update(|window, _| focused.is_focused(window)), "focused");
    }
}
