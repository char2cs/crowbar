use gpui::Context;
use layout::{Edge, Layout, Placement, TabId};
use ui::ToastKind;
use viewtabs::TabDrop;

use crate::panes::tracking::open_chats::panes_showing;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/components/sidebar/lib/pane-drop-actions.ts
const OTHER_SPACE: &str = "That chat belongs to a different space";

impl ShellView {
    pub(crate) fn drop_chat(
        &mut self,
        chat: &str,
        (tab, placement): (TabId, &Placement),
        cx: &mut Context<'_, Self>,
    ) {
        let dragged = self.data.chat_project(chat).unwrap_or_default();
        let target = space_of(self.state.layout(), tab).to_owned();
        if !self.refuses((&dragged, &target), cx) {
            self.state.open_chat_at(chat, (tab, placement.clone()));
        }
    }

    pub(crate) fn drop_chat_on_strip(
        &mut self,
        chat: &str,
        at: TabDrop,
        cx: &mut Context<'_, Self>,
    ) {
        if self.refuses_merge(chat, at, cx) {
            return;
        }
        let open = panes_showing(self.state.layout(), &[chat.to_owned()]);
        if let Some(pane) = open.first() {
            let commands = self.tabs.update(cx, |bar, _| bar.pane_drop(pane, at));
            return self.apply_guarded(commands, cx);
        }
        let TabDrop::Merge(tab) = at else {
            return;
        };
        let Some(placement) = after_last_pane(self.state.layout(), tab) else {
            return;
        };
        self.state.open_chat_at(chat, (tab, placement));
        self.refresh(cx);
    }

    fn refuses_merge(
        &mut self,
        chat: &str,
        at: TabDrop,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let TabDrop::Merge(tab) = at else {
            return false;
        };
        let dragged = self.data.chat_project(chat).unwrap_or_default();
        let target = space_of(self.state.layout(), tab).to_owned();
        self.refuses((&dragged, &target), cx)
    }

    pub(crate) fn drop_view(
        &mut self,
        tab: TabId,
        (onto, placement): (TabId, &Placement),
        cx: &mut Context<'_, Self>,
    ) {
        let layout = self.state.layout();
        let (dragged, target) = (space_of(layout, tab), space_of(layout, onto));
        let (dragged, target) = (dragged.to_owned(), target.to_owned());
        if !self.refuses((&dragged, &target), cx) {
            self.state.drop_tab(tab, (onto, placement.clone()));
        }
    }

    fn refuses(
        &mut self,
        (dragged, target): (&str, &str),
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let crossing = crosses(dragged, target);
        if crossing {
            let said = (ToastKind::Error, OTHER_SPACE.to_owned());
            self.show_notice(NoticeKey::Command, said, cx);
        }
        crossing
    }
}

fn after_last_pane(
    layout: &Layout,
    tab: TabId,
) -> Option<Placement> {
    let target = layout.tab(tab)?.pane_ids().pop()?;
    Some(Placement {
        target,
        edge: Edge::Right,
    })
}

fn space_of(
    layout: &Layout,
    tab: TabId,
) -> &str {
    layout.tab(tab).map_or("", layout::Tab::space)
}

fn crosses(
    dragged: &str,
    target: &str,
) -> bool {
    !dragged.is_empty() && !target.is_empty() && dragged != target
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::panes::kinds::core_panes::{chat_params, chat_type};
    use crate::shell::shell_view::tests::{Opened, open_shell};

    fn open_chats(
        opened: &mut Opened,
        chats: &[&str],
    ) {
        for chat in chats {
            open_view(opened, chat);
        }
        opened.shell.update(&mut opened.window, ShellView::refresh);
    }

    fn open_view(
        opened: &mut Opened,
        chat: &str,
    ) {
        let (type_id, params) = (
            chat_type().expect("type"),
            chat_params(chat).expect("params"),
        );
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.open_view(type_id, params);
        });
    }

    fn tab_of(
        opened: &mut Opened,
        chat: &str,
    ) -> TabId {
        opened.shell.read_with(&opened.window, |shell, _| {
            let layout = shell.state.layout();
            let pane = panes_showing(layout, &[chat.to_owned()]);
            let holds = |tab: &&layout::Tab| pane.first().is_some_and(|p| tab.pane(p).is_some());
            layout
                .tabs()
                .iter()
                .find(holds)
                .map(layout::Tab::id)
                .expect("a tab shows the chat")
        })
    }

    fn tab_count(opened: &mut Opened) -> usize {
        opened
            .shell
            .read_with(&opened.window, |shell, _| shell.state.layout().tabs().len())
    }

    fn drop_row(
        opened: &mut Opened,
        chat: &str,
        at: TabDrop,
    ) {
        let bar = opened
            .shell
            .read_with(&opened.window, |shell, _| shell.tabs.clone());
        let event = viewtabs::Event::DropChat {
            chat: chat.to_owned(),
            at,
        };
        bar.update(&mut opened.window, |_, cx| cx.emit(event));
        opened.window.run_until_parked();
    }

    #[gpui::test]
    fn a_row_dropped_on_a_tab_middle_moves_an_open_chat_or_opens_a_closed_one_in_that_tab(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_shell(cx);
        open_chats(&mut opened, &["t1", "t2"]);
        let (before, target) = (tab_count(&mut opened), tab_of(&mut opened, "t1"));
        drop_row(&mut opened, "t2", TabDrop::Merge(target));
        assert_eq!(tab_count(&mut opened), before - 1, "t2's own tab is gone");
        assert_eq!(tab_of(&mut opened, "t2"), target, "t2 joined t1's tab");
        drop_row(&mut opened, "t3", TabDrop::Merge(target));
        assert_eq!(tab_count(&mut opened), before - 1, "no new tab");
        assert_eq!(tab_of(&mut opened, "t3"), target, "t3 opened beside t1");
    }

    #[gpui::test]
    fn a_row_of_an_open_chat_dropped_between_tabs_moves_its_tab_there(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        open_chats(&mut opened, &["t1", "t2"]);
        let moved = tab_of(&mut opened, "t2");
        drop_row(&mut opened, "t2", TabDrop::Reorder(0));
        let first = opened.shell.read_with(&opened.window, |shell, _| {
            shell.state.layout().tabs().first().map(layout::Tab::id)
        });
        assert_eq!(first, Some(moved), "t2's tab leads the strip");
    }

    fn add(
        id: u64,
        space: &str,
    ) -> layout::Command {
        layout::Command::AddTab {
            id: TabId(id),
            title: String::new(),
            space: layout::SpaceId::new(space),
        }
    }

    fn layout() -> Layout {
        let mut layout = Layout::default();
        for (id, space) in [(1, "p1"), (2, "p2"), (3, "p1"), (4, "")] {
            layout.apply(add(id, space)).unwrap();
        }
        layout
    }

    #[test]
    fn a_drop_between_two_known_different_spaces_is_refused_even_without_chats() {
        let layout = layout();
        let cases = [
            ((1, 2), true),
            ((1, 3), false),
            ((4, 2), false),
            ((2, 4), false),
        ];
        for ((tab, onto), refused) in cases {
            let crossing = crosses(
                space_of(&layout, TabId(tab)),
                space_of(&layout, TabId(onto)),
            );
            assert_eq!(crossing, refused, "{tab} onto {onto}");
        }
    }
}
