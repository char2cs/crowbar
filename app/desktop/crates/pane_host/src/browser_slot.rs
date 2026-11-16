use std::sync::Arc;

use browser_nav::{BROWSER_PANE_TYPE, BrowserUrl, PageUpdate};
use protocol::{PaneState, PaneTypeId};
use webview::{IpcHandler, PageEvent, PageHandler};

use crate::host_event::HostEvent;
use crate::ipc_message::IpcMessage;
use crate::slot_owner::SlotOwner;
use crate::slot_request::BLANK_URL;

const URL_KEY: &str = "url";

pub(crate) fn is_browser(type_id: &PaneTypeId) -> bool {
    type_id.as_str() == BROWSER_PANE_TYPE
}

// SPEC: params carry ids only, so a browser pane's address lives in its saved state.
pub(crate) fn first_url(state: Option<&PaneState>) -> String {
    state
        .and_then(|state| state.as_value().get(URL_KEY))
        .and_then(serde_json::Value::as_str)
        .and_then(|wanted| BrowserUrl::parse(wanted).ok())
        .map_or_else(|| BLANK_URL.to_owned(), |url| url.as_str().to_owned())
}

// SPEC: the page gets no script bridge; this engine callback is its only channel, and a navigation to a scheme a browser pane may not load is cancelled.
pub(crate) fn page_handler(
    owner: SlotOwner,
    sink: async_channel::Sender<IpcMessage>,
) -> PageHandler {
    Arc::new(move |event| on_page(&owner, &sink, event))
}

pub(crate) fn pick_handler(
    owner: SlotOwner,
    sink: async_channel::Sender<IpcMessage>,
) -> IpcHandler {
    Arc::new(move |text| forward_pick(&owner, &sink, &text))
}

fn forward_pick(
    owner: &SlotOwner,
    sink: &async_channel::Sender<IpcMessage>,
    text: &str,
) {
    if let Some(update) = PageUpdate::from_pick_message(text) {
        send_update(owner, sink, &update);
    }
}

fn send_update(
    owner: &SlotOwner,
    sink: &async_channel::Sender<IpcMessage>,
    update: &PageUpdate,
) {
    let Some(pane) = owner.current() else {
        return;
    };
    let message = IpcMessage {
        pane,
        text: update.encode(),
    };
    if sink.try_send(message).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a page update");
    }
}

fn on_page(
    owner: &SlotOwner,
    sink: &async_channel::Sender<IpcMessage>,
    event: PageEvent,
) -> bool {
    let update = match event {
        PageEvent::Navigating(url) => return BrowserUrl::is_loadable(&url),
        PageEvent::Loading(url) => PageUpdate::Loading { url },
        PageEvent::Loaded(url) => PageUpdate::Loaded { url },
        PageEvent::Title(title) => PageUpdate::title(&title),
        _ => return true,
    };
    send_update(owner, sink, &update);
    true
}

pub(crate) fn browser_event(
    message: &IpcMessage,
    type_id: &PaneTypeId,
) -> Option<Vec<HostEvent>> {
    if !is_browser(type_id) {
        return None;
    }
    let events = PageUpdate::decode(&message.text)
        .map(|update| HostEvent::BrowserPage {
            pane: message.pane.clone(),
            update,
        })
        .into_iter()
        .collect();
    Some(events)
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;

    #[test]
    fn a_browser_pane_loads_only_what_a_browser_may_load() {
        let cases = [
            ("https://example.com/", "https://example.com/"),
            ("file:///etc/passwd", BLANK_URL),
            ("crowbar://local/panes/core/chat/index.html", BLANK_URL),
        ];
        for (wanted, expected) in cases {
            let state = PaneState::new(serde_json::json!({ "url": wanted })).unwrap();
            assert_eq!(first_url(Some(&state)), expected, "{wanted}");
        }
        assert_eq!(first_url(None), BLANK_URL);
    }

    #[test]
    fn page_events_reach_the_owner_and_forbidden_navigation_is_cancelled() {
        let (sink, inbox) = async_channel::bounded(8);
        let owner = SlotOwner::default();
        let handler = page_handler(owner.clone(), sink);
        assert!(handler(PageEvent::Loaded("https://a.test/".into())));
        assert!(inbox.try_recv().is_err(), "nobody owns the view yet");
        owner.assign(PaneId::new("p1").unwrap());
        assert!(handler(PageEvent::Title("hi".into())));
        let message = inbox.try_recv().unwrap();
        assert_eq!(message.pane.as_str(), "p1");
        assert_eq!(
            PageUpdate::decode(&message.text),
            Some(PageUpdate::title("hi"))
        );
        for url in [
            "file:///etc/passwd",
            "crowbar://local/x",
            "javascript:alert(1)",
        ] {
            assert!(!handler(PageEvent::Navigating(url.into())), "{url}");
        }
        assert!(handler(PageEvent::Navigating("https://a.test/".into())));
    }

    #[test]
    fn a_page_can_post_only_the_result_of_a_pick() {
        let (sink, inbox) = async_channel::bounded(8);
        let owner = SlotOwner::default();
        owner.assign(PaneId::new("p1").unwrap());
        let handler = pick_handler(owner, sink);
        handler(r#"{"page":"loaded","url":"https://a.test/"}"#.into());
        handler(r#"{"sdk":1,"kind":"command.run","command":"x"}"#.into());
        assert!(inbox.try_recv().is_err(), "foreign posts are dropped");
        handler(r#"{"crowbarPick":{"result":null}}"#.into());
        let message = inbox.try_recv().unwrap();
        assert_eq!(message.pane.as_str(), "p1");
        assert_eq!(
            PageUpdate::decode(&message.text),
            Some(PageUpdate::PickEnded)
        );
    }

    #[test]
    fn text_from_a_page_is_never_a_page_update_unless_it_is_one() {
        let frame = r#"{"sdk":1,"kind":"command.run","command":"x"}"#;
        assert_eq!(PageUpdate::decode(frame), None);
    }
}
