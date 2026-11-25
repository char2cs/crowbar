use std::rc::Rc;

pub(crate) trait ActiveSpace {
    fn active(&self) -> String;

    fn of_chat(
        &self,
        chat: &str,
    ) -> Option<String>;

    fn workspace_of_chat(
        &self,
        chat: &str,
    ) -> Option<String>;
}

impl ActiveSpace for store::Store {
    fn active(&self) -> String {
        self.active_project().unwrap_or_default()
    }

    fn of_chat(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.chat_project(chat)
    }

    fn workspace_of_chat(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.chat_workspace(chat)
    }
}

pub(crate) fn of_store(store: &Rc<store::Store>) -> Rc<dyn ActiveSpace> {
    Rc::<store::Store>::clone(store)
}

#[cfg(test)]
mod tests {
    use crate::panes::kinds::core_panes::{chat_params, chat_type};
    use crate::test_support::{open_chat, shell_rig};

    #[test]
    fn the_shell_shows_the_tab_of_the_active_space_and_the_stage_of_another() {
        let mut rig = shell_rig();
        rig.space.select("p1");
        open_chat(&mut rig.shell, "t1");
        let shown = rig
            .shell
            .showing()
            .map(|tab| (tab.id(), tab.focused().cloned()));
        assert!(shown.is_some(), "p1 shows its chat");
        rig.space.select("p2");
        assert!(rig.shell.showing().is_none(), "p2 shows its stage");
        rig.space.select("p1");
        let back = rig
            .shell
            .showing()
            .map(|tab| (tab.id(), tab.focused().cloned()));
        assert_eq!(back, shown, "p1 again, with its focus");
    }

    #[test]
    fn a_chat_of_another_space_opens_as_that_space_s_tab_without_being_shown() {
        let mut rig = shell_rig();
        rig.space.select("p1");
        rig.space.know(("c2", "p2"));
        open_chat(&mut rig.shell, "t1");
        let shown = rig.shell.showing().map(layout::Tab::id);
        let (Ok(type_id), Ok(params)) = (chat_type(), chat_params("c2")) else {
            panic!("a chat pane");
        };
        rig.shell.open_view(type_id, params);
        assert_eq!(
            rig.shell.showing().map(layout::Tab::id),
            shown,
            "p1 keeps its tab"
        );
        let layout = rig.shell.layout();
        let p2 = layout.showing("p2").map(|tab| tab.pane_ids().len());
        assert_eq!(p2, Some(1), "p2 remembers the chat as its tab");
    }
}
