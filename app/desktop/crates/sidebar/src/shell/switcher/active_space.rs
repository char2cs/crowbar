//! The space shown and what it holds.

use crate::shell::chats::ProjectTree;
use crate::shell::switcher::project_id::ProjectId;
use crate::shell::switcher::space_icon::SpaceIcon;

/// The space shown and what it holds.
pub trait ActiveSpace {
    /// The space shown, if any.
    fn active_project(&self) -> Option<ProjectId>;
    /// The chats tree of `project`.
    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree;
    /// The icon `project` shows.
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
    use crate::fixtures::DemoShell;

    #[test]
    fn only_the_active_space_holds_rows() {
        let data = DemoShell::reference();
        let active = data.active_project().expect("an active space");
        assert!(data.tree(&active).rows.len() > 1);
        assert_eq!(data.tree(&ProjectId::from("elsewhere")).rows.len(), 0);
        assert_eq!(data.space_icon(&active), SpaceIcon::Default);
    }
}
