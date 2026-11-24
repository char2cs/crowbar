use std::rc::Rc;

pub(crate) trait ActiveSpace {
    fn active(&self) -> String;
}

impl ActiveSpace for store::Store {
    fn active(&self) -> String {
        self.active_project().unwrap_or_default()
    }
}

pub(crate) fn of_store(store: &Rc<store::Store>) -> Rc<dyn ActiveSpace> {
    Rc::<store::Store>::clone(store)
}

#[cfg(test)]
mod tests {
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
}
