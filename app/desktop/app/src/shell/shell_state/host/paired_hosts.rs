use protocol::{DisplayText, HostId, PairedHost};
use store::HostSpec;

use crate::shell::shell_state::ShellState;

fn paired(spec: &HostSpec) -> Option<PairedHost> {
    let endpoint = spec.endpoint.as_deref()?;
    let host = (
        HostId::new(&spec.id),
        DisplayText::new(&spec.name),
        DisplayText::new(endpoint),
    );
    if let (Ok(id), Ok(name), Ok(endpoint)) = host {
        return Some(PairedHost { id, name, endpoint });
    }
    tracing::warn!(host = %spec.id, "a paired host cannot be described to panes");
    None
}

impl ShellState {
    pub(crate) fn set_paired_hosts(
        &mut self,
        hosts: &[HostSpec],
    ) {
        let hosts = hosts.iter().filter_map(paired).collect();
        if let Err(error) = self.host.set_paired_hosts(hosts) {
            tracing::warn!(%error, "sending the paired hosts to panes failed");
        }
    }
}

#[cfg(test)]
mod tests {
    use store::HostSpec;

    use crate::test_support::{shell_rig, synced_chat};

    fn told(
        rig: &crate::test_support::ShellRig,
        frame: &str,
    ) -> bool {
        rig.mock.calls().into_iter().any(|call| match call {
            webview::Call::Eval { script, .. } => script.contains(frame),
            _ => false,
        })
    }

    #[test]
    fn a_mounted_pane_learns_the_hosts_the_shell_paired() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let register = protocol::PaneFrame::Register {
            type_id: crate::panes::kinds::core_panes::chat_type().unwrap(),
            sdk: protocol::SDK_VERSION,
        };
        let text = protocol::encode(&register).unwrap();
        rig.shell.handle_ipc(panehost::IpcMessage { pane, text });
        let rigremote = HostSpec {
            id: "h1".into(),
            name: "rigremote".into(),
            endpoint: Some("127.0.0.1:4747".into()),
        };
        rig.shell.set_paired_hosts(&[rigremote]);
        let frame = r#"{\"sdk\":1,\"kind\":\"hosts\",\"hosts\":[{\"id\":\"h1\",\"name\":\"rigremote\",\"endpoint\":\"127.0.0.1:4747\"}]}"#;
        assert!(told(&rig, frame), "{:?}", rig.mock.calls());
    }
}
