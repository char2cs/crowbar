mod browser;
mod context;
mod desktop;
mod lifecycle;
mod reconcile;
mod refusals;

pub use browser::BrowseAction;
pub use reconcile::{
    Covered, Keyboard, LayoutSnapshot, Overlays, PaneDisplay, PaneKinds, SyncReport, VisiblePane,
};

use std::collections::BTreeMap;
use std::sync::Arc;

use layout::Layout;
use protocol::{
    DaemonTransport, KeyBinding, PaneId, SettingValues, ShellFrame, ThemeTokens, decode_pane_frame,
    encode,
};

use crate::catalog::{PaneKind, PaneRegistry};
use crate::error::{Error, Result};
use crate::pane_host::lifecycle::pane_phase::PanePhase;
use crate::pane_host::lifecycle::pane_runtime::PaneRuntime;
use crate::pane_host::reconcile::failures::Failures;
use crate::scheme::OriginTable;
use crate::scheme::PaneMenus;
use crate::scheme::PaneTables;
use crate::slot::Config;
use crate::slot::Factory;
use crate::slot::Gate;
use crate::slot::HostEvent;
use crate::slot::IpcMessage;
use crate::slot::IpcNotice;
use crate::slot::Pool;
use crate::slot::browser_event;
use crate::slot::drop_frame;
use crate::slot::focus_report;
use crate::slot::parse_hover;
use crate::slot::pointer_report;

/// Owns every pane web view: places them from the layout, pools and parks them, delivers
/// lifecycle frames, and turns what pages post into [`HostEvent`]s.
///
/// The host keeps no copy of pane state. The model owns it; the host reads it from each
/// [`LayoutSnapshot`] and reports changes as events.
pub struct PaneHost {
    pub(crate) registry: PaneRegistry,
    pub(crate) factory: Factory,
    pub(crate) pool: Pool,
    pub(crate) origins: OriginTable,
    pub(crate) menus: PaneMenus,
    pub(crate) transport: Arc<dyn DaemonTransport>,
    pub(crate) panes: BTreeMap<PaneId, PaneRuntime>,
    pub(crate) keybindings: Vec<KeyBinding>,
    pub(crate) theme: Option<ThemeTokens>,
    pub(crate) breakpoints: Option<Vec<protocol::Breakpoint>>,
    pub(crate) chat_zoom: Option<u16>,
    pub(crate) settings: Option<SettingValues>,
    pub(crate) paired_hosts: Option<Vec<protocol::PairedHost>>,
    pub(crate) focused: Option<PaneId>,
    pub(crate) keyboard: Keyboard,
    pub(crate) inbox: async_channel::Receiver<IpcMessage>,
    pub(crate) gate: Arc<Gate>,
    pub(crate) config: Config,
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
    /// Builds a host. `tables` must be the ones the [`PaneProtocol`](crate::PaneProtocol) of
    /// `webviews` shares.
    #[must_use]
    pub fn new(
        config: Config,
        webviews: webview::Host,
        tables: PaneTables,
        transport: Arc<dyn DaemonTransport>,
    ) -> Self {
        let PaneTables { origins, menus } = tables;
        let (sink, inbox) = async_channel::unbounded();
        let gate = Arc::new(Gate::new(sink));
        Self {
            registry: PaneRegistry::default(),
            pool: Pool::new(config.max_idle_slots),
            factory: Factory::new(webviews, config.clone(), Arc::clone(&gate)),
            origins,
            menus,
            transport,
            panes: BTreeMap::new(),
            keybindings: Vec::new(),
            theme: None,
            breakpoints: None,
            chat_zoom: None,
            settings: None,
            paired_hosts: None,
            focused: None,
            keyboard: Keyboard::default(),
            inbox,
            gate,
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

    /// Deletes cookies, cache and site storage of every browser pane.
    ///
    /// # Errors
    /// [`Error::WebView`] when the engine refuses.
    pub fn clear_browsing_data(&self) -> Result<()> {
        Ok(self.factory.clear_browsing_data()?)
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
            _ if runtime.kind == PaneKind::Native => PaneDisplay::Native,
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
            self.track(snapshot, &id);
        }
        self.hide_unlisted(snapshot, &mut events, &mut failures);
        for visible in &snapshot.visible {
            let seen = (snapshot.layout, &snapshot.overlays);
            failures.note(self.show(snapshot.parent, visible, seen));
        }
        self.focused.clone_from(&snapshot.focused);
        let keyboard = match snapshot.overlays.covered {
            Covered::Nothing => snapshot.keyboard,
            Covered::Everything | Covered::Areas(_) => Keyboard::Shell,
        };
        let held_by = std::mem::replace(&mut self.keyboard, keyboard);
        failures.note(self.route_activation(snapshot.layout));
        failures.note(self.send_groups(snapshot.layout));
        failures.note(self.settle_keyboard(held_by));
        SyncReport {
            events,
            failure: failures.into_first(),
        }
    }

    /// Handles one message a page posted. `layout` is the model the pane state lives in.
    ///
    /// A message from a pane that no longer has a web view is stale and ignored. A malformed
    /// or oversized frame is answered with a `protocol.error` frame and the pane stays; only a
    /// run of them fails the pane, inside its own frame.
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
        if let Some(event) = focus_report(&message) {
            return Ok(vec![event]);
        }
        if let Some(events) = type_id.and_then(|type_id| browser_event(&message, type_id)) {
            return Ok(events);
        }
        if let Some(frame) = drop_frame(&message) {
            self.send(&message.pane, &frame)?;
            return Ok(Vec::new());
        }
        let report = hover_report(&message).or_else(|| pointer_report(&message));
        if let Some(event) = report {
            return Ok(vec![event]);
        }
        if let Some(notice) = IpcNotice::parse(&message.text) {
            return self.report_notice(&message.pane, notice);
        }
        let frame = match decode_pane_frame(&message.text) {
            Ok(frame) => frame,
            Err(error) => return self.refuse(&message.pane, format!("invalid message: {error}")),
        };
        if let Some(runtime) = self.panes.get_mut(&message.pane) {
            runtime.violations = 0;
        }
        self.on_frame(message.pane, frame, layout)
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
}

#[cfg(test)]
mod tests {
    use protocol::{Lifecycle, PaneState};
    use serde_json::json;
    use webview::{Call, Visibility};

    use super::*;
    use crate::catalog::PaneLifecycle;
    use crate::error::Error;
    use crate::pane_host::browser::BrowseAction;
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

    fn a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url() {
        let mut f = fixture();
        let chat = instance("p1", "core.chat");
        f.sync(&[chat], &["p1"], Some("p1"));
        let calls = f.mock.calls();
        assert!(
            matches!(&calls[0], Call::Create { url, kind: webview::Kind::Core, visibility: Visibility::Hidden, .. } if url == "about:blank"),
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

    fn clearing_browsing_data_is_forwarded_to_the_web_view_engine() {
        let f = fixture();
        assert!(f.host.clear_browsing_data().is_ok(), "accepted");
        assert_eq!(f.mock.calls(), [Call::ClearBrowsingData]);
    }

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

    fn a_browser_pane_gets_no_origin_so_the_relay_never_authorises_it() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        f.sync(
            &[browser_at("b1", "https://example.com/")],
            &["b1"],
            Some("b1"),
        );
        let calls = f.mock.calls();
        let Some(Call::Create { id, kind, .. }) = calls.first() else {
            panic!("no web view was created: {calls:?}");
        };
        assert_eq!(*kind, webview::Kind::Browser);
        assert!(f.host.origins.get(*id).is_none(), "{calls:?}");
    }

    fn a_press_on_a_browser_page_focuses_its_pane() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        f.sync(&[browser_at("b1", "https://example.com/")], &["b1"], None);
        let pane = PaneId::new("b1").unwrap();
        let press = IpcMessage {
            pane: pane.clone(),
            text: r#"{"sdk":1,"kind":"shell.focus"}"#.into(),
        };
        assert_eq!(f.ipc(press), [HostEvent::PageFocused { pane }]);
    }

    fn only_a_browser_pane_with_a_web_view_can_be_browsed() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        f.sync(
            &[
                browser_at("b1", "https://example.com/"),
                instance("c1", "core.chat"),
            ],
            &["b1", "c1"],
            Some("b1"),
        );
        let go = BrowseAction::Go(::browser::PageUrl::parse("https://example.com/").unwrap());
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

    const CASES: &[fn()] = &[
        a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url,
        register_mounts_with_params_and_state_then_activates_the_focused_pane,
        focus_moves_activation_between_panes,
        clearing_browsing_data_is_forwarded_to_the_web_view_engine,
        a_parkable_pane_gives_its_web_view_to_the_next_pane,
        a_parked_pane_is_restored_from_its_state,
        releasing_a_pane_revokes_its_relay_tokens,
        destroy_on_hide_drops_state_and_remounts_empty,
        a_pane_of_an_unregistered_type_shows_a_placeholder_and_keeps_its_state,
        removing_a_pane_from_the_layout_destroys_it_and_frees_its_view,
        send_to_a_pane_without_a_web_view_is_a_typed_error,
        a_browser_pane_gets_no_origin_so_the_relay_never_authorises_it,
        a_press_on_a_browser_page_focuses_its_pane,
        only_a_browser_pane_with_a_web_view_can_be_browsed,
    ];

    #[test]
    fn the_host_drives_every_pane_through_its_lifecycle() {
        for case in CASES {
            case();
        }
    }
}
