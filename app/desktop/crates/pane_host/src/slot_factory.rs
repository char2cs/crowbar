use std::cell::Cell;
use std::sync::Arc;

use protocol::{DaemonTransport, PaneId, PaneOrigin, RelayScope};
use webview::{
    IpcHandler, Visibility, WebViewConfig, WebViewHost, WebViewKind,
};

use crate::bootstrap::bootstrap_script;
use crate::error::Result;
use crate::ipc_message::IpcMessage;
use crate::pane_host_config::PaneHostConfig;
use crate::slot::Slot;
use crate::slot_id::SlotId;
use crate::slot_owner::SlotOwner;
use crate::slot_request::SlotRequest;

pub(crate) struct SlotFactory {
    webviews: WebViewHost,
    transport: Arc<dyn DaemonTransport>,
    config: PaneHostConfig,
    sink: async_channel::Sender<IpcMessage>,
    next: Cell<u64>,
}

impl SlotFactory {
    pub(crate) fn new(
        webviews: WebViewHost,
        transport: Arc<dyn DaemonTransport>,
        config: PaneHostConfig,
        sink: async_channel::Sender<IpcMessage>,
    ) -> Self {
        Self {
            webviews,
            transport,
            config,
            sink,
            next: Cell::new(0),
        }
    }

    pub(crate) fn create(&self, request: SlotRequest) -> Result<Slot> {
        let SlotRequest {
            parent,
            kind,
            url,
            bounds,
        } = request;
        let id = SlotId(self.next.get());
        self.next.set(id.0 + 1);
        let owner = SlotOwner::default();
        let config = WebViewConfig {
            url,
            bounds,
            kind: kind.clone(),
            visibility: Visibility::Hidden,
            initialization_script: Some(self.script_for(id, &kind)),
            on_ipc: self.ipc_for(&owner),
        };
        let handle = self.webviews.create_in(parent, config)?;
        Ok(Slot {
            id,
            kind,
            handle,
            owner,
        })
    }

    fn script_for(&self, id: SlotId, kind: &WebViewKind) -> String {
        let origin = match kind {
            WebViewKind::Core => PaneOrigin::Core,
            WebViewKind::Extension(extension) => PaneOrigin::Extension(extension.clone()),
        };
        let scope = PaneId::new(format!("slot-{}", id.0))
            .ok()
            .map(|pane| RelayScope::new(self.config.host.clone(), pane, origin));
        let relay = scope.and_then(|scope| self.transport.relay_endpoint(&scope).ok());
        bootstrap_script(&self.config.scheme, &self.config.host, relay.as_ref())
    }

    fn ipc_for(&self, owner: &SlotOwner) -> IpcHandler {
        let owner = owner.clone();
        let sink = self.sink.clone();
        Arc::new(move |text| deliver(&owner, &sink, text))
    }
}

fn deliver(owner: &SlotOwner, sink: &async_channel::Sender<IpcMessage>, text: String) {
    let Some(pane) = owner.current() else {
        return;
    };
    if sink.try_send(IpcMessage { pane, text }).is_err() {
        tracing::warn!("pane host inbox is closed; dropping a pane message");
    }
}

#[cfg(test)]
#[path = "slot_factory_tests.rs"]
mod tests;
