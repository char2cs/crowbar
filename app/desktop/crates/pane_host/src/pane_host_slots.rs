use layout::Layout;
use protocol::{PaneId, RelayScope};
use webview::{ParentWindow, WebViewKind};

use crate::browser_slot::{first_url, is_browser};
use crate::entry_url::{entry_url, tagged};
use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_kind::PaneKind;
use crate::pane_phase::PanePhase;
use crate::pane_type::PaneType;
use crate::slot_id::SlotId;
use crate::slot_request::{BLANK_URL, SlotRequest};
use crate::stage::Stage;
use crate::visible_pane::VisiblePane;

impl PaneHost {
    pub(crate) fn release_slot(
        &mut self,
        pane: &PaneId,
    ) -> Result<()> {
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
        let view = slot.handle.id();
        let driven = slot
            .handle
            .hide()
            .and_then(|()| slot.handle.navigate(BLANK_URL));
        self.revoke_relay(view);
        self.origins.remove(view);
        match driven {
            Ok(()) => self.pool.release(slot_id),
            Err(ref error) => {
                tracing::warn!(%error, "slot could not be parked, discarding it");
                self.pool.discard(slot_id);
            },
        }
        Ok(driven?)
    }

    fn revoke_relay(
        &self,
        view: webview::WebViewId,
    ) {
        let Some(assignment) = self.origins.get(view) else {
            return;
        };
        let scope = RelayScope::new(self.config.host.clone(), assignment.pane, assignment.origin);
        self.transport.revoke_pane(&scope);
    }

    pub(crate) fn show(
        &mut self,
        parent: ParentWindow,
        visible: &VisiblePane,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get(&visible.pane) else {
            return Ok(());
        };
        let Some(pane_type) = self.registry.get(&runtime.type_id).cloned() else {
            return self.release_slot(&visible.pane);
        };
        if pane_type.kind == PaneKind::Native {
            return self.release_slot(&visible.pane);
        }
        match runtime.slot {
            Some(slot) => self
                .pool
                .get(slot)
                .map_or(Ok(()), |slot| slot.handle.set_bounds(visible.bounds))?,
            None => self.assign_slot(parent, &pane_type, (visible, layout))?,
        }
        if let Some(runtime) = self.panes.get_mut(&visible.pane) {
            runtime.reveal();
        }
        self.refresh_native_visibility(&visible.pane)
    }

    fn page_url(
        &self,
        pane_type: &PaneType,
        (visible, layout): (&VisiblePane, &Layout),
    ) -> String {
        if is_browser(&pane_type.id) {
            return first_url(layout.pane(&visible.pane).map(|instance| &instance.state));
        }
        entry_url(&self.config.scheme, &self.config.host, pane_type)
    }

    pub(crate) fn assign_slot(
        &mut self,
        parent: ParentWindow,
        pane_type: &PaneType,
        (visible, layout): (&VisiblePane, &Layout),
    ) -> Result<()> {
        let browser = is_browser(&pane_type.id);
        let kind = match pane_type.id.extension() {
            _ if browser => WebViewKind::Browser,
            Some(extension) => WebViewKind::Extension(extension),
            None => WebViewKind::Core,
        };
        let url = self.page_url(pane_type, (visible, layout));
        let request = SlotRequest {
            parent,
            kind,
            bounds: visible.bounds,
        };
        let slot_id = self.take_slot(request)?;
        let Some(slot) = self.pool.get(slot_id) else {
            return Ok(());
        };
        let generation = slot.owner.assign(visible.pane.clone());
        if let Some(runtime) = self.panes.get_mut(&visible.pane) {
            runtime.slot = Some(slot_id);
            runtime.phase = PanePhase::Loading;
            runtime.stage = Stage::Hidden;
        }
        let Some(slot) = self.pool.get(slot_id) else {
            return Ok(());
        };
        if browser {
            return Ok(slot.handle.navigate(&url)?);
        }
        let origin = pane_type.id.origin().clone();
        self.origins
            .set(slot.handle.id(), visible.pane.clone(), origin);
        Ok(slot.handle.navigate(&tagged(&url, generation))?)
    }

    pub(crate) fn take_slot(
        &mut self,
        request: SlotRequest,
    ) -> Result<SlotId> {
        let Some(free) = self.pool.find_free(&request.kind) else {
            let slot = self.factory.create(request)?;
            return Ok(self.pool.insert(slot));
        };
        if let Some(slot) = self.pool.get(free) {
            slot.handle.set_bounds(request.bounds)?;
        }
        Ok(free)
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use crate::test_support::{fixture, instance};

    #[test]
    fn releasing_a_shown_pane_keeps_an_idle_core_view_and_revokes_its_relay() {
        let mut f = fixture();
        let pane = PaneId::new("p1").unwrap();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.release_slot(&pane).unwrap();
        assert_eq!(f.host.web_view_count(), 1, "idle view kept for reuse");
        assert_eq!(f.revoked.panes(), [pane], "relay revoked");
    }

    #[test]
    fn releasing_a_pane_without_a_view_does_nothing() {
        let mut f = fixture();
        f.host
            .release_slot(&PaneId::new("missing").unwrap())
            .unwrap();
        assert_eq!(f.host.web_view_count(), 0, "no view");
        assert!(f.revoked.panes().is_empty(), "nothing revoked");
    }
}
