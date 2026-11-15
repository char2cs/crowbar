//! Folding directories of the Files panel.

use crate::sidebar_commands::SidebarCommands;

fn within(
    open: &str,
    directory: &str,
) -> bool {
    directory.is_empty()
        || open == directory
        || open
            .strip_prefix(directory)
            .is_some_and(|rest| rest.starts_with('/'))
}

impl SidebarCommands {
    pub(crate) fn collapse_directory(
        &self,
        directory: &str,
    ) {
        self.hub.edit(|state| {
            state
                .panels
                .expanded
                .retain(|open| !within(open, directory));
        });
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use tokio::runtime::Handle;

    use super::*;
    use crate::fleet::Fleet;
    use crate::hub::Hub;
    use crate::test_support::{FakeDaemon, state};

    fn opened(directories: &[&str]) -> SidebarCommands {
        let (mut state, daemon) = state();
        state
            .panels
            .expanded
            .extend(directories.iter().map(|open| (*open).to_owned()));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        SidebarCommands {
            fleet: Fleet::new(&Handle::current(), &hub, &transport),
            hub,
            handle: Handle::current(),
            outcome: Arc::new(|_| {}),
        }
    }

    const OPEN: [&str; 4] = ["src", "src/ui", "srcery", "docs"];

    const CASES: [(&str, &[&str]); 4] = [
        ("src", &["docs", "srcery"]),
        ("src/ui", &["docs", "src", "srcery"]),
        ("", &[]),
        ("other", &["docs", "src", "src/ui", "srcery"]),
    ];

    fn folded(directory: &str) -> Vec<String> {
        let commands = opened(&OPEN);
        commands.collapse_directory(directory);
        let mut now: Vec<String> = commands
            .hub
            .read(|state| state.panels.expanded.iter().cloned().collect());
        now.sort();
        now
    }

    #[tokio::test]
    async fn a_directory_folds_with_its_subdirectories_and_a_namesake_stays() {
        let folds: Vec<Vec<String>> = CASES
            .iter()
            .map(|(directory, _)| folded(directory))
            .collect();
        let wanted: Vec<Vec<String>> = CASES
            .iter()
            .map(|(_, left)| left.iter().map(|open| (*open).to_owned()).collect())
            .collect();
        assert_eq!(folds, wanted);
    }
}
