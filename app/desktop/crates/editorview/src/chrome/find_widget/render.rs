use editor::{FindOption, Vertical};
use gpui::{
    AnyElement, ClickEvent, Context, Div, Entity, FontWeight, Hsla, InteractiveElement as _,
    IntoElement, ParentElement as _, Render, SharedString, StatefulInteractiveElement as _, Styled,
    Window, div, px,
};
use ui::{Icon, IconName, Palette, Surface, TextInput, Theme, shadow};

use super::{COUNT_MARGIN, FindWidget, LABEL_SIZE};
use crate::editor_view::reveal::Reveal;
use crate::keys::FIND_CONTEXT;

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findWidget.css
const WIDTH: f32 = 419.0;
const RIGHT_INSET: f32 = 14.0;
const PADDING_X: f32 = 4.0;
const PART_TOP: f32 = 4.0;
const PART_LEFT: f32 = 17.0;
const PART_GAP: f32 = 4.0;
const TOGGLE_WIDTH: f32 = 18.0;
const INPUT_WIDTH: f32 = 228.0;
const INPUT_HEIGHT: f32 = 25.0;
const INPUT_PADDING_X: f32 = 4.0;
const OPTION_SIZE: f32 = 20.0;
const OPTION_GAP: f32 = 2.0;
const BUTTON_SIZE: f32 = 22.0;
const BUTTON_GAP: f32 = 3.0;
const ICON_SIZE: f32 = 16.0;
const RADIUS: f32 = 4.0;
const SMALL_RADIUS: f32 = 3.0;
const SHADOW_ALPHA: f32 = 0.36;
const ACTIVE_ALPHA: f32 = 0.4;

impl Render for FindWidget {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        if self.reveal == Reveal::Hidden {
            return div().into_any_element();
        }
        let palette = *Theme::of(cx).palette(Surface::Window);
        let height = super::strip::height(self.reveal);
        let replace_row = (self.reveal == Reveal::Replace).then(|| self.replace_row(&palette, cx));
        let widget = div()
            .id("find-widget")
            .debug_selector(|| "find-widget".into())
            .key_context(FIND_CONTEXT)
            .absolute()
            .top_0()
            .right(px(RIGHT_INSET))
            .w(px(WIDTH))
            .h(px(height))
            .px(px(PADDING_X))
            .bg(palette.background())
            .text_color(palette.foreground())
            .rounded_bl(px(RADIUS))
            .rounded_br(px(RADIUS))
            .shadow(shadow::md(SHADOW_ALPHA))
            .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| cx.stop_propagation())
            .child(self.reveal_toggle(&palette, cx))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(PART_GAP))
                    .mt(px(PART_TOP))
                    .ml(px(PART_LEFT))
                    .child(self.find_row(&palette, cx))
                    .children(replace_row),
            );
        strip(height, widget)
    }
}

fn strip(
    height: f32,
    widget: impl IntoElement,
) -> AnyElement {
    div()
        .relative()
        .flex_none()
        .w_full()
        .h(px(height))
        .child(widget)
        .into_any_element()
}

impl FindWidget {
    fn reveal_toggle(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let (icon, next) = match self.reveal {
            Reveal::Replace => (IconName::CaretDown, Reveal::Find),
            _ => (IconName::CaretRight, Reveal::Replace),
        };
        div()
            .id("find-reveal")
            .absolute()
            .left_0()
            .top_0()
            .h_full()
            .w(px(TOGGLE_WIDTH))
            .flex()
            .items_center()
            .justify_center()
            .hover(|style| style.bg(palette.accent()))
            .on_click(cx.listener(move |widget, _: &ClickEvent, _, cx| {
                widget.show(next, cx);
                cx.notify();
            }))
            .child(
                Icon::new(icon)
                    .size(px(ICON_SIZE))
                    .color(palette.foreground()),
            )
            .into_any_element()
    }

    fn find_row(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let options = [
            (FindOption::MatchCase, "Aa"),
            (FindOption::WholeWord, "ab"),
            (FindOption::Regex, ".*"),
        ]
        .map(|toggle| self.option_toggle(toggle, palette, cx));
        let in_selection = self.in_selection_button(palette);
        let count = self.count_element(palette, cx);
        row()
            .child(input_box(&self.find_input, palette).children(options))
            .child(count)
            .child(
                icon_button("find-previous", IconName::ArrowUp, palette).on_click(
                    cx.listener(|widget, _: &ClickEvent, _, cx| widget.step(Vertical::Up, cx)),
                ),
            )
            .child(
                icon_button("find-next", IconName::ArrowDown, palette).on_click(
                    cx.listener(|widget, _: &ClickEvent, _, cx| widget.step(Vertical::Down, cx)),
                ),
            )
            .child(
                in_selection
                    .on_click(cx.listener(|widget, _: &ClickEvent, _, cx| widget.toggle_scope(cx))),
            )
            .child(icon_button("find-close", IconName::X, palette).on_click(
                cx.listener(|widget, _: &ClickEvent, window, cx| widget.close(window, cx)),
            ))
    }

    fn in_selection_button(
        &self,
        palette: &Palette,
    ) -> gpui::Stateful<Div> {
        let button = icon_button("find-in-selection", IconName::TextAlignJustify, palette);
        if self.scope.is_none() {
            return button;
        }
        lit(button, palette)
    }

    fn replace_row(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let preserve_case = self.option_toggle((FindOption::PreserveCase, "AB"), palette, cx);
        row()
            .child(input_box(&self.replace_input, palette).child(preserve_case))
            .child(
                icon_button("replace-one", IconName::Swap, palette)
                    .ml(px(COUNT_MARGIN))
                    .on_click(cx.listener(|widget, _: &ClickEvent, window, cx| {
                        widget.replace_one(window, cx);
                    })),
            )
            .child(
                icon_button("replace-all", IconName::Checks, palette).on_click(cx.listener(
                    |widget, _: &ClickEvent, window, cx| {
                        widget.replace_all(window, cx);
                    },
                )),
            )
    }

    fn option_toggle(
        &self,
        (option, label): (FindOption, &'static str),
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let on = self.options.has(option);
        let toggle = div()
            .id(SharedString::from(label))
            .flex_none()
            .size(px(OPTION_SIZE))
            .ml(px(OPTION_GAP))
            .flex()
            .items_center()
            .justify_center()
            .rounded(px(SMALL_RADIUS))
            .text_size(px(LABEL_SIZE))
            .font_weight(FontWeight::MEDIUM)
            .child(label)
            .on_click(cx.listener(move |widget, _: &ClickEvent, _, cx| widget.toggle(option, cx)));
        if !on {
            return toggle.into_any_element();
        }
        lit(toggle, palette).into_any_element()
    }
}

fn row() -> Div {
    div().flex().items_center().h(px(INPUT_HEIGHT))
}

fn input_box(
    input: &Entity<TextInput>,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .flex_none()
        .items_center()
        .w(px(INPUT_WIDTH))
        .h_full()
        .px(px(INPUT_PADDING_X))
        .border_1()
        .border_color(palette.border())
        .rounded(px(SMALL_RADIUS))
        .bg(palette.background())
        .child(div().flex_1().min_w_0().child(input.clone()))
}

fn icon_button(
    id: &'static str,
    icon: IconName,
    palette: &Palette,
) -> gpui::Stateful<Div> {
    div()
        .id(id)
        .flex_none()
        .size(px(BUTTON_SIZE))
        .ml(px(BUTTON_GAP))
        .flex()
        .items_center()
        .justify_center()
        .rounded(px(RADIUS))
        .hover(|style| style.bg(palette.accent()))
        .child(
            Icon::new(icon)
                .size(px(ICON_SIZE))
                .color(palette.foreground()),
        )
}

fn lit<E: Styled>(
    element: E,
    palette: &Palette,
) -> E {
    let ring: Hsla = palette.ring();
    element
        .bg(ring.opacity(ACTIVE_ALPHA))
        .border_1()
        .border_color(ring)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::WIDTH;
    use crate::Access;
    use crate::chrome::find_widget::strip::{FIND_HEIGHT, REPLACE_HEIGHT};
    use crate::editor_view::reveal::Reveal;
    use crate::editor_view::test_support::{find_of, open_find, opened};

    #[gpui::test]
    fn the_widget_takes_monaco_s_size_and_grows_a_row_for_replace(cx: &mut TestAppContext) {
        let cases = [
            (Reveal::Hidden, None),
            (Reveal::Find, Some(FIND_HEIGHT)),
            (Reveal::Replace, Some(REPLACE_HEIGHT)),
        ];
        for (reveal, height) in cases {
            let (view, window) = opened(("total", Some(1)), Access::Editable, cx);
            let find = find_of(&view, window);
            open_find(&find, reveal, window);
            window.run_until_parked();
            let bounds = window
                .debug_bounds("find-widget")
                .filter(|_| reveal != Reveal::Hidden);
            let size =
                bounds.map(|bounds| (f32::from(bounds.size.width), f32::from(bounds.size.height)));
            assert_eq!(size, height.map(|height| (WIDTH, height)), "{reveal:?}");
        }
    }
}
