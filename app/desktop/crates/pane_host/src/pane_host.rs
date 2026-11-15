use std::collections::BTreeMap;
use std::sync::Arc;

use model::Layout;
use protocol::{
    DaemonTransport, ExtensionStatus, KeyBinding, Lifecycle, PaneId, PaneTypeId, ShellFrame,
    ThemeTokens, decode_pane_frame, encode,
};
use webview::WebViewHost;

use crate::browser_slot::browser_event;
use crate::error::{Error, Result};
use crate::failures::Failures;
use crate::host_event::HostEvent;
use crate::hover_message::parse_hover;
use crate::ipc_message::IpcMessage;
use crate::layout_snapshot::LayoutSnapshot;
use crate::origin_table::OriginTable;
use crate::overlay::Overlay;
use crate::pane_display::PaneDisplay;
use crate::pane_host_config::PaneHostConfig;
use crate::pane_kind::PaneKind;
use crate::pane_lifecycle::PaneLifecycle;
use crate::pane_phase::PanePhase;
use crate::pane_registry::PaneRegistry;
use crate::pane_runtime::PaneRuntime;
use crate::slot_factory::{INBOX_CAPACITY, SlotFactory};
use crate::slot_pool::SlotPool;
use crate::stage::Stage;
use crate::sync_report::SyncReport;

/// Owns every pane web view: places them from the layout, pools and parks them, delivers
/// lifecycle frames, and turns what pages post into [`HostEvent`]s.
///
/// The host keeps no copy of pane state. The model owns it; the host reads it from each
/// [`LayoutSnapshot`] and reports changes as events.
pub struct PaneHost {
    pub(crate) registry: PaneRegistry,
    pub(crate) factory: SlotFactory,
    pub(crate) pool: SlotPool,
    pub(crate) origins: OriginTable,
    pub(crate) transport: Arc<dyn DaemonTransport>,
    pub(crate) panes: BTreeMap<PaneId, PaneRuntime>,
    pub(crate) keybindings: Vec<KeyBinding>,
    pub(crate) theme: Option<ThemeTokens>,
    pub(crate) overlay: Overlay,
    pub(crate) focused: Option<PaneId>,
    pub(crate) inbox: async_channel::Receiver<IpcMessage>,
    pub(crate) config: PaneHostConfig,
}

impl std::fmt::Debug for PaneHost {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("PaneHost")
            .field("panes", &self.panes.len())
            .finish_non_exhaustive()
    }
}

fn hover_report(message: &IpcMessage) -> Option<HostEvent> {
    parse_hover(&message.text).map(|hovered| HostEvent::HoverChanged {
        pane: message.pane.clone(),
        hovered,
    })
}

impl PaneHost {
    /// Builds a host. `origins` must be the table the [`PaneProtocol`](crate::PaneProtocol)
    /// of `webviews` reads.
    #[must_use]
    pub fn new(
        config: PaneHostConfig,
        webviews: WebViewHost,
        origins: OriginTable,
        transport: Arc<dyn DaemonTransport>,
    ) -> Self {
        let (sink, inbox) = async_channel::bounded(INBOX_CAPACITY);
        Self {
            registry: PaneRegistry::default(),
            pool: SlotPool::new(config.max_idle_slots),
            factory: SlotFactory::new(webviews, config.clone(), sink),
            origins,
            transport,
            panes: BTreeMap::new(),
            keybindings: Vec::new(),
            theme: None,
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
    pub fn display(
        &self,
        pane: &PaneId,
    ) -> PaneDisplay {
        let Some(runtime) = self.panes.get(pane) else {
            return PaneDisplay::Empty;
        };
        let type_id = &runtime.type_id;
        match &runtime.phase {
            _ if self.registry.get(type_id).is_none() => PaneDisplay::Placeholder {
                type_id: type_id.clone(),
            },
            _ if self.is_native(type_id) => PaneDisplay::Native,
            PanePhase::Failed(message) => PaneDisplay::Error {
                message: message.clone(),
            },
            _ if runtime.slot.is_some() && runtime.is_visible() => PaneDisplay::Web,
            _ => PaneDisplay::Empty,
        }
    }

    /// Reconciles web views with the layout: creates, moves, shows, hides, parks and destroys.
    ///
    /// Every pane is reconciled even when another one fails; the first failure is returned in
    /// the report next to the events.
    pub fn sync(
        &mut self,
        snapshot: &LayoutSnapshot<'_>,
    ) -> SyncReport {
        let mut events = Vec::new();
        let mut failures = Failures::default();
        self.retire_removed(snapshot, &mut failures);
        for id in snapshot.pane_ids() {
            self.track(snapshot.layout, &id);
        }
        self.hide_unlisted(snapshot, &mut events, &mut failures);
        for visible in &snapshot.visible {
            failures.note(self.show(snapshot.parent, visible, snapshot.layout));
        }
        self.focused.clone_from(&snapshot.focused);
        failures.note(self.route_activation(snapshot.layout));
        failures.note(self.hand_focus_to_native());
        SyncReport {
            events,
            failure: failures.into_first(),
        }
    }

    /// Handles one message a page posted. `layout` is the model the pane state lives in.
    ///
    /// A message from a pane that no longer has a web view is stale and ignored. A malformed
    /// frame fails the pane: its frame shows the error and the window is unaffected.
    ///
    /// # Errors
    /// [`Error::WebView`] when answering the pane fails.
    pub fn handle_ipc(
        &mut self,
        message: IpcMessage,
        layout: &Layout,
    ) -> Result<Vec<HostEvent>> {
        let live = self
            .panes
            .get(&message.pane)
            .is_some_and(|runtime| runtime.slot.is_some());
        if !live {
            return Ok(Vec::new());
        }
        let type_id = self
            .panes
            .get(&message.pane)
            .map(|runtime| &runtime.type_id);
        if let Some(events) = type_id.and_then(|type_id| browser_event(&message, type_id)) {
            return Ok(events);
        }
        if let Some(event) = hover_report(&message) {
            return Ok(vec![event]);
        }
        let frame = match decode_pane_frame(&message.text) {
            Ok(frame) => frame,
            Err(error) => return self.reject(&message.pane, &error),
        };
        self.on_frame(message.pane, frame, layout)
    }

    fn reject(
        &mut self,
        pane: &PaneId,
        error: &protocol::Error,
    ) -> Result<Vec<HostEvent>> {
        self.fail(pane, format!("invalid message: {error}"))?;
        Ok(Vec::new())
    }

    /// Sends a frame to a pane's page.
    ///
    /// # Errors
    /// [`Error::NotShown`] when the pane has no web view, [`Error::Frame`] when the frame
    /// cannot be encoded and [`Error::WebView`] when the web view layer fails.
    pub fn send(
        &self,
        pane: &PaneId,
        frame: &ShellFrame,
    ) -> Result<()> {
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
    pub fn set_overlay(
        &mut self,
        overlay: Overlay,
    ) -> Result<()> {
        self.overlay = overlay;
        self.panes
            .values()
            .filter(|runtime| runtime.is_visible())
            .try_for_each(|runtime| self.set_native_visibility(runtime))
    }

    /// Replaces the active keybindings and pushes them to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`] when a pane cannot be reached.
    pub fn set_keybindings(
        &mut self,
        bindings: Vec<KeyBinding>,
    ) -> Result<()> {
        self.keybindings = bindings;
        let frame = ShellFrame::Keybindings {
            bindings: self.keybindings.clone(),
        };
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    /// Replaces the theme tokens and pushes them to mounted panes. Unchanged tokens send nothing.
    ///
    /// # Errors
    /// [`Error::WebView`] when a pane cannot be reached.
    pub fn set_theme(
        &mut self,
        tokens: ThemeTokens,
    ) -> Result<()> {
        if self.theme.as_ref() == Some(&tokens) {
            return Ok(());
        }
        let frame = ShellFrame::Theme {
            tokens: tokens.clone(),
        };
        self.theme = Some(tokens);
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    /// Applies an extension's new status to its panes: errors in their frames while it is
    /// unavailable or disabled, a fresh `mount` with the saved state when it is back.
    ///
    /// # Errors
    /// [`Error::WebView`] when a pane cannot be reached.
    pub fn extension_status(
        &mut self,
        status: &ExtensionStatus,
        layout: &Layout,
    ) -> Result<()> {
        let owned: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|(_, runtime)| {
                runtime.type_id.extension().as_ref() == Some(status.extension_id())
            })
            .map(|(id, _)| id.clone())
            .collect();
        let mut failures = Failures::default();
        for pane in owned {
            failures.note(self.apply_status(&pane, status, layout));
        }
        failures.into_first().map_or(Ok(()), Err)
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
    fn retire_removed(
        &mut self,
        snapshot: &LayoutSnapshot<'_>,
        failures: &mut Failures,
    ) {
        let gone: Vec<PaneId> = self
            .panes
            .keys()
            .filter(|id| snapshot.layout.pane(id).is_none())
            .cloned()
            .collect();
        for pane in gone {
            failures.note(self.send_lifecycle(&pane, Lifecycle::Destroy, snapshot.layout));
            failures.note(self.release_slot(&pane));
            self.panes.remove(&pane);
        }
    }

    fn track(
        &mut self,
        layout: &Layout,
        id: &PaneId,
    ) {
        let Some(instance) = layout.pane(id) else {
            return;
        };
        self.panes
            .entry(id.clone())
            .or_insert_with(|| PaneRuntime::new(instance.type_id.clone()));
    }

    fn hide_unlisted(
        &mut self,
        snapshot: &LayoutSnapshot<'_>,
        events: &mut Vec<HostEvent>,
        failures: &mut Failures,
    ) {
        let leaving: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|(id, runtime)| {
                runtime.is_visible() && !snapshot.visible.iter().any(|visible| &visible.pane == *id)
            })
            .map(|(id, _)| id.clone())
            .collect();
        for pane in leaving {
            failures.note(self.leave(&pane, events, snapshot.layout));
        }
    }

    fn is_native(
        &self,
        type_id: &PaneTypeId,
    ) -> bool {
        self.registry
            .get(type_id)
            .is_some_and(|pane_type| pane_type.kind == PaneKind::Native)
    }

    fn lifecycle_of(
        &self,
        type_id: &PaneTypeId,
    ) -> PaneLifecycle {
        self.registry
            .get(type_id)
            .map_or(PaneLifecycle::Parkable, |pane_type| pane_type.lifecycle)
    }

    fn leave(
        &mut self,
        pane: &PaneId,
        events: &mut Vec<HostEvent>,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get(pane) else {
            return Ok(());
        };
        match self.lifecycle_of(&runtime.type_id) {
            PaneLifecycle::KeepAlive => self.hide_in_place(pane, layout),
            PaneLifecycle::Parkable => {
                let sent = self.send_lifecycle(pane, Lifecycle::Park, layout);
                self.release_slot(pane).and(sent)
            },
            PaneLifecycle::DestroyOnHide => {
                let sent = self.send_lifecycle(pane, Lifecycle::Destroy, layout);
                let released = self.release_slot(pane);
                self.discard_state(pane, events);
                released.and(sent)
            },
        }
    }

    fn hide_in_place(
        &mut self,
        pane: &PaneId,
        layout: &Layout,
    ) -> Result<()> {
        if self
            .panes
            .get(pane)
            .is_some_and(|r| r.stage == Stage::Active)
        {
            self.set_activation(pane, Stage::Shown, layout)?;
        }
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.stage = Stage::Hidden;
        match runtime.slot.and_then(|id| self.pool.get(id)) {
            Some(slot) => Ok(slot.handle.hide()?),
            None => Ok(()),
        }
    }

    fn discard_state(
        &mut self,
        pane: &PaneId,
        events: &mut Vec<HostEvent>,
    ) {
        if let Some(runtime) = self.panes.get_mut(pane) {
            runtime.discard_state = true;
            events.push(HostEvent::StateDiscarded { pane: pane.clone() });
        }
    }

    fn send_lifecycle(
        &mut self,
        pane: &PaneId,
        event: Lifecycle,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get(pane).filter(|runtime| runtime.is_mounted()) else {
            return Ok(());
        };
        let frame = runtime.lifecycle_frame(pane, event, layout.pane(pane));
        self.send(pane, &frame)
    }

    pub(crate) fn refresh_native_visibility(
        &self,
        pane: &PaneId,
    ) -> Result<()> {
        self.panes
            .get(pane)
            .map_or(Ok(()), |runtime| self.set_native_visibility(runtime))
    }

    fn set_native_visibility(
        &self,
        runtime: &PaneRuntime,
    ) -> Result<()> {
        let Some(slot) = runtime.slot.and_then(|id| self.pool.get(id)) else {
            return Ok(());
        };
        let failed = matches!(runtime.phase, PanePhase::Failed(_));
        let shown = self.overlay == Overlay::Inactive && runtime.is_visible() && !failed;
        if shown {
            return Ok(slot.handle.show()?);
        }
        Ok(slot.handle.hide()?)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{CommandId, KeyChord, PaneState};
    use serde_json::json;
    use webview::{Call, Visibility, WebViewKind};

    use super::*;
    use crate::browse_action::BrowseAction;
    use crate::error::Error;
    use crate::test_support::*;

    fn is_mount(frame: &ShellFrame) -> bool {
        matches!(
            frame,
            ShellFrame::Lifecycle {
                event: Lifecycle::Mount,
                ..
            }
        )
    }

    fn last_visibility_is(
        f: &Fixture,
        expected: Visibility,
    ) -> bool {
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility { visibility, .. }) if *visibility == expected
        )
    }

    #[test]
    fn a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url() {
        let mut f = fixture();
        let chat = instance("p1", "core.chat");
        f.sync(&[chat], &["p1"], Some("p1"));
        let calls = f.mock.calls();
        assert!(
            matches!(&calls[0], Call::Create { url, kind: WebViewKind::Core, visibility: Visibility::Hidden, .. } if url == "about:blank"),
            "created blank and hidden: {calls:?}"
        );
        assert!(
            matches!(&calls[1], Call::Navigate { url, .. } if url == "crowbar://local/panes/core/x/index.html?g=1"),
            "then sent to its entry page: {calls:?}"
        );
        assert_eq!(shown(&f.mock), 1, "shown once placed");
        assert_eq!(
            f.host.display(&PaneId::new("p1").unwrap()),
            PaneDisplay::Web,
            "display"
        );
    }

    #[test]
    fn register_mounts_with_params_and_state_then_activates_the_focused_pane() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        assert!(
            lifecycle_events(&f.mock).is_empty(),
            "nothing is sent before the page registers"
        );
        register(&mut f, "p1", "core.chat");
        let frames = frames_sent(&f.mock);
        assert!(
            matches!(&frames[0], ShellFrame::Lifecycle { event: Lifecycle::Mount, params, state, .. }
            if params.get("threadId") == Some("p1") && state.as_value() == &json!({"from": "p1"})),
            "{frames:?}"
        );
        assert_eq!(
            lifecycle_events(&f.mock),
            [Lifecycle::Mount, Lifecycle::Activate],
            "mount then activate"
        );
    }

    #[test]
    fn focus_moves_activation_between_panes() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.sync(&panes, &["p1", "p2"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        register(&mut f, "p2", "core.chat");
        f.sync(&panes, &["p1", "p2"], Some("p2"));
        let events = lifecycle_events(&f.mock);
        assert_eq!(
            events,
            [
                Lifecycle::Mount,
                Lifecycle::Activate,
                Lifecycle::Mount,
                Lifecycle::Deactivate,
                Lifecycle::Activate
            ],
            "p1 mounted+active, p2 mounted, then focus moved"
        );
    }

    #[test]
    fn a_parkable_pane_gives_its_web_view_to_the_next_pane() {
        let mut f = fixture();
        let panes = [instance("a", "core.chat"), instance("b", "core.chat")];
        f.sync(&panes, &["a"], Some("a"));
        register(&mut f, "a", "core.chat");
        f.sync(&panes, &["b"], Some("b"));
        assert_eq!(
            f.host.web_view_count(),
            1,
            "the slot is reused, not duplicated"
        );
        let creates = f
            .mock
            .calls()
            .iter()
            .filter(|c| matches!(c, Call::Create { .. }))
            .count();
        assert_eq!(creates, 1, "one native web view for two tabs");
        assert!(
        f.mock.calls().iter().any(|c| matches!(c, Call::Navigate { url, .. } if url.starts_with("crowbar://local/panes/core/x/index.html?g="))),
        "navigated into the existing view"
    );
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Park),
            "a was parked"
        );
    }

    #[test]
    fn a_parked_pane_is_restored_from_its_state() {
        let mut f = fixture();
        let panes = [instance("a", "core.chat"), instance("b", "core.chat")];
        f.sync(&panes, &["a"], Some("a"));
        register(&mut f, "a", "core.chat");
        f.sync(&panes, &["b"], Some("b"));
        register(&mut f, "b", "core.chat");
        f.sync(&panes, &["a"], Some("a"));
        register(&mut f, "a", "core.chat");
        let mounts: Vec<ShellFrame> = frames_sent(&f.mock).into_iter().filter(is_mount).collect();
        assert!(
            matches!(&mounts[2], ShellFrame::Lifecycle { pane_id, state, .. } if pane_id.as_str() == "a" && state.as_value() == &json!({"from": "a"})),
            "a restored from its state"
        );
    }

    #[test]
    fn keep_alive_panes_are_hidden_not_parked() {
        let mut f = fixture();
        let panes = [instance("t", "core.term"), instance("c", "core.chat")];
        f.sync(&panes, &["t"], Some("t"));
        register(&mut f, "t", "core.term");
        f.sync(&panes, &["c"], Some("c"));
        let events = lifecycle_events(&f.mock);
        assert_eq!(
            events,
            [Lifecycle::Mount, Lifecycle::Activate, Lifecycle::Deactivate],
            "deactivated, never parked"
        );
        assert_eq!(
            f.host.web_view_count(),
            2,
            "its web view stays alive beside the chat's"
        );
        assert_eq!(
            f.host.display(&PaneId::new("t").unwrap()),
            PaneDisplay::Empty,
            "nothing drawn for it"
        );
    }

    #[test]
    fn releasing_a_pane_revokes_its_relay_tokens() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        assert!(
            f.revoked.panes().is_empty(),
            "shown panes keep their tokens"
        );
        f.sync(&panes, &[], None);
        assert_eq!(f.revoked.panes(), [PaneId::new("p1").unwrap()]);
    }

    #[test]
    fn destroy_on_hide_drops_state_and_remounts_empty() {
        let mut f = fixture();
        let panes = [instance("s", "core.scratch")];
        f.sync(&panes, &["s"], Some("s"));
        register(&mut f, "s", "core.scratch");
        let events = f.sync(&panes, &[], None);
        assert_eq!(
            events,
            [HostEvent::StateDiscarded {
                pane: PaneId::new("s").unwrap()
            }],
            "shell is told"
        );
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
            "destroy sent"
        );
        f.sync(&panes, &["s"], Some("s"));
        register(&mut f, "s", "core.scratch");
        let last_mount = frames_sent(&f.mock).into_iter().rev().find(is_mount);
        assert!(
            matches!(last_mount, Some(ShellFrame::Lifecycle { state, .. }) if state == PaneState::default()),
            "empty state"
        );
    }

    #[test]
    fn an_overlay_hides_web_views_without_telling_any_pane() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        let before = frames_sent(&f.mock).len();
        f.host.set_overlay(Overlay::Active).unwrap();
        assert!(last_visibility_is(&f, Visibility::Hidden), "hidden");
        f.sync(&panes, &["p1"], Some("p1"));
        assert!(
            last_visibility_is(&f, Visibility::Hidden),
            "stays hidden while the overlay lasts"
        );
        f.host.set_overlay(Overlay::Inactive).unwrap();
        assert!(last_visibility_is(&f, Visibility::Shown), "shown again");
        assert_eq!(frames_sent(&f.mock).len(), before, "no frame was sent");
    }

    #[test]
    fn a_pane_of_an_unregistered_type_shows_a_placeholder_and_keeps_its_state() {
        let mut f = fixture();
        let orphan = instance("o", "ext.gone.thing");
        f.sync(std::slice::from_ref(&orphan), &["o"], Some("o"));
        assert!(f.mock.calls().is_empty(), "no web view for a missing type");
        assert_eq!(
            f.host.display(&orphan.pane_id),
            PaneDisplay::Placeholder {
                type_id: orphan.type_id.clone()
            },
            "placeholder"
        );
        f.sync(&[orphan], &[], None);
    }

    #[test]
    fn removing_a_pane_from_the_layout_destroys_it_and_frees_its_view() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        f.sync(&[], &[], None);
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
            "destroy sent"
        );
        assert_eq!(
            f.host.display(&PaneId::new("p1").unwrap()),
            PaneDisplay::Empty,
            "forgotten"
        );
    }

    #[test]
    fn keybindings_reach_mounted_panes_now_and_later() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        let binding = KeyBinding::new(
            KeyChord::new("mod+k").unwrap(),
            CommandId::new("palette.open").unwrap(),
        );
        f.host.set_keybindings(vec![binding.clone()]).unwrap();
        assert!(
            frames_sent(&f.mock).is_empty(),
            "not mounted yet, nothing sent"
        );
        register(&mut f, "p1", "core.chat");
        assert!(
            frames_sent(&f.mock).contains(&ShellFrame::Keybindings {
                bindings: vec![binding]
            }),
            "sent on mount"
        );
    }

    #[test]
    fn send_to_a_pane_without_a_web_view_is_a_typed_error() {
        let f = fixture();
        let frame = ShellFrame::Keybindings {
            bindings: Vec::new(),
        };
        assert!(
            matches!(
                f.host.send(&PaneId::new("nope").unwrap(), &frame),
                Err(Error::NotShown(_))
            ),
            "no view"
        );
    }

    #[test]
    fn a_browser_pane_gets_no_origin_so_the_relay_never_authorises_it() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        f.sync(&[instance("b1", "core.browser")], &["b1"], Some("b1"));
        let calls = f.mock.calls();
        let Some(Call::Create { id, kind, .. }) = calls.first() else {
            panic!("no web view was created: {calls:?}");
        };
        assert_eq!(*kind, WebViewKind::Browser);
        assert!(f.host.origins.get(*id).is_none(), "{calls:?}");
    }

    #[test]
    fn a_native_pane_gets_no_web_view_and_no_lifecycle() {
        let mut f = fixture();
        let mut native = pane_type("core.native", PaneLifecycle::KeepAlive);
        native.kind = PaneKind::Native;
        f.host.registry_mut().register(native).unwrap();
        let pane = instance("n1", "core.native");
        f.sync(std::slice::from_ref(&pane), &["n1"], Some("n1"));
        assert!(
            f.mock.calls().is_empty(),
            "no web view: {:?}",
            f.mock.calls()
        );
        assert_eq!(
            f.host.display(&pane.pane_id),
            PaneDisplay::Native,
            "display"
        );
        f.sync(&[pane], &[], None);
        assert!(f.mock.calls().is_empty(), "still none after hiding");
    }

    #[test]
    fn focusing_a_native_pane_takes_keyboard_focus_from_the_web_view() {
        let mut f = fixture();
        let mut native = pane_type("core.native", PaneLifecycle::KeepAlive);
        native.kind = PaneKind::Native;
        f.host.registry_mut().register(native).unwrap();
        let panes = [instance("c1", "core.chat"), instance("n1", "core.native")];
        f.sync(&panes, &["c1", "n1"], Some("c1"));
        let before = f.mock.calls().len();
        f.sync(&panes, &["c1", "n1"], Some("n1"));
        let calls = f.mock.calls();
        let returned = calls
            .iter()
            .skip(before)
            .any(|call| matches!(call, Call::FocusParent { .. }));
        assert!(returned, "{calls:?}");
    }

    #[test]
    fn only_a_browser_pane_with_a_web_view_can_be_browsed() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        f.sync(
            &[instance("b1", "core.browser"), instance("c1", "core.chat")],
            &["b1", "c1"],
            Some("b1"),
        );
        let go = BrowseAction::Go(browser_nav::BrowserUrl::parse("https://example.com/").unwrap());
        assert!(f.host.browse(&PaneId::new("b1").unwrap(), &go).is_ok());
        assert!(
            f.host
                .browse(&PaneId::new("b1").unwrap(), &BrowseAction::Back)
                .is_ok()
        );
        assert!(matches!(
            f.host.browse(&PaneId::new("c1").unwrap(), &go),
            Err(Error::NotShown(_))
        ));
        let navigated = f.mock.calls().into_iter().any(
            |call| matches!(call, Call::Navigate { url, .. } if url == "https://example.com/"),
        );
        assert!(navigated);
    }

    const CASES: &[(&str, &str, Lifecycle)] = &[
        ("a parkable pane is parked", "core.chat", Lifecycle::Park),
        (
            "a keep-alive pane is only deactivated",
            "core.term",
            Lifecycle::Deactivate,
        ),
        (
            "a destroy-on-hide pane is destroyed",
            "core.scratch",
            Lifecycle::Destroy,
        ),
    ];

    #[test]
    fn hiding_a_pane_sends_the_event_its_lifecycle_calls_for() {
        let wrong: Vec<&str> = CASES
            .iter()
            .filter(|(_, type_id, expected)| {
                let mut f = fixture();
                let panes = [instance("p", type_id)];
                f.sync(&panes, &["p"], Some("p"));
                register(&mut f, "p", type_id);
                f.sync(&panes, &[], None);
                lifecycle_events(&f.mock).last() != Some(expected)
            })
            .map(|(name, ..)| *name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }
}
