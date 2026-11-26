mod browser;
mod context;
mod lifecycle;
mod reconcile;
mod refusals;

pub use browser::BrowseAction;
pub use reconcile::{
    Covered, Keyboard, LayoutSnapshot, Overlays, PaneKinds, SyncReport, VisiblePane,
};

use std::collections::BTreeMap;
use std::sync::Arc;

use layout::Layout;
use protocol::{DaemonTransport, PaneId, ShellFrame, decode_pane_frame, encode};

use crate::catalog::PaneRegistry;
use crate::error::{Error, Result};
use crate::pane_host::lifecycle::pane_runtime::PaneRuntime;
use crate::pane_host::reconcile::failures::Failures;
use crate::scheme::MenuAsk;
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
use crate::slot::hover_report;
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
    pub(crate) context: Vec<ShellFrame>,
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
        let bindings = Vec::new();
        let context = vec![ShellFrame::Keybindings { bindings }];
        Self {
            registry: PaneRegistry::default(),
            pool: Pool::new(config.max_idle_slots),
            factory: Factory::new(webviews, config.clone(), Arc::clone(&gate)),
            origins,
            menus,
            transport,
            panes: BTreeMap::new(),
            context,
            focused: None,
            keyboard: Keyboard::default(),
            inbox,
            gate,
            config,
        }
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

    /// Whether `pane`'s page registered and was sent its `mount`, so a frame sent now is heard.
    #[must_use]
    pub fn is_mounted(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.get(pane).is_some_and(PaneRuntime::is_mounted)
    }

    /// The desktop's own chooser, menu and dialog, shown on the web views' platform thread.
    #[must_use]
    pub fn desktop(&self) -> webview::Desktop {
        self.factory.desktop()
    }

    /// The native menus pages ask for. Await it on the main thread, pop each up over its pane
    /// and answer it.
    #[must_use]
    pub fn menu_asks(&self) -> async_channel::Receiver<MenuAsk> {
        self.menus.asks()
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
            Err(error) => return self.refuse(&message.pane, &format!("invalid message: {error}")),
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
    use webview::{Call, ChoiceKind, FileChoice, ParentWindow, Visibility};

    use super::*;
    use crate::test_support::*;

    fn mounts(f: &Fixture) -> Vec<(String, PaneState)> {
        let frames = frames_sent(&f.mock).into_iter();
        frames
            .filter_map(|frame| match frame {
                ShellFrame::Lifecycle {
                    event: Lifecycle::Mount,
                    pane_id,
                    state,
                    ..
                } => Some((pane_id.to_string(), state)),
                _ => None,
            })
            .collect()
    }

    fn state_of(pane: &str) -> PaneState {
        PaneState::new(json!({ "from": pane })).unwrap()
    }

    fn a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        let calls = f.mock.calls();
        assert!(
            matches!(&calls[0], Call::Create { url, kind: webview::Kind::Core, visibility: Visibility::Hidden, .. } if url == "about:blank"),
            "created blank and hidden: {calls:?}"
        );
        assert!(
            matches!(&calls[1], Call::Navigate { url, .. } if url == "crowbar://local/panes/core/x/index.html?g=1"),
            "then sent to its entry page: {calls:?}"
        );
        assert_eq!(f.view_calls(0), ["show"], "shown once placed");
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
            if params.get("threadId") == Some("p1") && *state == state_of("p1")),
            "{frames:?}"
        );
        let events = lifecycle_events(&f.mock);
        assert_eq!(
            events,
            [Lifecycle::Mount, Lifecycle::Activate],
            "mount then activate"
        );
    }

    fn focus_moves_activation_between_panes() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.mount(&panes, Some("p1"));
        f.sync(&panes, &["p1", "p2"], Some("p2"));
        let (mount, activate, deactivate) =
            (Lifecycle::Mount, Lifecycle::Activate, Lifecycle::Deactivate);
        let moved = [mount, activate, mount, deactivate, activate];
        assert_eq!(
            lifecycle_events(&f.mock),
            moved,
            "p1 mounted+active, p2 mounted, focus moved"
        );
    }

    fn a_parkable_pane_gives_its_web_view_to_the_next_pane_and_is_restored_from_its_state() {
        let mut f = fixture();
        let panes = [instance("a", "core.chat"), instance("b", "core.chat")];
        f.sync(&panes, &["a"], Some("a"));
        register(&mut f, "a", "core.chat");
        f.sync(&panes, &["b"], Some("b"));
        let creates = f
            .mock
            .calls()
            .into_iter()
            .filter(|c| matches!(c, Call::Create { .. }));
        assert_eq!(
            (creates.count(), f.host.web_view_count()),
            (1, 1),
            "one web view for two"
        );
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Park),
            "a was parked"
        );
        register(&mut f, "b", "core.chat");
        f.sync(&panes, &["a"], Some("a"));
        register(&mut f, "a", "core.chat");
        let restored = [
            ("a", state_of("a")),
            ("b", state_of("b")),
            ("a", state_of("a")),
        ];
        assert_eq!(
            mounts(&f),
            restored.map(|(pane, state)| (pane.to_owned(), state))
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
        f.mount(&panes, Some("s"));
        let pane = PaneId::new("s").unwrap();
        assert_eq!(
            f.sync(&panes, &[], None),
            [HostEvent::StateDiscarded { pane }],
            "told"
        );
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
            "destroy sent"
        );
        f.mount(&panes, Some("s"));
        let remounted = mounts(&f).pop().map(|(_, state)| state);
        assert_eq!(remounted, Some(PaneState::default()), "empty state");
    }

    fn a_pane_of_an_unregistered_type_gets_no_web_view() {
        let mut f = fixture();
        let orphan = instance("o", "ext.gone.thing");
        f.sync(std::slice::from_ref(&orphan), &["o"], Some("o"));
        f.sync(&[orphan], &[], None);
        assert!(f.mock.calls().is_empty(), "no web view for a missing type");
    }

    fn removing_a_pane_from_the_layout_destroys_it_and_frees_its_view() {
        let mut f = fixture();
        f.mount(&[instance("p1", "core.chat")], Some("p1"));
        f.sync(&[], &[], None);
        assert!(
            lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
            "destroy sent"
        );
        let pane = PaneId::new("p1").unwrap();
        assert!(!f.host.is_mounted(&pane), "forgotten");
        let frame = ShellFrame::Keybindings {
            bindings: Vec::new(),
        };
        let sent = f.host.send(&pane, &frame);
        assert!(matches!(sent, Err(Error::NotShown(_))), "no view: {sent:?}");
    }

    fn the_engine_s_own_services_run_on_the_web_views_platform() {
        let f = fixture();
        let choice = FileChoice {
            title: "Select Files",
            kind: ChoiceKind::Files,
            filter: None,
        };
        let parent = ParentWindow::X11(7);
        f.host
            .desktop()
            .choose(parent, choice, Box::new(|_| ()))
            .unwrap();
        f.host.clear_browsing_data().unwrap();
        let services = [
            Call::ChooseFiles { parent, choice },
            Call::ClearBrowsingData,
        ];
        assert_eq!(f.mock.calls(), services);
    }

    const CASES: &[fn()] = &[
        a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url,
        register_mounts_with_params_and_state_then_activates_the_focused_pane,
        focus_moves_activation_between_panes,
        a_parkable_pane_gives_its_web_view_to_the_next_pane_and_is_restored_from_its_state,
        releasing_a_pane_revokes_its_relay_tokens,
        destroy_on_hide_drops_state_and_remounts_empty,
        a_pane_of_an_unregistered_type_gets_no_web_view,
        removing_a_pane_from_the_layout_destroys_it_and_frees_its_view,
        the_engine_s_own_services_run_on_the_web_views_platform,
    ];

    #[test]
    fn the_host_drives_every_pane_through_its_lifecycle() {
        for case in CASES {
            case();
        }
    }
}
