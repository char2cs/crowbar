use std::cell::Cell;
use std::sync::Arc;

use webview::{Bounds, IpcHandler, Visibility, WebViewConfig, WebViewHost, WebViewKind};

use crate::bootstrap::bootstrap_script;
use crate::browser_slot::page_handler;
use crate::error::Result;
use crate::ipc_envelope::open_envelope;
use crate::ipc_message::IpcMessage;
use crate::pane_host_config::PaneHostConfig;
use crate::slot::Slot;
use crate::slot_id::SlotId;
use crate::slot_owner::SlotOwner;
use crate::slot_request::{BLANK_URL, SlotRequest};

pub(crate) const INBOX_CAPACITY: usize = 256;
const MAX_MESSAGE_BYTES: usize = 1 << 20;

pub(crate) struct SlotFactory {
    webviews: WebViewHost,
    config: PaneHostConfig,
    sink: async_channel::Sender<IpcMessage>,
    next: Cell<u64>,
}

impl SlotFactory {
    pub(crate) fn new(
        webviews: WebViewHost,
        config: PaneHostConfig,
        sink: async_channel::Sender<IpcMessage>,
    ) -> Self {
        Self {
            webviews,
            config,
            sink,
            next: Cell::new(0),
        }
    }

    pub(crate) fn create(
        &self,
        request: SlotRequest,
    ) -> Result<Slot> {
        let SlotRequest {
            parent,
            kind,
            bounds,
        } = request;
        let id = SlotId(self.next.get());
        self.next.set(id.0 + 1);
        let owner = SlotOwner::default();
        let config = self.config_for(&kind, bounds, &owner);
        let handle = self.webviews.create_in(parent, config)?;
        Ok(Slot {
            id,
            kind,
            handle,
            owner,
        })
    }

    fn config_for(
        &self,
        kind: &WebViewKind,
        bounds: Bounds,
        owner: &SlotOwner,
    ) -> WebViewConfig {
        if *kind == WebViewKind::Browser {
            return self.browser_config(bounds, owner);
        }
        self.pane_config((kind.clone(), bounds), owner)
    }

    // SPEC: a browser web view gets no bootstrap script, no script bridge and no relay token, so a page the user browses to can never reach the daemon.
    fn browser_config(
        &self,
        bounds: Bounds,
        owner: &SlotOwner,
    ) -> WebViewConfig {
        WebViewConfig {
            url: BLANK_URL.into(),
            bounds,
            kind: WebViewKind::Browser,
            visibility: Visibility::Hidden,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: Some(page_handler(owner.clone(), self.sink.clone())),
        }
    }

    fn pane_config(
        &self,
        (kind, bounds): (WebViewKind, Bounds),
        owner: &SlotOwner,
    ) -> WebViewConfig {
        WebViewConfig {
            url: BLANK_URL.into(),
            bounds,
            kind,
            visibility: Visibility::Hidden,
            initialization_script: Some(bootstrap_script(&self.config.scheme, &self.config.host)),
            on_ipc: self.ipc_for(owner),
            on_page: self.config.page_handler.clone(),
        }
    }

    fn ipc_for(
        &self,
        owner: &SlotOwner,
    ) -> IpcHandler {
        let owner = owner.clone();
        let sink = self.sink.clone();
        Arc::new(move |text| deliver(&owner, &sink, &text))
    }
}

fn deliver(
    owner: &SlotOwner,
    sink: &async_channel::Sender<IpcMessage>,
    text: &str,
) {
    if text.len() > MAX_MESSAGE_BYTES {
        tracing::warn!(bytes = text.len(), "dropping an oversized pane message");
        return;
    }
    let Some((generation, frame)) = open_envelope(text) else {
        return;
    };
    let Some(pane) = owner.resolve(generation) else {
        return;
    };
    let message = IpcMessage {
        pane,
        text: frame.to_owned(),
    };
    if sink.try_send(message).is_err() {
        tracing::warn!("pane host inbox is full or closed; dropping a pane message");
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use protocol::{HostId, PaneId};
    use webview::{Bounds, Call, MockPlatform, ParentWindow, WebViewKind};

    use super::*;

    fn factory(mock: &Rc<MockPlatform>) -> (SlotFactory, async_channel::Receiver<IpcMessage>) {
        let (sink, inbox) = async_channel::bounded(INBOX_CAPACITY);
        let config = PaneHostConfig {
            scheme: "crowbar".into(),
            host: HostId::new("local").unwrap(),
            max_idle_slots: 1,
            page_handler: None,
        };
        let webviews = WebViewHost::with_platform(Rc::<MockPlatform>::clone(mock));
        (SlotFactory::new(webviews, config, sink), inbox)
    }

    fn square() -> Bounds {
        Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        }
    }

    fn request() -> SlotRequest {
        SlotRequest {
            parent: ParentWindow::X11(3),
            kind: WebViewKind::Core,
            bounds: square(),
        }
    }

    #[test]
    fn slots_are_born_hidden_and_blank_so_nothing_loads_before_the_origin_is_known() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, _inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        assert!(
            matches!(
                mock.calls().first(),
                Some(Call::Create { url, visibility: Visibility::Hidden, parent: ParentWindow::X11(3), .. })
                    if url == BLANK_URL
            ),
            "{:?}",
            mock.calls()
        );
        assert_eq!(slot.id, SlotId(0), "first slot id");
        assert_eq!(
            factory.create(request()).unwrap().id,
            SlotId(1),
            "ids advance"
        );
    }

    enum Page {
        Unowned,
        Current,
        Stale,
    }

    const CASES: &[(&str, Page, bool)] = &[
        ("nobody owns the slot", Page::Unowned, false),
        ("the owner's current page", Page::Current, true),
        (
            "a page left over from the previous pane",
            Page::Stale,
            false,
        ),
    ];

    fn generation_for(
        page: &Page,
        stale: u64,
        current: u64,
    ) -> u64 {
        if matches!(page, Page::Stale) {
            return stale;
        }
        current
    }

    fn delivered(page: &Page) -> bool {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        let stale = slot.owner.assign(PaneId::new("old").unwrap());
        let current = slot.owner.assign(PaneId::new("new").unwrap());
        if matches!(page, Page::Unowned) {
            slot.owner.clear();
        }
        let generation = generation_for(page, stale, current);
        mock.simulate_ipc(slot.handle.id(), &format!("{generation}:hello"))
            .unwrap();
        inbox.try_recv().is_ok()
    }

    #[test]
    fn only_the_current_page_of_the_owner_is_delivered() {
        for (name, page, expected) in CASES {
            assert_eq!(delivered(page), *expected, "{name}");
        }
    }

    #[test]
    fn ipc_is_attributed_to_the_page_generation_of_the_owner() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        let first = slot.owner.assign(PaneId::new("p1").unwrap());
        mock.simulate_ipc(slot.handle.id(), &format!("{first}:hello"))
            .unwrap();
        let message = inbox.try_recv().unwrap();
        assert_eq!(
            (message.pane.as_str(), message.text.as_str()),
            ("p1", "hello"),
            "owner"
        );
        let second = slot.owner.assign(PaneId::new("p2").unwrap());
        mock.simulate_ipc(slot.handle.id(), &format!("{first}:from the old page"))
            .unwrap();
        assert!(
            inbox.try_recv().is_err(),
            "the previous pane's page is stale"
        );
        mock.simulate_ipc(slot.handle.id(), &format!("{second}:again"))
            .unwrap();
        assert_eq!(
            inbox.try_recv().unwrap().pane.as_str(),
            "p2",
            "the new page"
        );
    }

    #[test]
    fn oversized_messages_are_dropped_and_a_full_inbox_does_not_block() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        let generation = slot.owner.assign(PaneId::new("p1").unwrap());
        let huge = format!("{generation}:{}", "x".repeat(MAX_MESSAGE_BYTES));
        mock.simulate_ipc(slot.handle.id(), &huge).unwrap();
        assert!(inbox.try_recv().is_err(), "oversized");
        for _ in 0..=INBOX_CAPACITY {
            mock.simulate_ipc(slot.handle.id(), &format!("{generation}:m"))
                .unwrap();
        }
        assert_eq!(inbox.len(), INBOX_CAPACITY, "excess messages are dropped");
    }
}
