use std::sync::Arc;

use model::{Layout, LayoutHistory};
use pane_host::{
    DirBundle, ExtensionBundles, HostEvent, IpcMessage, LayoutSnapshot, OriginTable, PaneHost,
    PaneHostConfig, PaneProtocol,
};
use protocol::{DaemonTransport, HostId};
use webview::{Bounds, HostConfig, ParentWindow, WebViewHost};

use crate::app_options::AppOptions;
use crate::core_panes::{first_commands, register_core_panes};
use crate::event_commands::commands_for;

const SCHEME: &str = "crowbar";

/// The shell's logic without any window: the layout, and the pane host that follows it.
pub(crate) struct ShellState {
    layout: LayoutHistory,
    host: PaneHost,
    last_content: Option<(ParentWindow, Bounds)>,
}

impl std::fmt::Debug for ShellState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ShellState").finish_non_exhaustive()
    }
}

impl ShellState {
    pub(crate) fn new(options: &AppOptions, transport: Arc<dyn DaemonTransport>) -> anyhow::Result<Self> {
        let origins = OriginTable::default();
        let protocol = PaneProtocol::new(
            SCHEME,
            Arc::clone(&transport),
            Arc::new(DirBundle::new(&options.panes_dir)),
            ExtensionBundles::default(),
            origins.clone(),
        );
        let webviews = WebViewHost::native(HostConfig {
            scheme: SCHEME.into(),
            protocol: Arc::new(protocol),
            data_dir: options.data_dir.clone(),
        })?;
        let config = PaneHostConfig {
            scheme: SCHEME.into(),
            host: HostId::new("local")?,
            max_idle_slots: 2,
        };
        let host = PaneHost::new(config, webviews, transport, origins);
        Self::with_host(host, &options.chat_entry)
    }

    pub(crate) fn with_host(mut host: PaneHost, chat_entry: &str) -> anyhow::Result<Self> {
        register_core_panes(host.registry_mut(), chat_entry)?;
        let mut layout = LayoutHistory::new(Layout::default());
        for command in first_commands()? {
            layout.apply(command)?;
        }
        Ok(Self { layout, host, last_content: None })
    }


    pub(crate) fn inbox(&self) -> async_channel::Receiver<IpcMessage> {
        self.host.inbox()
    }

    /// Re-places every pane after the content area moved or resized.
    pub(crate) fn set_content(&mut self, parent: ParentWindow, content: Bounds) {
        self.last_content = Some((parent, content));
        self.sync();
    }

    pub(crate) fn handle_ipc(&mut self, message: IpcMessage) {
        match self.host.handle_ipc(message) {
            Ok(events) => self.apply(&events),
            Err(error) => tracing::warn!(%error, "handling a pane message failed"),
        }
    }

    fn apply(&mut self, events: &[HostEvent]) {
        for command in commands_for(events) {
            if let Err(error) = self.layout.apply(command) {
                tracing::warn!(%error, "pane event could not be applied");
            }
        }
        for event in events {
            tracing::debug!(?event, "pane event");
        }
    }

    fn sync(&mut self) {
        let Some((parent, content)) = self.last_content else {
            return;
        };
        let snapshot = LayoutSnapshot::from_layout(self.layout.layout(), parent, content);
        match self.host.sync(&snapshot) {
            Ok(events) => self.apply(&events),
            Err(error) => tracing::error!(%error, "syncing panes with the layout failed"),
        }
    }
}

#[cfg(test)]
#[path = "shell_state_tests.rs"]
mod tests;
