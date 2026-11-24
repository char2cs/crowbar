use gpui::{AnyElement, Context, IntoElement as _, ParentElement as _, Styled as _, div, px};

use super::EditorView;
use super::view_button::ViewButton;

const ACTION_GAP: f32 = 4.0;

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
            .then(|| ViewButton::PREVIEW.element(cx))
    }

    pub(super) fn rich_view_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        self.toolbar
            .has_rich_view()
            .then(|| ViewButton::RICH_VIEW.element(cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, VisualTestContext};

    use super::*;
    use crate::EditorEvent;
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
            click(window, ViewButton::PREVIEW.id);
            click(window, ViewButton::RICH_VIEW.id);
            assert_eq!(*heard.borrow(), expected, "{path}");
        }
    }
}
