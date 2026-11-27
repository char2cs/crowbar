use gpui::{App, InteractiveElement as _, IntoElement, ParentElement, Styled, div, px};
use settings::Page;
use ui::{DropdownMenu, Icon, IconName, IconWeight, MenuAction, MenuItem, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::intent::Sink;

const HEIGHT: f32 = 28.0;
const MAX_WIDTH: f32 = 192.0;
const ICON: f32 = 16.0;
const CARET: f32 = 14.0;
const BORDER_ALPHA: f32 = 0.7;
const CARD_ALPHA: f32 = 0.5;

pub(crate) fn page_picker(
    frame: &Frame<'_>,
    cx: &App,
) -> impl IntoElement {
    DropdownMenu::new("settings-page-picker", trigger(frame, cx), items(frame))
}

fn trigger(
    frame: &Frame<'_>,
    cx: &App,
) -> impl IntoElement {
    let theme = Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let page = frame.state.page();
    let label = page.map_or_else(String::new, |page| page.label().to_owned());
    let icon = page.and_then(|page| icon_named(page.icon()));
    let muted = palette.muted_foreground();
    div()
        .debug_selector(|| "settings-page-picker".to_owned())
        .flex()
        .items_center()
        .gap(spacing::SNUG)
        .h(px(HEIGHT))
        .max_w(px(MAX_WIDTH))
        .min_w_0()
        .px(spacing::COMFORTABLE)
        .rounded(radius::MD)
        .border_1()
        .border_color(palette.border().opacity(BORDER_ALPHA))
        .bg(palette.card().opacity(CARD_ALPHA))
        .text_color(palette.foreground())
        .font_family(theme.heading_family())
        .text_size(ui::text_size::SM)
        .cursor_pointer()
        .children(icon.map(|icon| {
            Icon::new(icon)
                .weight(IconWeight::Duotone)
                .size(px(ICON))
                .color(muted)
        }))
        .child(div().min_w_0().truncate().child(label))
        .child(Icon::new(IconName::CaretDown).size(px(CARET)).color(muted))
}

fn items(frame: &Frame<'_>) -> Vec<MenuItem> {
    frame
        .state
        .visible_pages()
        .into_iter()
        .map(|page| item(frame.sink.clone(), page))
        .collect()
}

fn item(
    sink: Sink,
    page: &Page,
) -> MenuItem {
    let id = page.id().clone();
    MenuItem::Action(MenuAction::new(
        page.label().to_owned(),
        move |window, cx| {
            sink.deliver(Intent::Select(id.clone()), (window, cx));
        },
    ))
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px, size};

    use crate::test_support::open;

    #[gpui::test]
    fn a_narrow_window_swaps_the_rail_for_a_page_picker_in_the_title(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "appearance");
        cx.simulate_resize(size(px(640.0), px(480.0)));
        cx.run_until_parked();
        assert!(cx.debug_bounds("rail-appearance").is_none(), "rail shown");
        assert!(
            cx.debug_bounds("settings-page-picker").is_some(),
            "no picker"
        );
    }
}
