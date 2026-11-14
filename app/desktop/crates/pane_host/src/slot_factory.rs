use std::cell::Cell;
use std::sync::Arc;

use protocol::{DaemonTransport, PaneId, PaneKind, RelayScope};
use webview::{
    Bounds, IpcHandler, ParentWindow, Visibility, WebViewConfig, WebViewHost, WebViewKind,
};

use crate::bootstrap::bootstrap_script;
use crate::error::Result;
use crate::ipc_message::IpcMessage;
use crate::pane_host_config::PaneHostConfig;
use crate::slot::Slot;
use crate::slot_id::SlotId;
use crate::slot_owner::SlotOwner;

/// Creates pooled web views: wires their ipc to the pane that owns them and injects the
/// bootstrap script with the relay endpoint of the new slot.
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

    pub(crate) fn create(
        &self,
        parent: ParentWindow,
        kind: WebViewKind,
        url: &str,
        bounds: Bounds,
    ) -> Result<Slot> {
        let id = SlotId(self.next.get());
        self.next.set(id.0 + 1);
        let owner = SlotOwner::default();
        let config = WebViewConfig {
            url: url.to_owned(),
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
        let (pane_kind, extension) = match kind {
            WebViewKind::Core => (PaneKind::Core, None),
            WebViewKind::Extension(extension) => (PaneKind::Extension, Some(extension.clone())),
        };
        let scope = PaneId::new(format!("slot-{}", id.0))
            .ok()
            .map(|pane| RelayScope {
                host: self.config.host.clone(),
                pane,
                kind: pane_kind,
                extension,
            });
        let relay = scope.and_then(|scope| self.transport.relay_endpoint(&scope).ok());
        bootstrap_script(&self.config.scheme, &self.config.host, relay.as_ref())
    }

    fn ipc_for(&self, owner: &SlotOwner) -> IpcHandler {
        let owner = owner.clone();
        let sink = self.sink.clone();
        Arc::new(move |text| {
            let Some(pane) = owner.current() else {
                return;
            };
            if sink.try_send(IpcMessage { pane, text }).is_err() {
                tracing::warn!("pane host inbox is closed; dropping a pane message");
            }
        })
    }
}

#[cfg(test)]
#[path = "slot_factory_tests.rs"]
mod tests;
