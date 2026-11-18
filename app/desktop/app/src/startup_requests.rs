use gpui::App;
use startup::{StartupActions, StartupCommand};

use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

#[derive(Debug, Clone)]
pub(crate) struct StartupRequests(pub(crate) Requests);

impl StartupActions for StartupRequests {
    fn perform(
        &self,
        command: StartupCommand,
        _cx: &mut App,
    ) {
        send(&self.0, ShellRequest::Startup(command));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn a_startup_command_becomes_a_shell_request(cx: &mut TestAppContext) {
        let (requests, queue) = async_channel::unbounded();
        let actions = StartupRequests(requests);
        cx.update(|cx| actions.perform(StartupCommand::Retry, cx));
        assert_eq!(
            queue.try_recv(),
            Ok(ShellRequest::Startup(StartupCommand::Retry)),
            "queued for the shell"
        );
    }
}
