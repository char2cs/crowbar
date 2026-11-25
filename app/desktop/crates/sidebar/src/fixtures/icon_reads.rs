//! What the demo data answers about running icon requests.

use crate::fixtures::demo_shell::DemoShell;
use crate::shell::IconRequests;
use crate::shell::IconTarget;

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
}

impl IconRequests for DemoShell {
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool {
        self.busy_icons.contains(target)
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
}
