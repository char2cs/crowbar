use std::collections::HashSet;

use sidebar::{Presence, RowId};

#[derive(Debug, Default)]
pub(crate) struct OpenChats(HashSet<RowId>);

impl OpenChats {
    pub(crate) fn replace(
        &mut self,
        chats: HashSet<RowId>,
    ) -> bool {
        if self.0 == chats {
            return false;
        }
        self.0 = chats;
        true
    }

    pub(crate) fn presence_of(
        &self,
        row: &RowId,
    ) -> Presence {
        if self.0.contains(row) {
            return Presence::Open;
        }
        Presence::Absent
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use sidebar::DemoShell;

    use super::*;
    use crate::sidebar::Sidebar;
    use crate::test_support::open;

    fn presence(
        sidebar: &gpui::Entity<Sidebar>,
        row: &str,
        cx: &mut gpui::VisualTestContext,
    ) -> Presence {
        sidebar.read_with(cx, |this, _| this.open_chats.presence_of(&RowId::from(row)))
    }

    #[gpui::test]
    fn only_the_chats_the_shell_last_reported_open_rest_on_the_open_fill(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| {
            this.set_open_chats([RowId::from("chat")], cx);
        });
        let shown = (
            presence(&sidebar, "chat", cx),
            presence(&sidebar, "main", cx),
        );
        sidebar.update(cx, |this, cx| this.set_open_chats([], cx));
        let closed = presence(&sidebar, "chat", cx);
        assert_eq!(
            (shown, closed),
            ((Presence::Open, Presence::Absent), Presence::Absent)
        );
    }
}
