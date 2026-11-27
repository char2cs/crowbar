use std::sync::Arc;

use tokio::runtime::Handle;

use super::{FakeDaemon, FakeFiles, state};
use crate::live::Fleet;
use crate::live::Hub;
use crate::live::OutcomeSink;
use crate::sidebar_commands::SidebarCommands;

pub(crate) fn followed_commands(
    outcome: OutcomeSink,
    files: &Arc<FakeFiles>,
) -> (SidebarCommands, Arc<FakeDaemon>) {
    let (mut state, daemon) = state();
    state.panels.follow(Some("hc".into()));
    let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
    let hub = Hub::new(state, Arc::new(|| {}));
    let commands = SidebarCommands {
        fleet: Fleet::new(&Handle::current(), &hub, (&transport, Arc::new(|_| {}))),
        hub,
        handle: Handle::current(),
        outcome,
        files: Arc::<FakeFiles>::clone(files),
    };
    (commands, daemon)
}
