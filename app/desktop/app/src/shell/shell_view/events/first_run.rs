mod hosts;
pub(crate) mod landing;

use gpui::{AppContext as _, Context, Entity, Window};
use sidebar::{Host, HostChoice as _, HostId, HostKind, SpaceList as _};

use crate::shell::inbox::data_signal;
use crate::shell::inbox::request_queue::send;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_view::ShellView;
use crate::shell::shell_view::events::first_run::hosts::Hosts;
use crate::shell::shell_view::events::first_run::landing::{Landing, landing_for};

impl ShellView {
    pub(crate) fn update_landing(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let view = self.data.view();
        let hosts = view.hosts();
        let spaces = view.projects().len();
        let census = Hosts {
            has_local: self.data.has_local_host(),
            paired: hosts.len(),
        };
        let landing = landing_for(spaces, view.sync(), &census);
        match (&self.first_run, landing) {
            (Some((shown, oobe)), now) if *shown == now => {
                oobe.update(cx, oobe::View::changed);
            },
            (_, Landing::Home) => self.leave_first_run(cx),
            (_, now) => self.enter_first_run(now, cx),
        }
    }

    fn enter_first_run(
        &mut self,
        landing: Landing,
        cx: &mut Context<'_, Self>,
    ) {
        let oobe = match landing {
            Landing::FirstRun => self.first_run_view(cx),
            _ => Some(cx.new(|_| oobe::View::add_host())),
        };
        let Some(oobe) = oobe else {
            return;
        };
        self.subscriptions
            .push(cx.subscribe(&oobe, |this, _, event: &oobe::Event, _| {
                send(&this.requests, ShellRequest::FirstRun(*event));
            }));
        self.first_run = Some((landing, oobe));
        cx.notify();
    }

    fn first_run_view(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Entity<oobe::View>> {
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let check = self
            .data
            .system_check(signal)
            .inspect_err(|error| tracing::warn!(%error, "the system check could not start"))
            .ok()?;
        Some(cx.new(|_| oobe::View::first_run(check)))
    }

    fn leave_first_run(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.first_run.take().is_some() {
            cx.notify();
        }
    }

    pub(crate) fn on_oobe(
        &mut self,
        event: oobe::Event,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            oobe::Event::ChooseFolder => {
                self.show_folder_import(folderimport::Kind::Space, window, cx);
            },
            oobe::Event::AddHost => self.queue_pairing(),
            oobe::Event::MoveWindow => window.start_window_move(),
            _ => {},
        }
    }

    pub(crate) fn local_host_id(&self) -> Option<HostId> {
        local_host(self.data.view().hosts())
    }

    fn queue_pairing(&self) {
        send(
            &self.requests,
            ShellRequest::Outcome(store::Outcome::PairHost),
        );
    }
}

fn local_host(hosts: Vec<Host>) -> Option<HostId> {
    hosts
        .into_iter()
        .find(|host| host.kind == HostKind::Local)
        .map(|host| host.id)
}

#[cfg(test)]
mod tests {
    use sidebar::HostState;

    use super::*;

    fn host(
        id: &str,
        kind: HostKind,
    ) -> Host {
        Host {
            id: HostId::from(id),
            name: id.to_owned(),
            kind,
            state: HostState::Unknown,
            endpoint: None,
        }
    }

    #[test]
    fn the_first_space_goes_to_this_computers_own_daemon() {
        let hosts = vec![host("box", HostKind::Remote), host("here", HostKind::Local)];
        assert_eq!(local_host(hosts), Some(HostId::from("here")));
        assert_eq!(local_host(vec![host("box", HostKind::Remote)]), None);
        assert_eq!(local_host(Vec::new()), None);
    }
}
