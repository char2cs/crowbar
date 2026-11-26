//! Folding directories of the Files panel.

use crate::reads::files::followed_workspace_id;
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
            let workspace = followed_workspace_id(state).to_owned();
            state
                .panels
                .expansions
                .of_mut(&workspace)
                .retain(|open| !within(open, directory));
        });
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use tokio::runtime::Handle;

    use super::*;
    use crate::live::Fleet;
    use crate::live::Hub;
    use crate::test_support::{FakeDaemon, FakeFiles, state};

    fn opened(directories: &[&str]) -> SidebarCommands {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        state
            .panels
            .expansions
            .of_mut("hw")
            .extend(directories.iter().map(|open| (*open).to_owned()));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        SidebarCommands {
            fleet: Fleet::new(&Handle::current(), &hub, (&transport, Arc::new(|_| {}))),
            hub,
            handle: Handle::current(),
            outcome: Arc::new(|_| {}),
            files: Arc::new(FakeFiles::default()),
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
            .read(|state| state.panels.expansions.paths("hw"));
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
