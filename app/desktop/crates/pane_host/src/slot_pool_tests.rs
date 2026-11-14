use std::rc::Rc;
use std::sync::Arc;

use webview::{Bounds, MockPlatform, ParentWindow, Visibility, WebViewConfig, WebViewHost};

use super::*;
use crate::slot_owner::SlotOwner;

fn slot(host: &WebViewHost, id: u64, kind: WebViewKind) -> Slot {
    let config = WebViewConfig {
        url: String::new(),
        bounds: Bounds {
            x: 0.0,
            y: 0.0,
            width: 1.0,
            height: 1.0,
        },
        kind: kind.clone(),
        visibility: Visibility::Hidden,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
    };
    let handle = host.create_in(ParentWindow::X11(1), config).unwrap();
    Slot {
        id: SlotId(id),
        kind,
        handle,
        owner: SlotOwner::default(),
    }
}

fn host() -> WebViewHost {
    WebViewHost::with_platform(Rc::new(MockPlatform::default()))
}

#[test]
fn a_free_slot_of_the_right_kind_is_found() {
    let host = host();
    let mut pool = SlotPool::new(2);
    let ext = WebViewKind::Extension(protocol::ExtensionId::new("git").unwrap());
    let core = pool.insert(slot(&host, 1, WebViewKind::Core));
    pool.insert(slot(&host, 2, ext.clone()));
    assert_eq!(pool.find_free(&WebViewKind::Core), Some(core), "core");
    assert!(pool.find_free(&ext).is_some(), "extension");
    pool.get(core)
        .unwrap()
        .owner
        .assign(protocol::PaneId::new("p").unwrap());
    assert_eq!(pool.find_free(&WebViewKind::Core), None, "taken");
}

#[test]
fn released_core_slots_are_kept_up_to_the_idle_limit() {
    let host = host();
    let mut pool = SlotPool::new(1);
    let ids: Vec<SlotId> = (0..3)
        .map(|n| pool.insert(slot(&host, n, WebViewKind::Core)))
        .collect();
    for id in &ids {
        pool.get(*id)
            .unwrap()
            .owner
            .assign(protocol::PaneId::new("p").unwrap());
    }
    for id in &ids {
        pool.release(*id);
    }
    assert_eq!(pool.len(), 1, "only one idle slot is kept");
}

#[test]
fn extension_slots_are_never_reused() {
    let host = host();
    let mut pool = SlotPool::new(4);
    let ext = WebViewKind::Extension(protocol::ExtensionId::new("git").unwrap());
    let id = pool.insert(slot(&host, 1, ext));
    pool.get(id)
        .unwrap()
        .owner
        .assign(protocol::PaneId::new("p").unwrap());
    pool.release(id);
    assert_eq!(pool.len(), 0, "destroyed on release");
}
