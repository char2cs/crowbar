use browser::PageState;
use browser_agent::TabInfo;
use layout::{Layout, Tab};
use protocol::PaneId;

use crate::browser_pages::BrowserPages;
use crate::core_panes::BROWSER;

const THREAD_ID: &str = "threadId";
const URL: &str = "url";

fn thread_of(
    tab: &Tab,
    pane: &PaneId,
) -> Option<String> {
    let instance = tab.pane(pane)?;
    instance.params.get(THREAD_ID).map(str::to_owned)
}

fn is_browser(
    tab: &Tab,
    pane: &PaneId,
) -> bool {
    tab.pane(pane)
        .is_some_and(|instance| instance.type_id.as_str() == BROWSER)
}

fn owner_chat(
    tab: &Tab,
    pane: &PaneId,
) -> Option<String> {
    thread_of(tab, pane).or_else(|| {
        tab.pane_ids()
            .iter()
            .find_map(|other| thread_of(tab, other))
    })
}

pub(crate) fn chat_tab<'a>(
    layout: &'a Layout,
    chat: &str,
) -> Option<&'a Tab> {
    layout.tabs().iter().find(|tab| {
        tab.pane_ids()
            .iter()
            .any(|pane| thread_of(tab, pane).as_deref() == Some(chat))
    })
}

pub(crate) fn owned_browser(
    layout: &Layout,
    chat: &str,
    tab_id: &str,
) -> Option<PaneId> {
    let pane = PaneId::new(tab_id).ok()?;
    let tab = layout.tab_of(&pane)?;
    let owned = is_browser(tab, &pane) && owner_chat(tab, &pane).as_deref() == Some(chat);
    owned.then_some(pane)
}

pub(crate) fn chat_browser(
    layout: &Layout,
    chat: &str,
) -> Option<PaneId> {
    let tab = chat_tab(layout, chat)?;
    tab.pane_ids()
        .into_iter()
        .find(|pane| is_browser(tab, pane) && owner_chat(tab, pane).as_deref() == Some(chat))
}

pub(crate) fn address_of(
    layout: &Layout,
    pages: &BrowserPages,
    pane: &PaneId,
) -> String {
    let shown = pages.get(pane).map(|state| state.url().to_owned());
    let saved = layout
        .pane(pane)
        .and_then(|instance| instance.state.as_value().get(URL))
        .and_then(serde_json::Value::as_str)
        .map(str::to_owned);
    shown
        .filter(|url| !url.is_empty())
        .or(saved)
        .unwrap_or_default()
}

fn owned_by_chat(
    tab: &Tab,
    pane: PaneId,
) -> Option<(PaneId, String)> {
    let chat = owner_chat(tab, &pane)?;
    is_browser(tab, &pane).then_some((pane, chat))
}

fn browsers_of(tab: &Tab) -> Vec<(PaneId, String)> {
    let panes = tab.pane_ids();
    panes
        .into_iter()
        .filter_map(|pane| owned_by_chat(tab, pane))
        .collect()
}

fn tab_info(
    layout: &Layout,
    pages: &BrowserPages,
    (pane, chat_id): (PaneId, String),
) -> TabInfo {
    let state = pages.get(&pane);
    TabInfo {
        url: address_of(layout, pages, &pane),
        title: state
            .and_then(PageState::title)
            .unwrap_or_default()
            .to_owned(),
        loading: state.is_some_and(PageState::is_loading),
        id: pane.to_string(),
        chat_id,
    }
}

pub(crate) fn browser_tabs(
    layout: &Layout,
    pages: &BrowserPages,
) -> Vec<TabInfo> {
    let browsers = layout.tabs().iter().flat_map(browsers_of);
    browsers
        .map(|browser| tab_info(layout, pages, browser))
        .collect()
}

#[cfg(test)]
mod tests {
    use browser::PageUpdate;
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;

    type Seed = (u64, &'static str, &'static str, Params, PaneState);

    fn open(
        layout: &mut Layout,
        seed: Seed,
    ) {
        let (tab, name, type_id, params, state) = seed;
        let tab = TabId(tab);
        if layout.tab(tab).is_none() {
            let title = String::new();
            layout.apply(Command::AddTab { id: tab, title }).unwrap();
        }
        let instance = PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params,
            state,
        };
        let placement = None;
        layout
            .apply(Command::OpenPane {
                tab,
                instance,
                placement,
            })
            .unwrap();
    }

    fn thread(chat: &str) -> Params {
        Params::default().with(THREAD_ID, chat).unwrap()
    }

    fn at(url: &str) -> PaneState {
        PaneState::new(json!({ "url": url })).unwrap()
    }

    fn two_chats() -> Layout {
        let mut layout = Layout::default();
        let none = || (Params::default(), PaneState::default());
        let seeds: Vec<Seed> = vec![
            (1, "p1", "core.chat", thread("c1"), PaneState::default()),
            (1, "b1", BROWSER, none().0, at("https://a.test/")),
            (2, "p2", "core.chat", thread("c2"), PaneState::default()),
            (2, "b2", BROWSER, thread("c2"), PaneState::default()),
            (3, "t1", "core.terminal", none().0, none().1),
            (3, "b3", BROWSER, none().0, none().1),
        ];
        for seed in seeds {
            open(&mut layout, seed);
        }
        layout
    }

    #[test]
    fn the_tabs_of_the_daemon_are_the_browsers_that_belong_to_a_chat_with_the_page_state_over_the_saved_address()
     {
        let layout = two_chats();
        let mut pages = BrowserPages::default();
        let all = |pages: &BrowserPages| browser_tabs(&layout, pages);
        let owners: Vec<(String, String)> = all(&pages)
            .into_iter()
            .map(|tab| (tab.id, tab.chat_id))
            .collect();
        let expected = [("b1", "c1"), ("b2", "c2")].map(|(a, b)| (a.to_owned(), b.to_owned()));
        assert_eq!(
            owners, expected,
            "a browser in a tab with no chat belongs to nobody"
        );
        assert_eq!(all(&pages)[0].url, "https://a.test/", "the saved address");
        let first = PaneId::new("b1").unwrap();
        pages.apply(
            first.clone(),
            PageUpdate::Loading {
                url: "https://b.test/x".into(),
            },
        );
        pages.apply(first, PageUpdate::title("B"));
        let tab = all(&pages).remove(0);
        assert_eq!(
            (tab.url.as_str(), tab.title.as_str(), tab.loading),
            ("https://b.test/x", "B", true)
        );
    }

    #[test]
    fn a_chat_reaches_only_its_own_browsers() {
        let layout = two_chats();
        let reach =
            |chat: &str, tab: &str| owned_browser(&layout, chat, tab).map(|pane| pane.to_string());
        let cases = [
            ("c1", "b1", Some("b1")),
            ("c1", "b2", None),
            ("c2", "b2", Some("b2")),
            ("c1", "p1", None),
            ("c1", "nope", None),
            ("c2", "b3", None),
        ];
        for (chat, tab, expected) in cases {
            assert_eq!(reach(chat, tab).as_deref(), expected, "{chat} {tab}");
        }
    }

    #[test]
    fn a_chat_finds_its_tab_and_its_browser() {
        let layout = two_chats();
        let tab_of = |chat: &str| chat_tab(&layout, chat).map(Tab::id);
        assert_eq!(tab_of("c2"), Some(TabId(2)));
        assert_eq!(tab_of("c9"), None);
        let browser = |chat: &str| chat_browser(&layout, chat).map(|pane| pane.to_string());
        assert_eq!(browser("c1").as_deref(), Some("b1"));
        assert_eq!(browser("c9"), None);
    }
}
