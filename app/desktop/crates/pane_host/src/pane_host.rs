use std::collections::BTreeMap;
use std::sync::Arc;

use protocol::{
    DaemonTransport, ExtensionStatus, KeyBinding, Lifecycle, PaneFrame, PaneId, PaneTypeId,
    ShellFrame, StatusKind, decode_pane_frame, encode,
};
use webview::{Bounds, ParentWindow, WebViewHost, WebViewKind};

use crate::entry_url::entry_url;
use crate::error::{Error, Result};
use crate::host_event::HostEvent;
use crate::ipc_message::IpcMessage;
use crate::layout_snapshot::LayoutSnapshot;
use crate::origin::Origin;
use crate::origin_table::OriginTable;
use crate::overlay::Overlay;
use crate::pane_display::PaneDisplay;
use crate::pane_host_config::PaneHostConfig;
use crate::pane_lifecycle::PaneLifecycle;
use crate::pane_phase::PanePhase;
use crate::pane_registry::PaneRegistry;
use crate::pane_runtime::PaneRuntime;
use crate::pane_type::PaneType;
use crate::slot_factory::SlotFactory;
use crate::slot_id::SlotId;
use crate::slot_pool::SlotPool;
use crate::visible_pane::VisiblePane;

/// Owns every pane web view: places them from the layout, pools and parks them, delivers
/// lifecycle frames, and turns what pages post into [`HostEvent`]s.
///
/// The host keeps no copy of pane state. The model owns it; the host reads it from each
/// [`LayoutSnapshot`] and reports changes as events.
pub struct PaneHost {
    registry: PaneRegistry,
    factory: SlotFactory,
    pool: SlotPool,
    origins: OriginTable,
    panes: BTreeMap<PaneId, PaneRuntime>,
    keybindings: Vec<KeyBinding>,
    overlay: Overlay,
    focused: Option<PaneId>,
    inbox: async_channel::Receiver<IpcMessage>,
    config: PaneHostConfig,
}

impl std::fmt::Debug for PaneHost {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("PaneHost")
            .field("panes", &self.panes.len())
            .finish_non_exhaustive()
    }
}

impl PaneHost {
    /// Builds a host. `origins` must be the table the [`PaneProtocol`](crate::PaneProtocol)
    /// of `webviews` reads.
    #[must_use]
    pub fn new(
        config: PaneHostConfig,
        webviews: WebViewHost,
        transport: Arc<dyn DaemonTransport>,
        origins: OriginTable,
    ) -> Self {
        let (sink, inbox) = async_channel::unbounded();
        Self {
            registry: PaneRegistry::default(),
            pool: SlotPool::new(config.max_idle_slots),
            factory: SlotFactory::new(webviews, transport, config.clone(), sink),
            origins,
            panes: BTreeMap::new(),
            keybindings: Vec::new(),
            overlay: Overlay::Inactive,
            focused: None,
            inbox,
            config,
        }
    }

    /// The pane type registry.
    #[must_use]
    pub fn registry(&self) -> &PaneRegistry {
        &self.registry
    }

    /// Mutable access to the registry, to register or remove types.
    pub fn registry_mut(&mut self) -> &mut PaneRegistry {
        &mut self.registry
    }

    /// The queue of messages pages post. Await it on the main thread and pass each message to
    /// [`PaneHost::handle_ipc`].
    #[must_use]
    pub fn inbox(&self) -> async_channel::Receiver<IpcMessage> {
        self.inbox.clone()
    }

    /// Number of live web views, in use or idle.
    #[must_use]
    pub fn web_view_count(&self) -> usize {
        self.pool.len()
    }

    /// What the shell draws in `pane`'s rectangle.
    #[must_use]
    pub fn display(&self, pane: &PaneId) -> PaneDisplay {
        let Some(runtime) = self.panes.get(pane) else {
            return PaneDisplay::Empty;
        };
        let type_id = &runtime.instance.type_id;
        match &runtime.phase {
            _ if self.registry.get(type_id).is_none() => PaneDisplay::Placeholder {
                type_id: type_id.clone(),
            },
            PanePhase::Failed(message) => PaneDisplay::Error {
                message: message.clone(),
            },
            _ if runtime.slot.is_some() && runtime.visible => PaneDisplay::Web,
            _ => PaneDisplay::Empty,
        }
    }

    /// Reconciles web views with the layout: creates, moves, shows, hides, parks and destroys.
    ///
    /// # Errors
    /// [`Error::WebView`] when a web view cannot be created or driven.
    pub fn sync(&mut self, snapshot: &LayoutSnapshot) -> Result<Vec<HostEvent>> {
        let mut events = Vec::new();
        self.retire_removed(snapshot)?;
        for instance in &snapshot.panes {
            let runtime = self
                .panes
                .entry(instance.pane_id.clone())
                .or_insert_with(|| PaneRuntime::new(instance.clone()));
            runtime.instance = instance.clone();
        }
        self.hide_unlisted(snapshot, &mut events)?;
        for visible in &snapshot.visible {
            self.show(snapshot.parent, visible)?;
        }
        self.focused.clone_from(&snapshot.focused);
        self.route_activation()?;
        Ok(events)
    }

    /// Handles one message a page posted.
    ///
    /// A message from a pane that no longer has a web view is stale and ignored. A malformed
    /// frame fails the pane: its frame shows the error and the window is unaffected.
    ///
    /// # Errors
    /// [`Error::WebView`] when answering the pane fails.
    pub fn handle_ipc(&mut self, message: IpcMessage) -> Result<Vec<HostEvent>> {
        let live = self
            .panes
            .get(&message.pane)
            .is_some_and(|runtime| runtime.slot.is_some());
        if !live {
            return Ok(Vec::new());
        }
        match decode_pane_frame(&message.text) {
            Ok(frame) => self.on_frame(message.pane, frame),
            Err(error) => {
                self.fail(&message.pane, format!("invalid message: {error}"))?;
                Ok(Vec::new())
            },
        }
    }

    /// Sends a frame to a pane's page.
    ///
    /// # Errors
    /// [`Error::NotShown`] when the pane has no web view, [`Error::Frame`] when the frame
    /// cannot be encoded and [`Error::WebView`] when the web view layer fails.
    pub fn send(&self, pane: &PaneId, frame: &ShellFrame) -> Result<()> {
        let slot = self
            .panes
            .get(pane)
            .and_then(|runtime| runtime.slot)
            .and_then(|id| self.pool.get(id))
            .ok_or_else(|| Error::NotShown(pane.clone()))?;
        Ok(slot.handle.post_message(&encode(frame)?)?)
    }

    /// Hides or shows every pane for a native overlay. No pane is told.
    ///
    /// # Errors
    /// [`Error::WebView`] when a web view cannot be driven.
    pub fn set_overlay(&mut self, overlay: Overlay) -> Result<()> {
        self.overlay = overlay;
        for runtime in self.panes.values().filter(|runtime| runtime.visible) {
            self.set_native_visibility(runtime)?;
        }
        Ok(())
    }

    /// Replaces the active keybindings and pushes them to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`] when a pane cannot be reached.
    pub fn set_keybindings(&mut self, bindings: Vec<KeyBinding>) -> Result<()> {
        self.keybindings = bindings;
        let frame = ShellFrame::Keybindings {
            bindings: self.keybindings.clone(),
        };
        for (id, runtime) in &self.panes {
            if runtime.is_mounted() {
                self.send(id, &frame)?;
            }
        }
        Ok(())
    }

    /// Applies an extension's new status to its panes: errors in their frames while it is
    /// unavailable or disabled, a fresh `mount` with the saved state when it is back.
    ///
    /// # Errors
    /// [`Error::WebView`] when a pane cannot be reached.
    pub fn extension_status(&mut self, status: &ExtensionStatus) -> Result<()> {
        let owned: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|(_, runtime)| {
                runtime.instance.type_id.extension().as_ref() == Some(&status.extension_id)
            })
            .map(|(id, _)| id.clone())
            .collect();
        for pane in owned {
            self.apply_status(&pane, status)?;
        }
        Ok(())
    }

    /// Gives keyboard focus back to the shell, for example after a click on native chrome.
    ///
    /// # Errors
    /// [`Error::WebView`] when the web view cannot be driven.
    pub fn focus_shell(&self) -> Result<()> {
        let slot = self
            .focused
            .as_ref()
            .and_then(|pane| self.panes.get(pane))
            .and_then(|runtime| runtime.slot)
            .and_then(|id| self.pool.get(id));
        Ok(slot.map_or(Ok(()), |slot| slot.handle.focus_parent())?)
    }
}

impl PaneHost {
    fn retire_removed(&mut self, snapshot: &LayoutSnapshot) -> Result<()> {
        let kept: Vec<&PaneId> = snapshot
            .panes
            .iter()
            .map(|instance| &instance.pane_id)
            .collect();
        let gone: Vec<PaneId> = self
            .panes
            .keys()
            .filter(|id| !kept.contains(id))
            .cloned()
            .collect();
        for pane in gone {
            self.send_lifecycle(&pane, Lifecycle::Destroy)?;
            self.release_slot(&pane)?;
            self.panes.remove(&pane);
        }
        Ok(())
    }

    fn hide_unlisted(
        &mut self,
        snapshot: &LayoutSnapshot,
        events: &mut Vec<HostEvent>,
    ) -> Result<()> {
        let listed: Vec<&PaneId> = snapshot
            .visible
            .iter()
            .map(|visible| &visible.pane)
            .collect();
        let leaving: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|(id, runtime)| runtime.visible && !listed.contains(id))
            .map(|(id, _)| id.clone())
            .collect();
        for pane in leaving {
            self.leave(&pane, events)?;
        }
        Ok(())
    }

    fn lifecycle_of(&self, type_id: &PaneTypeId) -> PaneLifecycle {
        self.registry
            .get(type_id)
            .map_or(PaneLifecycle::Parkable, |pane_type| pane_type.lifecycle)
    }

    fn leave(&mut self, pane: &PaneId, events: &mut Vec<HostEvent>) -> Result<()> {
        let Some(runtime) = self.panes.get(pane) else {
            return Ok(());
        };
        match self.lifecycle_of(&runtime.instance.type_id) {
            PaneLifecycle::KeepAlive => self.hide_in_place(pane),
            PaneLifecycle::Parkable => {
                self.send_lifecycle(pane, Lifecycle::Park)?;
                self.release_slot(pane)
            },
            PaneLifecycle::DestroyOnHide => {
                self.send_lifecycle(pane, Lifecycle::Destroy)?;
                self.release_slot(pane)?;
                self.discard_state(pane, events);
                Ok(())
            },
        }
    }

    fn hide_in_place(&mut self, pane: &PaneId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.visible = false;
        match runtime.slot.and_then(|id| self.pool.get(id)) {
            Some(slot) => Ok(slot.handle.hide()?),
            None => Ok(()),
        }
    }

    fn discard_state(&mut self, pane: &PaneId, events: &mut Vec<HostEvent>) {
        if let Some(runtime) = self.panes.get_mut(pane) {
            runtime.discard_state = true;
            events.push(HostEvent::StateDiscarded { pane: pane.clone() });
        }
    }

    fn send_lifecycle(&mut self, pane: &PaneId, event: Lifecycle) -> Result<()> {
        let Some(runtime) = self.panes.get(pane).filter(|runtime| runtime.is_mounted()) else {
            return Ok(());
        };
        let frame = runtime.lifecycle_frame(event);
        self.send(pane, &frame)
    }

    fn release_slot(&mut self, pane: &PaneId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        let Some(slot_id) = runtime.slot else {
            return Ok(());
        };
        runtime.detach();
        let Some(slot) = self.pool.get(slot_id) else {
            return Ok(());
        };
        slot.handle.hide()?;
        slot.handle.navigate("about:blank")?;
        self.origins.remove(slot.handle.id());
        self.pool.release(slot_id);
        Ok(())
    }

    fn show(&mut self, parent: ParentWindow, visible: &VisiblePane) -> Result<()> {
        let Some(runtime) = self.panes.get(&visible.pane) else {
            return Ok(());
        };
        let Some(pane_type) = self.registry.get(&runtime.instance.type_id).cloned() else {
            return self.release_slot(&visible.pane);
        };
        match runtime.slot {
            Some(slot) => self
                .pool
                .get(slot)
                .map_or(Ok(()), |slot| Ok(slot.handle.set_bounds(visible.bounds)?))?,
            None => self.assign_slot(parent, &pane_type, visible)?,
        }
        if let Some(runtime) = self.panes.get_mut(&visible.pane) {
            runtime.visible = true;
        }
        self.refresh_native_visibility(&visible.pane)
    }

    fn assign_slot(
        &mut self,
        parent: ParentWindow,
        pane_type: &PaneType,
        visible: &VisiblePane,
    ) -> Result<()> {
        let kind = match pane_type.id.extension() {
            Some(extension) => WebViewKind::Extension(extension),
            None => WebViewKind::Core,
        };
        let url = entry_url(&self.config.scheme, &self.config.host, pane_type);
        let slot_id = self.take_slot(parent, kind, &url, visible.bounds)?;
        let Some(slot) = self.pool.get(slot_id) else {
            return Ok(());
        };
        slot.owner.assign(visible.pane.clone());
        self.origins
            .set(slot.handle.id(), Origin::of(&pane_type.id));
        if let Some(runtime) = self.panes.get_mut(&visible.pane) {
            runtime.slot = Some(slot_id);
            runtime.phase = PanePhase::Loading;
            runtime.activated = false;
        }
        Ok(())
    }

    fn take_slot(
        &mut self,
        parent: ParentWindow,
        kind: WebViewKind,
        url: &str,
        bounds: Bounds,
    ) -> Result<SlotId> {
        let Some(free) = self.pool.find_free(&kind) else {
            let slot = self.factory.create(parent, kind, url, bounds)?;
            return Ok(self.pool.insert(slot));
        };
        if let Some(slot) = self.pool.get(free) {
            slot.handle.set_bounds(bounds)?;
            slot.handle.navigate(url)?;
        }
        Ok(free)
    }

    fn refresh_native_visibility(&self, pane: &PaneId) -> Result<()> {
        self.panes
            .get(pane)
            .map_or(Ok(()), |runtime| self.set_native_visibility(runtime))
    }

    fn set_native_visibility(&self, runtime: &PaneRuntime) -> Result<()> {
        let Some(slot) = runtime.slot.and_then(|id| self.pool.get(id)) else {
            return Ok(());
        };
        let failed = matches!(runtime.phase, PanePhase::Failed(_));
        if self.overlay == Overlay::Inactive && runtime.visible && !failed {
            Ok(slot.handle.show()?)
        } else {
            Ok(slot.handle.hide()?)
        }
    }
}

impl PaneHost {
    fn route_activation(&mut self) -> Result<()> {
        let mut changes = Vec::new();
        for (id, runtime) in &self.panes {
            let wanted =
                runtime.visible && runtime.is_mounted() && self.focused.as_ref() == Some(id);
            if wanted != runtime.activated {
                changes.push((id.clone(), wanted));
            }
        }
        for (pane, activate) in changes {
            self.set_activation(&pane, activate)?;
        }
        Ok(())
    }

    fn set_activation(&mut self, pane: &PaneId, activate: bool) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.activated = activate;
        let event = if activate {
            Lifecycle::Activate
        } else {
            Lifecycle::Deactivate
        };
        let frame = runtime.lifecycle_frame(event);
        self.send(pane, &frame)?;
        match (
            activate,
            self.panes.get(pane).and_then(|runtime| runtime.slot),
        ) {
            (true, Some(slot)) => self
                .pool
                .get(slot)
                .map_or(Ok(()), |slot| Ok(slot.handle.focus()?)),
            _ => Ok(()),
        }
    }

    fn on_frame(&mut self, pane: PaneId, frame: PaneFrame) -> Result<Vec<HostEvent>> {
        let event = match frame {
            PaneFrame::Register { type_id } => {
                return self.on_register(&pane, &type_id).map(|()| Vec::new());
            },
            PaneFrame::SetTitle { title } => HostEvent::TitleChanged { pane, title },
            PaneFrame::SetState { state } => {
                if let Some(runtime) = self.panes.get_mut(&pane) {
                    runtime.instance.state = state.clone();
                }
                HostEvent::StateChanged { pane, state }
            },
            PaneFrame::Open { type_id, params } => HostEvent::OpenRequested {
                from: pane,
                type_id,
                params,
            },
            PaneFrame::Request { id, method, params } => HostEvent::RequestMade {
                pane,
                id,
                method,
                params,
            },
            PaneFrame::Subscribe { id, event } => HostEvent::Subscribed { pane, id, event },
            PaneFrame::Unsubscribe { id } => HostEvent::Unsubscribed { pane, id },
            PaneFrame::RunCommand { command } => HostEvent::CommandRequested { pane, command },
        };
        Ok(vec![event])
    }

    fn on_register(&mut self, pane: &PaneId, type_id: &PaneTypeId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if &runtime.instance.type_id != type_id {
            let message = format!(
                "page registered as {type_id} but was opened as {}",
                runtime.instance.type_id
            );
            return self.fail(pane, message);
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(Lifecycle::Mount);
        runtime.discard_state = false;
        self.send(pane, &mount)?;
        if !self.keybindings.is_empty() {
            self.send(
                pane,
                &ShellFrame::Keybindings {
                    bindings: self.keybindings.clone(),
                },
            )?;
        }
        self.route_activation()
    }

    fn fail(&mut self, pane: &PaneId, message: String) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.phase = PanePhase::Failed(message);
        runtime.activated = false;
        self.refresh_native_visibility(pane)
    }

    fn apply_status(&mut self, pane: &PaneId, status: &ExtensionStatus) -> Result<()> {
        if self.panes.get(pane).is_some_and(PaneRuntime::is_mounted) {
            self.send(pane, &ShellFrame::ExtensionStatus(status.clone()))?;
        }
        match status.status {
            StatusKind::Unavailable | StatusKind::Disabled => {
                let reason = status
                    .reason
                    .clone()
                    .unwrap_or_else(|| format!("extension {:?}", status.status));
                self.fail(pane, reason)
            },
            StatusKind::Running | StatusKind::Back => self.recover(pane),
        }
    }

    fn recover(&mut self, pane: &PaneId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if !matches!(runtime.phase, PanePhase::Failed(_)) || runtime.slot.is_none() {
            return Ok(());
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(Lifecycle::Mount);
        self.send(pane, &mount)?;
        self.refresh_native_visibility(pane)
    }
}

#[cfg(test)]
#[path = "pane_host_tests.rs"]
mod tests;
