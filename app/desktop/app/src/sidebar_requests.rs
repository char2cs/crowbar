use crate::request_queue::Requests;
use crate::shell_request::ShellRequest;

#[derive(Debug, Clone)]
pub(crate) struct SidebarRequests {
    pub(crate) requests: Requests,
    pub(crate) data: shell_data::SidebarCommands,
}

impl sidebar::CommandSink for SidebarRequests {
    fn dispatch(
        &self,
        command: sidebar::Command,
    ) -> Result<(), sidebar::Error> {
        if shell_data::ShellData::serves(&command) {
            return self.data.dispatch(command);
        }
        self.requests
            .try_send(ShellRequest::Sidebar(command))
            .map_err(|_| sidebar::Error::Refused {
                reason: "the shell is closing".into(),
            })
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use protocol::{DaemonRequest, DaemonTransport, RelayEndpoint, RelayScope, Responder};
    use sidebar::{Command, CommandSink as _, ProjectId};

    use super::*;

    struct NoDaemon;

    impl DaemonTransport for NoDaemon {
        fn request(
            &self,
            _request: DaemonRequest,
            respond: Responder,
        ) {
            respond(Err(protocol::Error::Timeout));
        }

        fn relay_endpoint(
            &self,
            _scope: &RelayScope,
        ) -> protocol::Result<RelayEndpoint> {
            Err(protocol::Error::Timeout)
        }
    }

    fn desk(dir: &std::path::Path) -> shell_data::HostDesk {
        let registry = Arc::new(transport::HostRegistry::open(dir.join("hosts.json")).unwrap());
        let tokens = Arc::new(transport::MemoryTokenStore::default());
        shell_data::HostDesk {
            book: Arc::new(crate::transport_book::TransportBook::new(registry, tokens)),
            notes: Arc::new(shell_data::HostNotes::open(dir.join("notes.json")).unwrap()),
        }
    }

    fn data(
        runtime: &tokio::runtime::Runtime,
        dir: &std::path::Path,
    ) -> shell_data::ShellData {
        let transport: Arc<dyn DaemonTransport> = Arc::new(NoDaemon);
        let wiring = shell_data::ShellWiring {
            desk: desk(dir),
            signal: Arc::new(|| {}),
            outcome: Arc::new(|_| {}),
        };
        shell_data::ShellData::start(runtime.handle(), &transport, Vec::new(), wiring)
    }

    #[test]
    fn shell_commands_reach_the_queue_and_data_commands_go_to_the_data_layer() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .build()
            .unwrap();
        let dir = tempfile::tempdir().unwrap();
        let data = data(&runtime, dir.path());
        let (sender, receiver) = async_channel::unbounded();
        let sink = SidebarRequests {
            requests: sender,
            data: data.commands(),
        };
        sink.dispatch(Command::OpenSettings).unwrap();
        assert_eq!(
            receiver.try_recv().unwrap(),
            ShellRequest::Sidebar(Command::OpenSettings),
            "the shell's own command"
        );
        let refused = sink.dispatch(Command::NewThread(ProjectId::from("p")));
        assert!(refused.is_err(), "no space is loaded to start a thread in");
        assert!(receiver.try_recv().is_err(), "a data command is not queued");
        drop(receiver);
        assert!(
            sink.dispatch(Command::ImportRepo).is_err(),
            "a closed shell refuses"
        );
    }
}
