use gpui::{AppContext as _, Context, Entity};
use oobe_view::{OobeEvent, OobeView};
use sidebar::{Command, CommandSink as _, Host, HostId, HostKind, SidebarData as _};

use super::ShellView;
use crate::hosts::Hosts;
use crate::landing::{Landing, landing_for, space_name};
use crate::request_queue::send;
use crate::shell_request::ShellRequest;

impl ShellView {
    pub(super) fn update_landing(
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
                oobe.update(cx, OobeView::changed);
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
            _ => Some(cx.new(|_| OobeView::add_host())),
        };
        let Some(oobe) = oobe else {
            return;
        };
        self.subscriptions.push(cx.subscribe(&oobe, Self::on_oobe));
        self.first_run = Some((landing, oobe));
        self.update_overlay(cx);
    }

    fn first_run_view(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Entity<OobeView>> {
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let check = self
            .data
            .system_check(signal)
            .inspect_err(|error| tracing::warn!(%error, "the system check could not start"))
            .ok()?;
        Some(cx.new(|_| OobeView::first_run(check)))
    }

    fn leave_first_run(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.first_run.take().is_some() {
            self.update_overlay(cx);
        }
    }

    #[expect(
        clippy::trivially_copy_pass_by_ref,
        reason = "gpui subscribe passes the event by reference"
    )]
    fn on_oobe(
        &mut self,
        _: Entity<OobeView>,
        event: &OobeEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            OobeEvent::ChooseFolder => self.choose_first_space(cx),
            OobeEvent::AddHost => self.queue_pairing(),
            _ => {},
        }
    }

    pub(super) fn create_first_space(
        &mut self,
        folder: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(host) = self.local_host_id() else {
            return;
        };
        let name = space_name(folder).unwrap_or_else(|| folder.to_owned());
        let command = Command::CreateSpace {
            name,
            folder: folder.to_owned(),
            host,
        };
        if let Err(error) = self.data.commands().dispatch(command) {
            tracing::warn!(%error, "the first space could not be created");
            self.show_notice(
                crate::notice_key::NoticeKey::Command,
                (ui::ToastKind::Error, error.to_string()),
                cx,
            );
        }
    }

    fn choose_first_space(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(host) = self.local_host_id() {
            self.choose_space_folder(host.as_str(), cx);
        }
    }

    fn local_host_id(&self) -> Option<HostId> {
        local_host(self.data.view().hosts())
    }

    fn queue_pairing(&self) {
        send(
            &self.requests,
            ShellRequest::Outcome(shell_data::Outcome::PairHost),
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
