use gpui::Context;
use ideshell::{ChatTab, SplitToggle, TabState};
use layout::{Command, EditorView, PaneGroup, Presentation};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;

impl LayoutView {
    pub(crate) fn toggle_of(
        group: &PaneGroup,
        anchor: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> SplitToggle {
        let view = EditorView::of(group);
        let flip = LayoutEvent::Apply(Command::SetEditorView {
            pane: anchor.clone(),
            view: view.flipped(),
        });
        SplitToggle {
            key: anchor.as_str().to_owned().into(),
            view,
            on_toggle: Self::emitting(flip, cx),
        }
    }

    pub(crate) fn chat_tab_of(
        &self,
        (group, shown): (&PaneGroup, &Presentation),
        cx: &mut Context<'_, Self>,
    ) -> Option<ChatTab> {
        let chat = group.chat().filter(|_| shown.chat_tab)?;
        let look = self.look_of(chat);
        Some(ChatTab {
            pane: chat.clone(),
            label: look.label,
            icon: look.icon,
            artwork: look.artwork,
            state: chat_state(group),
            select: Self::select_action(chat, cx),
        })
    }
}

fn chat_state(group: &PaneGroup) -> TabState {
    if group.chat_selected() {
        return TabState::Active;
    }
    TabState::Idle
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::Command;

    use super::*;
    use crate::test_support::{layout_of, open, open_tab, pane_id};

    #[gpui::test]
    fn the_toggle_flips_the_group_editor_view(cx: &mut TestAppContext) {
        let mut layout = layout_of("a", &[]);
        open_tab(&mut layout, "t1");
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let toggle = view.update(cx, |view, cx| {
            let group = view
                .tab
                .as_ref()
                .and_then(|tab| tab.group_of(&pane_id("a")))
                .cloned();
            group.map(|group| LayoutView::toggle_of(&group, &pane_id("t1"), cx))
        });
        let toggle = toggle.expect("a group");
        assert_eq!(toggle.view, EditorView::Open, "a tab opened the editor");
        cx.update(|window, cx| (toggle.on_toggle)(window, cx));
        let close = Command::SetEditorView {
            pane: pane_id("t1"),
            view: EditorView::Closed,
        };
        assert_eq!(commands.borrow().as_slice(), &[close]);
    }
}
