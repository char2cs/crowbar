//! One page entry of the rail.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use settings::Page;
use ui::{HoverFill, Icon, IconWeight, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;

const HEIGHT: f32 = 32.0;
const ICON: f32 = 18.0;
const LABEL: f32 = 19.0;
const ACTIVE_TINT: f32 = 0.12;

pub(crate) fn rail_item(
    frame: &Frame<'_>,
    page: &Page,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let selected = frame
        .state
        .page()
        .filter(|selected| selected.id() == page.id());
    let active = selected.is_some();
    let name = format!("rail-{}", page.id());
    let id = ElementId::from(SharedString::from(name.clone()));
    let color = selected.map_or(palette.muted_foreground(), |_| palette.foreground());
    let base = div()
        .flex()
        .items_center()
        .gap(spacing::ITEM)
        .w_full()
        .h(px(HEIGHT))
        .px(spacing::ITEM)
        .rounded(radius::XL)
        .text_color(color);
    let fill = selected.map_or(palette.accent(), |_| palette.primary().opacity(ACTIVE_TINT));
    let item = HoverFill::new(id, fill).apply(base, (window, cx));
    let foreground = palette.foreground();
    item.when(active, |item| item.bg(fill))
        .when(!active, |item| {
            item.hover(move |style| style.text_color(foreground))
        })
        .cursor_pointer()
        .debug_selector(move || name)
        .on_click(frame.sink.send(Intent::Select(page.id().clone())))
        .children(
            icon_named(page.icon())
                .map(|icon| Icon::new(icon).weight(IconWeight::Duotone).size(px(ICON))),
        )
        .child(label(page, Theme::of(cx).heading_family()))
}

fn label(
    page: &Page,
    family: &'static str,
) -> impl IntoElement {
    div()
        .font_family(family)
        .text_size(px(LABEL))
        .line_height(px(HEIGHT))
        .overflow_hidden()
        .whitespace_nowrap()
        .child(page.label().to_owned())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::RailMove;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};

    fn selected_page(modal: &crate::settings_modal::SettingsModal) -> Option<String> {
        modal.state.page().map(|page| page.id().to_string())
    }

    #[gpui::test]
    fn selecting_and_moving_change_the_active_item(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let active = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
            inspect(harness, cx, selected_page)
        };
        run(&harness, Intent::Select("terminal".into()), cx);
        assert_eq!(active(&harness, cx), Some("terminal".to_owned()));
        run(&harness, Intent::Move(RailMove::Next), cx);
        assert_eq!(active(&harness, cx), Some("browser".to_owned()));
    }
}
