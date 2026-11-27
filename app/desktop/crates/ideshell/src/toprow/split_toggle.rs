use gpui::{App, IntoElement, RenderOnce, SharedString, Window};
use layout::EditorView;
use ui::IconName;

use crate::tabbar::tab_actions::TabAction;
use crate::toprow::row_button::RowButton;
use crate::toprow::row_ink::RowInk;

#[derive(IntoElement)]
pub struct SplitToggle {
    pub key: SharedString,
    pub view: EditorView,
    pub on_toggle: TabAction,
}

ui::opaque_debug!(SplitToggle);

impl RenderOnce for SplitToggle {
    fn render(
        self,
        _window: &mut Window,
        _cx: &mut App,
    ) -> impl IntoElement {
        let (tip, ink) = look_of(self.view);
        RowButton {
            id: format!("split-{}", self.key).into(),
            icon: IconName::SquareSplitHorizontal,
            tip: tip.into(),
            ink,
            on_click: self.on_toggle,
        }
    }
}

fn look_of(view: EditorView) -> (&'static str, RowInk) {
    match view {
        EditorView::Open => ("Show chat only", RowInk::Strong),
        EditorView::Closed => ("Show editor alongside chat", RowInk::Muted),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_toggle_names_what_a_click_will_show_and_looks_pressed_while_open() {
        let cases = [
            (EditorView::Open, "Show chat only", RowInk::Strong),
            (
                EditorView::Closed,
                "Show editor alongside chat",
                RowInk::Muted,
            ),
        ];
        for (view, tip, ink) in cases {
            assert_eq!(look_of(view), (tip, ink), "{view:?}");
        }
    }
}
