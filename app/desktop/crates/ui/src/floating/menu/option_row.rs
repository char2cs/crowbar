//! One option of a select's list: a 16px check column, then the label, 28px tall.
//! Spec: web/src/components/ui/select.tsx (`SelectItem`).

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, Hsla, InteractiveElement, IntoElement, ParentElement, Pixels, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::floating::menu::menu_item::MenuAction;
use crate::floating::menu::row::colors;
use crate::floating::menu::row_handlers::RowHandlers;
use crate::foundation::ControlScale;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::radius;

pub(crate) const HEIGHT: Pixels = px(28.0);
pub(crate) const START: Pixels = px(8.0);
pub(crate) const CHECK: Pixels = px(16.0);
pub(crate) const GAP: Pixels = px(8.0);
const END: Pixels = px(16.0);
const DISABLED_OPACITY: f32 = 0.64;

#[derive(IntoElement)]
pub(crate) struct OptionRow {
    index: usize,
    action: MenuAction,
    highlight: Pose,
    handlers: RowHandlers,
}

impl OptionRow {
    pub(crate) fn new(
        index: usize,
        action: MenuAction,
        highlight: Pose,
        handlers: RowHandlers,
    ) -> Self {
        Self {
            index,
            action,
            highlight,
            handlers,
        }
    }
}

impl RenderOnce for OptionRow {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let point = ControlScale::of(window);
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let action = &self.action;
        let (fill, text) = colors(action, self.highlight, palette, theme.appearance());
        let (index, handlers) = (self.index, self.handlers);
        div()
            .id(("menu-row", index))
            .debug_selector(move || format!("menu-row-{index}"))
            .flex()
            .items_center()
            .gap(GAP)
            .h(HEIGHT + point.growth())
            .pl(START)
            .pr(END)
            .rounded(radius::SM)
            .bg(fill)
            .text_color(text)
            .text_size(point.text())
            .when(action.disabled, |d| d.opacity(DISABLED_OPACITY))
            .when(!action.disabled, |d| {
                d.cursor_pointer()
                    .on_mouse_move(move |_, window, cx| (handlers.hover)(index, window, cx))
                    .on_click(move |_, window, cx| (handlers.choose)(index, window, cx))
            })
            .child(check_slot(action, (text, point.icon())))
            .child(label(index, action))
    }
}

fn check_slot(
    action: &MenuAction,
    (color, size): (Hsla, Pixels),
) -> Div {
    let check = Icon::new(IconName::Check)
        .weight(IconWeight::Bold)
        .size(size)
        .color(color);
    div()
        .flex_none()
        .w(CHECK)
        .when(action.checked, |slot| slot.child(check))
}

fn label(
    index: usize,
    action: &MenuAction,
) -> Div {
    div()
        .debug_selector(move || format!("option-label-{index}"))
        .min_w_0()
        .child(action.label.clone())
}

opaque_debug!(OptionRow);

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::floating::menu::menu_item::MenuItem;
    use crate::floating::menu::{Menu, RowStyle};
    use crate::test_support::probe::open;

    #[gpui::test]
    fn an_option_is_28px_tall_with_its_label_after_a_16px_check_column(cx: &mut TestAppContext) {
        let cx = open(cx, |window, cx| {
            let items: Rc<[MenuItem]> = Rc::from(vec![MenuItem::Action(
                MenuAction::new("Dark", |_, _| {}).checked(),
            )]);
            cx.new(|cx| Menu::new(items, window, cx).row_style(RowStyle::Option))
        });
        let row = cx.debug_bounds("menu-row-0").unwrap_or_default();
        assert_eq!(row.size.height, HEIGHT);
        let label = cx.debug_bounds("option-label-0").unwrap_or_default();
        assert_eq!(label.left() - row.left(), START + CHECK + GAP);
    }
}
