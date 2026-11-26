use gpui::{
    AnyElement, ClickEvent, Context, InteractiveElement as _, IntoElement as _, ParentElement as _,
    StatefulInteractiveElement as _, Styled as _, div, px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip};

use crate::EditorEvent;
use crate::editor_view::EditorView;

const ICON_SIZE: f32 = 16.0;

pub(in crate::editor_view) struct ViewButton {
    pub(in crate::editor_view) id: &'static str,
    pub(in crate::editor_view) label: &'static str,
    pub(in crate::editor_view) size: (f32, f32),
    pub(in crate::editor_view) radius: f32,
    pub(in crate::editor_view) event: EditorEvent,
}

impl ViewButton {
    pub(in crate::editor_view) const PREVIEW: Self = Self {
        id: "editor-preview",
        label: "Preview",
        size: (36.0, 32.0),
        radius: 4.0,
        event: EditorEvent::PreviewRequested,
    };
    pub(in crate::editor_view) const RICH_VIEW: Self = Self {
        id: "editor-rich-view",
        label: "View rich editor",
        size: (32.0, 28.0),
        radius: 10.0,
        event: EditorEvent::RichViewRequested,
    };

    pub(in crate::editor_view) fn element(
        self,
        cx: &mut Context<'_, EditorView>,
    ) -> AnyElement {
        let Self {
            id,
            label,
            size: (width, height),
            radius,
            event,
        } = self;
        let palette = *Theme::of(cx).palette(Surface::Window);
        let trigger = div()
            .id(id)
            .debug_selector(|| id.to_owned())
            .flex()
            .w(px(width))
            .h(px(height))
            .items_center()
            .justify_center()
            .rounded(px(radius))
            .hover(|style| style.bg(palette.accent()))
            .on_click(cx.listener(move |_, _: &ClickEvent, _, cx| cx.emit(event.clone())))
            .child(
                Icon::new(IconName::Eye)
                    .size(px(ICON_SIZE))
                    .color(palette.muted_foreground()),
            );
        Tooltip::new(id, label, trigger).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::editor_view::find::find_actions::FIND_BUTTON;
    use crate::editor_view::test_support::opened_at;

    #[gpui::test]
    fn the_toolbar_buttons_take_develop_s_button_sizes(cx: &mut TestAppContext) {
        let (_view, window) = opened_at("README.md", cx);
        window.run_until_parked();
        let cases = [
            (ViewButton::PREVIEW.id, 36.0, 32.0),
            (FIND_BUTTON, 36.0, 32.0),
            (ViewButton::RICH_VIEW.id, 32.0, 28.0),
        ];
        for (id, width, height) in cases {
            let size = window.debug_bounds(id).map(|bounds| bounds.size);
            assert_eq!(size, Some(gpui::size(px(width), px(height))), "{id}");
        }
    }
}
