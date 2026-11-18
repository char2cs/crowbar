use std::sync::Arc;

use tokio::runtime::Handle;

use super::{FakeDaemon, FakeFiles, state};
use crate::fleet::Fleet;
use crate::hub::Hub;
use crate::outcome::OutcomeSink;
use crate::sidebar_commands::SidebarCommands;

/// Commands over a fake daemon with the chat `hc` followed, saying their outcomes to `outcome`.
pub(crate) fn followed_commands(
    outcome: OutcomeSink,
    files: &Arc<FakeFiles>,
) -> (SidebarCommands, Arc<FakeDaemon>) {
    let (mut state, daemon) = state();
    state.panels.follow(Some("hc".into()));
    let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
    let hub = Hub::new(state, Arc::new(|| {}));
    let commands = SidebarCommands {
        fleet: Fleet::new(&Handle::current(), &hub, &transport),
        hub,
        handle: Handle::current(),
        outcome,
        files: Arc::<FakeFiles>::clone(files),
    };
    (commands, daemon)
}
