use std::cell::RefCell;
use std::collections::HashSet;

use gpui::{App, Bounds, Pixels};
use startup_view::{StartupActions, StartupCommand};
use ui::{OverlayHost, OverlayId};

use crate::shell_request::ShellRequest;

type Requests = async_channel::Sender<ShellRequest>;

fn send(requests: &Requests, request: ShellRequest) {
    if requests.try_send(request).is_err() {
        tracing::warn!("the shell is closing; a request was dropped");
    }
}

/// Where the sidebar sends its commands: onto the shell's request queue.
#[derive(Debug, Clone)]
pub(crate) struct SidebarRequests(pub(crate) Requests);

impl sidebar::CommandSink for SidebarRequests {
    fn dispatch(&self, command: sidebar::Command) -> Result<(), sidebar::Error> {
        self.0
            .try_send(ShellRequest::Sidebar(command))
            .map_err(|_| sidebar::Error::Refused {
                reason: "the shell is closing".into(),
            })
    }
}

/// The buttons of the startup view.
#[derive(Debug, Clone)]
pub(crate) struct StartupRequests(pub(crate) Requests);

impl StartupActions for StartupRequests {
    fn perform(&self, command: StartupCommand, _cx: &mut App) {
        send(&self.0, ShellRequest::Startup(command));
    }
}

/// Counts the open overlays of the views so web views can step aside while any is up.
#[derive(Debug)]
pub(crate) struct OverlayRequests {
    requests: Requests,
    open: RefCell<HashSet<OverlayId>>,
}

impl OverlayRequests {
    pub(crate) fn new(requests: Requests) -> Self {
        Self {
            requests,
            open: RefCell::new(HashSet::new()),
        }
    }

    fn report(&self) {
        let open = !self.open.borrow().is_empty();
        send(&self.requests, ShellRequest::Overlays { open });
    }
}

impl OverlayHost for OverlayRequests {
    fn overlay_changed(&self, overlay: OverlayId, _bounds: Bounds<Pixels>) {
        if self.open.borrow_mut().insert(overlay) {
            self.report();
        }
    }

    fn overlay_closed(&self, overlay: OverlayId) {
        if self.open.borrow_mut().remove(&overlay) {
            self.report();
        }
    }
}

#[cfg(test)]
#[path = "request_sinks_tests.rs"]
mod tests;
