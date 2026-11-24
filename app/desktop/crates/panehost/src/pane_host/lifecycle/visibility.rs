use layout::Layout;
use protocol::{Lifecycle, PaneId, PaneTypeId};

use crate::catalog::PaneKind;
use crate::catalog::PaneLifecycle;
use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::lifecycle::pane_phase::PanePhase;
use crate::pane_host::lifecycle::pane_runtime::PaneRuntime;
use crate::pane_host::lifecycle::stage::Stage;
use crate::pane_host::reconcile::Failures;
use crate::pane_host::reconcile::LayoutSnapshot;
use crate::slot::HostEvent;

impl PaneHost {
    pub(crate) fn retire_removed(
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

    pub(crate) fn track(
        &mut self,
        snapshot: &LayoutSnapshot<'_>,
        id: &PaneId,
    ) {
        let Some(instance) = snapshot.layout.pane(id) else {
            return;
        };
        let kind = snapshot
            .kinds
            .kind_of(id)
            .unwrap_or_else(|| self.type_kind(&instance.type_id));
        let runtime = self
            .panes
            .entry(id.clone())
            .or_insert_with(|| PaneRuntime::new(instance.type_id.clone()));
        runtime.kind = kind;
    }

    pub(crate) fn hide_unlisted(
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

    fn type_kind(
        &self,
        type_id: &PaneTypeId,
    ) -> PaneKind {
        self.registry
            .get(type_id)
            .map_or(PaneKind::Web, |pane_type| pane_type.kind)
    }

    pub(crate) fn lifecycle_of(
        &self,
        type_id: &PaneTypeId,
    ) -> PaneLifecycle {
        self.registry
            .get(type_id)
            .map_or(PaneLifecycle::Parkable, |pane_type| pane_type.lifecycle)
    }

    pub(crate) fn leave(
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

    pub(crate) fn hide_in_place(
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

    pub(crate) fn discard_state(
        &mut self,
        pane: &PaneId,
        events: &mut Vec<HostEvent>,
    ) {
        if let Some(runtime) = self.panes.get_mut(pane) {
            runtime.discard_state = true;
            events.push(HostEvent::StateDiscarded { pane: pane.clone() });
        }
    }

    pub(crate) fn send_lifecycle(
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

    pub(crate) fn set_native_visibility(
        &self,
        runtime: &PaneRuntime,
    ) -> Result<()> {
        let Some(slot) = runtime.slot.and_then(|id| self.pool.get(id)) else {
            return Ok(());
        };
        let failed = matches!(runtime.phase, PanePhase::Failed(_));
        let shown = runtime.is_visible() && !failed;
        if shown {
            return Ok(slot.handle.show()?);
        }
        Ok(slot.handle.hide()?)
    }
}

#[cfg(test)]
mod tests {
    use webview::Call;

    use super::*;
    use crate::pane_host::reconcile::{PaneDisplay, PaneKinds};
    use crate::test_support::*;

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

    #[derive(Debug)]
    struct Says(&'static str, PaneKind);

    impl PaneKinds for Says {
        fn kind_of(
            &self,
            pane: &PaneId,
        ) -> Option<PaneKind> {
            (pane.as_str() == self.0).then_some(self.1)
        }
    }

    #[test]
    fn the_shell_decides_per_pane_whether_a_page_or_a_native_view_draws_it() {
        let cases = [
            ("n1", "core.native", PaneKind::Web, 1, PaneDisplay::Web),
            ("c1", "core.chat", PaneKind::Native, 0, PaneDisplay::Native),
        ];
        for (id, type_id, kind, views, display) in cases {
            let mut f = fixture();
            let mut native = pane_type("core.native", PaneLifecycle::KeepAlive);
            native.kind = PaneKind::Native;
            f.host.registry_mut().register(native).unwrap();
            let pane = instance(id, type_id);
            f.sync_kinds(std::slice::from_ref(&pane), &[id], &Says(id, kind));
            assert_eq!(
                f.host.web_view_count(),
                views,
                "{type_id} drawn as {kind:?}"
            );
            assert_eq!(f.host.display(&pane.pane_id), display, "{type_id}");
        }
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
        for (name, type_id, expected) in CASES {
            let mut f = fixture();
            let panes = [instance("p", type_id)];
            f.sync(&panes, &["p"], Some("p"));
            register(&mut f, "p", type_id);
            f.sync(&panes, &[], None);
            assert_eq!(lifecycle_events(&f.mock).last(), Some(expected), "{name}");
        }
    }
}
