use layout::Layout;
use protocol::{PaneId, RelayScope};
use webview::{ParentWindow, Visibility};

use crate::catalog::PaneKind;
use crate::catalog::PaneType;
use crate::catalog::{entry_url, tagged};
use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::lifecycle::pane_phase::PanePhase;
use crate::pane_host::lifecycle::stage::Stage;
use crate::pane_host::reconcile::{Overlays, VisiblePane};
use crate::slot::Id;
use crate::slot::{BLANK_URL, Request};
use crate::slot::{first_url, is_browser};

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
            .set_visibility(Visibility::Hidden)
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
        view: webview::Id,
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
        (layout, overlays): (&Layout, &Overlays),
    ) -> Result<()> {
        let Some(runtime) = self.panes.get(&visible.pane) else {
            return Ok(());
        };
        let Some(pane_type) = self.registry.get(&runtime.type_id).cloned() else {
            return self.release_slot(&visible.pane);
        };
        if runtime.kind == PaneKind::Native {
            return self.release_slot(&visible.pane);
        }
        if runtime.slot.is_none() && self.page_url(&pane_type, (visible, layout)) == BLANK_URL {
            return Ok(());
        }
        match runtime.slot {
            Some(slot) => self
                .pool
                .get(slot)
                .map_or(Ok(()), |slot| slot.handle.set_bounds(visible.bounds))?,
            None => self.assign_slot(parent, &pane_type, (visible, layout))?,
        }
        let Some(runtime) = self.panes.get_mut(&visible.pane) else {
            return Ok(());
        };
        runtime.reveal();
        runtime.covered = overlays.covered.hides(&visible.bounds);
        if let Some(slot) = runtime.slot.and_then(|slot| self.pool.get(slot)) {
            slot.handle.set_cutout(overlays.cut_for(&visible.bounds))?;
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
            _ if browser => webview::Kind::Browser,
            Some(extension) => webview::Kind::Extension(extension),
            None => webview::Kind::Core,
        };
        let url = self.page_url(pane_type, (visible, layout));
        let request = Request {
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
        request: Request,
    ) -> Result<Id> {
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
    use webview::{Bounds, Call};

    use crate::pane_host::reconcile::Overlays;
    use crate::test_support::{fixture, instance};

    #[test]
    fn the_handle_s_hole_is_cut_from_the_view_under_it_and_filled_when_it_goes() {
        let mut f = fixture();
        let panes = [instance("c", "core.chat")];
        let rect = |x, y, height| Bounds {
            x,
            y,
            width: 69.0,
            height,
        };
        let (handle, hole) = (rect(125.5, 10.0, 21.0), rect(115.5, 0.0, 11.0));
        let cut = |cutout| Overlays {
            cutout,
            ..Overlays::default()
        };
        for cutout in [Some(handle), Some(handle), None] {
            f.sync_under(&panes, &["c"], cut(cutout));
        }
        let cuts: Vec<Option<Bounds>> = f
            .mock
            .calls()
            .iter()
            .filter_map(|call| match call {
                Call::SetCutout { hole, .. } => Some(*hole),
                _ => None,
            })
            .collect();
        assert_eq!(cuts, [Some(hole), Some(hole), None], "cut, then filled");
    }
}
