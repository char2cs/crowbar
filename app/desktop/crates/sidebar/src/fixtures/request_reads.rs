//! What the demo data answers about running requests.

use crate::fixtures::demo_shell::DemoShell;
use crate::shell::IconTarget;
use crate::shell::Requests;

impl DemoShell {
    /// The same data with a request changing the icon of `target` still running.
    #[must_use]
    pub fn with_busy_icon(
        mut self,
        target: IconTarget,
    ) -> Self {
        self.busy_icons.push(target);
        self
    }

    /// The same data with a request adding `folder` still running.
    #[must_use]
    pub fn with_adding(
        mut self,
        folder: &str,
    ) -> Self {
        self.adding.push(folder.to_owned());
        self
    }
}

impl Requests for DemoShell {
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool {
        self.busy_icons.contains(target)
    }

    fn adding(
        &self,
        folder: &str,
    ) -> bool {
        self.adding.iter().any(|adding| adding == folder)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::RowId;

    #[test]
    fn only_a_target_marked_busy_answers_busy() {
        let busy = IconTarget::Repo(RowId::from("demo"));
        let data = DemoShell::reference().with_busy_icon(busy.clone());
        assert!(data.icon_busy(&busy));
        assert!(!DemoShell::reference().icon_busy(&busy));
    }

    #[test]
    fn only_a_folder_being_added_answers_adding() {
        let data = DemoShell::reference().with_adding("/w/app");
        assert!(data.adding("/w/app"));
        assert!(!data.adding("/w/other"));
    }
}
