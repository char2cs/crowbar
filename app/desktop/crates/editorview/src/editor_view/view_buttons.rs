use gpui::{
    AnyElement, ClickEvent, Context, InteractiveElement as _, IntoElement as _, ParentElement as _,
    StatefulInteractiveElement as _, Styled as _, div, px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip};

use super::EditorView;
use crate::EditorEvent;

const BUTTON_SIZE: f32 = 20.0;
const ICON_SIZE: f32 = 14.0;
const BUTTON_RADIUS: f32 = 4.0;
const ACTION_GAP: f32 = 4.0;
const PREVIEW: &str = "editor-preview";
const RICH_VIEW: &str = "editor-rich-view";
const PREVIEW_LABEL: &str = "Preview";
const RICH_VIEW_LABEL: &str = "View rich editor";

impl EditorView {
    pub(super) fn toolbar_actions(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        div()
            .flex()
            .items_center()
            .gap(px(ACTION_GAP))
            .children(self.preview_button(cx))
            .child(Self::find_button(cx))
            .into_any_element()
    }

    fn preview_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        self.toolbar
            .previews()
            .then(|| button((PREVIEW, PREVIEW_LABEL), EditorEvent::PreviewRequested, cx))
    }

    pub(super) fn rich_view_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        self.toolbar.has_rich_view().then(|| {
            button(
                (RICH_VIEW, RICH_VIEW_LABEL),
                EditorEvent::RichViewRequested,
                cx,
            )
        })
    }
}

fn button(
    (id, label): (&'static str, &'static str),
    event: EditorEvent,
    cx: &mut Context<'_, EditorView>,
) -> AnyElement {
    let palette = *Theme::of(cx).palette(Surface::Window);
    let trigger = div()
        .id(id)
        .debug_selector(|| id.to_owned())
        .flex()
        .size(px(BUTTON_SIZE))
        .items_center()
        .justify_center()
        .rounded(px(BUTTON_RADIUS))
        .hover(|style| style.bg(palette.accent()))
        .on_click(cx.listener(move |_, _: &ClickEvent, _, cx| cx.emit(event)))
        .child(
            Icon::new(IconName::Eye)
                .size(px(ICON_SIZE))
                .color(palette.muted_foreground()),
        );
    Tooltip::new(id, label, trigger).into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, VisualTestContext};

    use super::*;
    use crate::editor_view::test_support::{listened, opened_at};

    fn click(
        window: &mut VisualTestContext,
        id: &'static str,
    ) -> bool {
        window.run_until_parked();
        let Some(bounds) = window.debug_bounds(id) else {
            return false;
        };
        window.simulate_click(bounds.center(), Modifiers::none());
        true
    }

    #[gpui::test]
    fn a_markdown_view_offers_its_preview_and_rich_view_an_html_one_only_its_preview(
        cx: &mut TestAppContext
    ) {
        let cases = [
            (
                "README.md",
                vec![
                    EditorEvent::PreviewRequested,
                    EditorEvent::RichViewRequested,
                ],
            ),
            (
                "docs/a.markdown",
                vec![
                    EditorEvent::PreviewRequested,
                    EditorEvent::RichViewRequested,
                ],
            ),
            ("site/index.html", vec![EditorEvent::PreviewRequested]),
            ("site/old.htm", vec![EditorEvent::PreviewRequested]),
            ("src/a.ts", vec![]),
        ];
        for (path, expected) in cases {
            let (view, window) = opened_at(path, cx);
            let heard = listened(&view, window);
            click(window, PREVIEW);
            click(window, RICH_VIEW);
            assert_eq!(*heard.borrow(), expected, "{path}");
        }
    }
}
