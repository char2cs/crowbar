use std::collections::BTreeSet;
use std::sync::Arc;
use std::time::Duration;

use gpui::{AppContext as _, AsyncApp, Context, WeakEntity};
use settings::{Key, Read as _};
use sidebar::{HostKind, PaneFocus as _, SpaceList as _};

use super::ShellView;
use crate::core_panes::TERMINAL;
use crate::readout_dock::ReadoutDock;
use crate::readout_lines::lines_of;
use crate::readout_net::ReadoutNet;
use crate::readout_snapshot::ReadoutSnapshot;
use crate::readout_view::ReadoutView;
use crate::reconnect_count::recount;

pub(super) const BUILD_READOUT: &str = "buildReadout";
const SAMPLE_EVERY: Duration = Duration::from_secs(1);
const VERSION: &str = env!("CARGO_PKG_VERSION");

#[cfg(debug_assertions)]
const CHANNEL: &str = "dev ";
#[cfg(not(debug_assertions))]
const CHANNEL: &str = "";

fn build_text(channel: &str) -> String {
    format!("{channel}{VERSION}")
}

async fn keep_sampling(
    this: WeakEntity<ShellView>,
    mut cx: AsyncApp,
) {
    while sampled_after_a_beat(&this, &mut cx).await {}
}

async fn sampled_after_a_beat(
    this: &WeakEntity<ShellView>,
    cx: &mut AsyncApp,
) -> bool {
    cx.background_executor().timer(SAMPLE_EVERY).await;
    this.update(cx, ShellView::sample_readout).is_ok()
}

fn panes_of(tab: &layout::Tab) -> Vec<&layout::PaneInstance> {
    tab.pane_ids()
        .iter()
        .filter_map(|id| tab.pane(id))
        .collect()
}

impl ShellView {
    pub(super) fn apply_readout(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let shown = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(BUILD_READOUT)))
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        if !shown {
            self.readout = crate::readout_state::ReadoutState::new(Arc::clone(&self.ports.clock));
            cx.notify();
            return;
        }
        if self.readout.view.is_some() {
            return;
        }
        self.readout.view = Some(cx.new(|_| ReadoutView::default()));
        self.readout.tick = Some(cx.spawn(async move |this, cx| {
            keep_sampling(this, cx.clone()).await;
        }));
        self.sample_readout(cx);
    }

    fn sample_readout(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(view) = self.readout.view.clone() else {
            return;
        };
        let net = self.link_net();
        self.readout.reconnects = recount(self.readout.reconnects, (self.readout.net, net));
        self.readout.net = net;
        let snapshot = ReadoutSnapshot {
            build: build_text(CHANNEL),
            net,
            reconnects: self.readout.reconnects,
            frame: self.readout.clock.stats(self.ports.clock.instant()),
            counts: self.readout_counts(),
        };
        let dock = ReadoutDock::of(&self.sidebar.read(cx).preferences());
        view.update(cx, |view, cx| view.show((lines_of(&snapshot), dock), cx));
    }

    fn link_net(&self) -> ReadoutNet {
        self.data
            .view()
            .hosts()
            .iter()
            .find(|host| host.kind == HostKind::Local)
            .map_or(ReadoutNet::Idle, |host| ReadoutNet::of(host.state))
    }

    fn readout_counts(&self) -> (usize, usize, usize) {
        let layout = self.state.layout();
        let panes: Vec<&layout::PaneInstance> = layout.tabs().iter().flat_map(panes_of).collect();
        let terminals = panes
            .iter()
            .filter(|pane| pane.type_id.as_str() == TERMINAL)
            .count();
        let spaces: BTreeSet<String> = panes
            .iter()
            .filter_map(|pane| pane.params.get("threadId"))
            .filter_map(|chat| self.data.chat_project(chat))
            .collect();
        (
            terminals,
            self.data.view().running_agents().len(),
            spaces.len(),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_debug_build_is_marked_dev_and_a_release_build_shows_its_version_alone() {
        assert_eq!(build_text("dev "), format!("dev {VERSION}"));
        assert_eq!(build_text(""), VERSION);
    }
}
