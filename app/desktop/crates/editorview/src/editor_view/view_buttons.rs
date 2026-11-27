mod view_button;

use gpui::{
    AnyElement, ClickEvent, Context, Focusable, InteractiveElement as _, IntoElement as _,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, div, px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip};

use super::EditorView;
use crate::editor_view::view_buttons::view_button::ViewButton;

const ACTION_GAP: f32 = 4.0;
pub(in crate::editor_view) const FIND_BUTTON: &str = "find-in-file";
const FIND_BUTTON_LABEL: &str = "Find in file";
const FIND_BUTTON_WIDTH: f32 = 36.0;
const FIND_BUTTON_HEIGHT: f32 = 32.0;
const FIND_ICON_SIZE: f32 = 16.0;
const FIND_BUTTON_RADIUS: f32 = 4.0;

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

    fn find_button(cx: &mut Context<'_, Self>) -> AnyElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let trigger = div()
            .id(FIND_BUTTON)
            .debug_selector(|| FIND_BUTTON.to_owned())
            .flex()
            .w(px(FIND_BUTTON_WIDTH))
            .h(px(FIND_BUTTON_HEIGHT))
            .items_center()
            .justify_center()
            .rounded(px(FIND_BUTTON_RADIUS))
            .hover(|style| style.bg(palette.accent()))
            .on_click(cx.listener(|view, _: &ClickEvent, window, cx| {
                window.focus(&Focusable::focus_handle(view.state.read(cx), cx), cx);
                view.state
                    .update(cx, |state, cx| state.open_search(false, cx));
            }))
            .child(
                Icon::new(IconName::MagnifyingGlass)
                    .size(px(FIND_ICON_SIZE))
                    .color(palette.muted_foreground()),
            );
        Tooltip::new(FIND_BUTTON, FIND_BUTTON_LABEL, trigger).into_any_element()
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
    use crate::editor_view::test_support::{listened, opened_at, press, state_of};

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
    fn ctrl_f_and_the_find_button_open_the_editor_s_own_search_panel(cx: &mut TestAppContext) {
        let (view, window) = opened_at("src/a.ts", cx);
        let heard = listened(&view, window);
        let open = |window: &mut VisualTestContext| {
            window.run_until_parked();
            window.read(|cx| state_of(&view, cx).read(cx).search_session().open)
        };
        assert!(!open(window), "closed at first");
        press("@input::Search", window);
        assert!(open(window), "ctrl-f");
        press("escape", window);
        assert!(!open(window), "escape");
        assert!(click(window, "find-in-file"), "the toolbar button");
        assert!(open(window), "the toolbar button");
        assert!(heard.borrow().is_empty());
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
