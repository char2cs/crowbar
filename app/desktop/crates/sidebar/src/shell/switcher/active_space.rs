use crate::shell::chats::ProjectTree;
use crate::shell::switcher::project_id::ProjectId;
use crate::shell::switcher::space_icon::SpaceIcon;

pub trait ActiveSpace {
    fn active_project(&self) -> Option<ProjectId>;
    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree;
    fn space_icon(
        &self,
        _project: &ProjectId,
    ) -> SpaceIcon {
        SpaceIcon::Default
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::DemoShell;

    #[test]
    fn only_the_active_space_holds_rows() {
        let data = DemoShell::reference();
        let active = data.active_project().expect("an active space");
        assert!(data.tree(&active).rows.len() > 1);
        assert_eq!(data.tree(&ProjectId::from("elsewhere")).rows.len(), 0);
        assert_eq!(data.space_icon(&active), SpaceIcon::Default);
    }
}
