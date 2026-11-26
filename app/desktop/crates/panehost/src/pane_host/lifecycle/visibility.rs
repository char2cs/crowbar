use layout::Layout;
use protocol::{Lifecycle, PaneId, PaneTypeId};
use webview::Visibility;

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
            Some(slot) => Ok(slot.handle.set_visibility(Visibility::Hidden)?),
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
        let failed = runtime.phase == PanePhase::Failed;
        let seen = runtime.is_visible() && !failed;
        let visibility = match (seen, runtime.covered) {
            (false, _) => Visibility::Hidden,
            (true, true) => Visibility::Covered,
            (true, false) => Visibility::Shown,
        };
        Ok(slot.handle.set_visibility(visibility)?)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pane_host::reconcile::{Covered, Overlays, PaneKinds};
    use crate::test_support::*;

    #[test]
    fn keep_alive_panes_are_hidden_not_parked() {
        let mut f = fixture();
        let panes = [instance("t", "core.term"), instance("c", "core.chat")];
        f.sync(&panes, &["t"], Some("t"));
        register(&mut f, "t", "core.term");
        f.sync(&panes, &["c"], Some("c"));
        let events = lifecycle_events(&f.mock);
        let deactivated = [Lifecycle::Mount, Lifecycle::Activate, Lifecycle::Deactivate];
        assert_eq!(events, deactivated, "deactivated, never parked");
        assert_eq!(
            f.host.web_view_count(),
            2,
            "its web view stays alive beside the chat's"
        );
    }

    #[test]
    fn a_covered_pane_s_web_view_is_covered_in_place_and_the_shell_keeps_the_keys_until_it_is_shown()
     {
        let mut f = fixture();
        let panes = [instance("c", "core.chat")];
        f.mount(&panes, Some("c"));
        let shown = f.calls();
        let everything = Overlays {
            covered: Covered::Everything,
            cutout: None,
        };
        f.sync_under(&panes, &["c"], everything);
        assert_eq!(f.view_calls(shown), ["cover", "focus_parent"], "covered");
        let covered = f.calls();
        f.sync(&panes, &["c"], Some("c"));
        assert_eq!(f.view_calls(covered), ["show", "focus"], "uncovered");
        let events = lifecycle_events(&f.mock);
        assert_eq!(
            events,
            [Lifecycle::Mount, Lifecycle::Activate],
            "never parked nor deactivated"
        );
    }

    fn with_native(f: &mut Fixture) {
        let mut native = pane_type("core.native", PaneLifecycle::KeepAlive);
        native.kind = PaneKind::Native;
        f.host.registry_mut().register(native).unwrap();
    }

    #[test]
    fn a_native_pane_gets_no_web_view_and_no_lifecycle() {
        let mut f = fixture();
        with_native(&mut f);
        let pane = instance("n1", "core.native");
        f.sync(std::slice::from_ref(&pane), &["n1"], Some("n1"));
        assert!(
            f.mock.calls().is_empty(),
            "no web view: {:?}",
            f.mock.calls()
        );
        f.sync(&[pane], &[], None);
        assert!(f.mock.calls().is_empty(), "still none after hiding");
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
            ("n1", "core.native", PaneKind::Web, 1),
            ("c1", "core.chat", PaneKind::Native, 0),
        ];
        for (id, type_id, kind, views) in cases {
            let mut f = fixture();
            with_native(&mut f);
            let pane = instance(id, type_id);
            f.sync_kinds(&[pane], &[id], &Says(id, kind));
            assert_eq!(
                f.host.web_view_count(),
                views,
                "{type_id} drawn as {kind:?}"
            );
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
            f.mount(&panes, Some("p"));
            f.sync(&panes, &[], None);
            assert_eq!(lifecycle_events(&f.mock).last(), Some(expected), "{name}");
        }
    }
}
