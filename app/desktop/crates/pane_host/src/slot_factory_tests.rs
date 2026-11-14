use std::rc::Rc;

use protocol::{DaemonRequest, HostId, RelayEndpoint, Responder};
use webview::{Call, MockPlatform};

use super::*;

struct Relay;

impl DaemonTransport for Relay {
    fn request(&self, _request: DaemonRequest, respond: Responder) {
        respond(Err(protocol::Error::Timeout));
    }

    fn relay_endpoint(&self, scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        Ok(RelayEndpoint {
            url: format!("ws://127.0.0.1:1/{}", scope.pane),
            token: "t".into(),
        })
    }
}

fn factory(mock: &Rc<MockPlatform>) -> (SlotFactory, async_channel::Receiver<IpcMessage>) {
    let (sink, inbox) = async_channel::unbounded();
    let config = PaneHostConfig {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
    };
    let webviews = WebViewHost::with_platform(mock.clone());
    (
        SlotFactory::new(webviews, Arc::new(Relay), config, sink),
        inbox,
    )
}

const BOUNDS: Bounds = Bounds {
    x: 0.0,
    y: 0.0,
    width: 10.0,
    height: 10.0,
};

#[test]
fn slots_start_hidden_with_a_bootstrap_naming_their_relay() {
    let mock = Rc::new(MockPlatform::default());
    let (factory, _inbox) = factory(&mock);
    let slot = factory
        .create(
            ParentWindow::X11(3),
            WebViewKind::Core,
            "crowbar://local/x",
            BOUNDS,
        )
        .unwrap();
    assert!(
        matches!(
            mock.calls().first(),
            Some(Call::Create {
                visibility: Visibility::Hidden,
                parent: ParentWindow::X11(3),
                ..
            })
        ),
        "hidden at birth"
    );
    assert_eq!(slot.id, SlotId(0), "first slot id");
    let second = factory
        .create(ParentWindow::X11(3), WebViewKind::Core, "x", BOUNDS)
        .unwrap();
    assert_eq!(second.id, SlotId(1), "ids advance");
}

#[test]
fn ipc_is_attributed_to_the_current_owner() {
    let mock = Rc::new(MockPlatform::default());
    let (factory, inbox) = factory(&mock);
    let slot = factory
        .create(ParentWindow::X11(3), WebViewKind::Core, "x", BOUNDS)
        .unwrap();
    mock.simulate_ipc(slot.handle.id(), "dropped: nobody owns the slot")
        .unwrap();
    assert!(inbox.try_recv().is_err(), "unowned messages are dropped");
    slot.owner.assign(PaneId::new("p1").unwrap());
    mock.simulate_ipc(slot.handle.id(), "hello").unwrap();
    let message = inbox.try_recv().unwrap();
    assert_eq!(
        (message.pane.as_str(), message.text.as_str()),
        ("p1", "hello"),
        "owner"
    );
    slot.owner.assign(PaneId::new("p2").unwrap());
    mock.simulate_ipc(slot.handle.id(), "again").unwrap();
    assert_eq!(
        inbox.try_recv().unwrap().pane.as_str(),
        "p2",
        "follows reassignment"
    );
}

#[test]
fn extension_slots_get_extension_scoped_relays() {
    let mock = Rc::new(MockPlatform::default());
    let (factory, _inbox) = factory(&mock);
    let kind = WebViewKind::Extension(protocol::ExtensionId::new("git").unwrap());
    let script = factory.script_for(SlotId(7), &kind);
    assert!(
        script.contains("ws://127.0.0.1:1/slot-7"),
        "relay for slot 7"
    );
}
