use std::cell::RefCell;
use std::collections::BTreeMap;

use browserbar::BrowserBar;
use gpui::{AnyView, App, AppContext as _, Entity};
use panehost::BrowseAction;
use protocol::PaneId;

use crate::panes::browser::browser_ids::BrowserIds;
use crate::panes::browser::picking_panes::PickingPanes;
use crate::shell::inbox::shell_request::ShellRequest;

const CHROME_HEIGHT: f32 = 40.0;

pub(crate) struct BrowserBars {
    ids: BrowserIds,
    bars: RefCell<BTreeMap<PaneId, Entity<BrowserBar>>>,
    picking: PickingPanes,
    requests: async_channel::Sender<ShellRequest>,
}

impl BrowserBars {
    pub(crate) fn new(
        ids: BrowserIds,
        requests: async_channel::Sender<ShellRequest>,
    ) -> Self {
        Self {
            ids,
            bars: RefCell::default(),
            picking: PickingPanes::default(),
            requests,
        }
    }

    pub(crate) fn height(
        &self,
        pane: &PaneId,
    ) -> f32 {
        if self.ids.contains(pane) {
            return CHROME_HEIGHT;
        }
        0.0
    }

    pub(crate) fn ground(
        &self,
        pane: &PaneId,
        palette: &ui::Palette,
    ) -> Option<gpui::Hsla> {
        self.ids.contains(pane).then(|| palette.background())
    }

    pub(crate) fn view(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> Option<AnyView> {
        if !self.ids.contains(pane) {
            return None;
        }
        let existing = self.bars.borrow().get(pane).cloned();
        let bar = existing.unwrap_or_else(|| self.create(pane, cx));
        Some(bar.into())
    }

    pub(crate) fn show_address(
        &self,
        pane: &PaneId,
        address: &str,
        cx: &mut App,
    ) {
        let bar = self.bars.borrow().get(pane).cloned();
        if let Some(bar) = bar {
            bar.update(cx, |bar, cx| bar.set_address(address, cx));
        }
    }

    pub(crate) fn toggle_pick(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> BrowseAction {
        let action = self.picking.toggle(pane);
        self.show_picking(pane, cx);
        action
    }

    pub(crate) fn finish_pick(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> bool {
        let pending = self.picking.finish(pane);
        self.show_picking(pane, cx);
        pending
    }

    fn show_picking(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) {
        let picking = self.picking.is_picking(pane);
        let bar = self.bars.borrow().get(pane).cloned();
        let Some(bar) = bar else {
            return;
        };
        if picking {
            bar.update(cx, BrowserBar::begin_pick);
            return;
        }
        bar.update(cx, BrowserBar::end_pick);
    }

    fn create(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> Entity<BrowserBar> {
        let bar = cx.new(BrowserBar::new);
        let (requests, owner) = (self.requests.clone(), pane.clone());
        cx.subscribe(&bar, move |_, event: &browserbar::Event, _| {
            forward(&requests, &owner, event);
        })
        .detach();
        self.bars.borrow_mut().insert(pane.clone(), bar.clone());
        bar
    }
}

fn forward(
    requests: &async_channel::Sender<ShellRequest>,
    pane: &PaneId,
    event: &browserbar::Event,
) {
    let request = ShellRequest::Browse {
        pane: pane.clone(),
        event: event.clone(),
    };
    if requests.try_send(request).is_err() {
        tracing::debug!("the shell is closing; an address bar request goes unseen");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{one_tab, pane};

    #[test]
    fn a_browser_pane_lays_the_pane_fill_under_its_page_and_other_panes_do_not() {
        let layout = one_tab([
            pane("b1", browser::BROWSER_PANE_TYPE),
            pane("c1", "core.chat"),
        ]);
        let ids = BrowserIds::default();
        ids.follow(&layout);
        let bars = BrowserBars::new(ids, async_channel::unbounded().0);
        let theme = ui::Theme::default();
        let palette = theme.palette(ui::Surface::Window);
        let ground = |pane: &str| bars.ground(&PaneId::new(pane).unwrap(), palette);
        assert_eq!(ground("b1"), Some(palette.background()));
        assert_eq!(ground("c1"), None);
        let chat = PaneId::new("c1").unwrap();
        assert!(bars.height(&chat).abs() < f32::EPSILON, "no chrome strip");
    }
}
