use gpui::{
    AnyElement, App, AppContext as _, ClipboardItem, Context, DismissEvent, Pixels, Point, Window,
};
use layout::Command;
use protocol::PaneId;
use terminalview::Event;
use ui::{Menu, MenuAction, MenuItem, ToastKind};

use crate::panes::kinds::pane_spec::PaneSpec;
use crate::panes::terminals::link_choice::LinkChoice;
use crate::panes::terminals::link_menu::LinkMenu;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;

impl ShellView {
    pub(crate) fn on_terminal(
        &mut self,
        (pane, event): (&PaneId, Event),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            Event::OpenUrl { url, position } => {
                let items = self.link_rows(pane, &url);
                self.open_link_menu((items, position), window, cx);
            },
            Event::OpenFile { path, line } => self.open_printed_file(pane, (&path, line), cx),
            Event::Exited(_) => self.close_terminal(pane, cx),
            Event::FontSizeChanged(size) => self.store_terminal_font_size(size),
            _ => {},
        }
    }

    fn link_rows(
        &self,
        pane: &PaneId,
        url: &str,
    ) -> Vec<MenuItem> {
        LinkChoice::ALL
            .into_iter()
            .map(|choice| MenuItem::Action(self.link_row((pane, url), choice)))
            .collect()
    }

    fn open_link_menu(
        &mut self,
        (items, at): (Vec<MenuItem>, Point<Pixels>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(native) = ui::NativeMenu::installed(cx) {
            return native.open(&items.into(), (at, ui::MenuEdge::Start), (window, cx));
        }
        let menu = cx.new(|cx| Menu::new(items, window, cx));
        cx.subscribe(&menu, |this, _, _: &DismissEvent, cx| {
            this.close_link_menu(cx);
        })
        .detach();
        self.link_menu = Some(LinkMenu { menu, at });
        cx.notify();
    }

    fn link_row(
        &self,
        (pane, url): (&PaneId, &str),
        choice: LinkChoice,
    ) -> MenuAction {
        let requests = self.requests.clone();
        let request = ShellRequest::Link {
            pane: pane.clone(),
            url: url.to_owned(),
            choice,
        };
        MenuAction::new(choice.label(), move |_, _| {
            send_choice(&requests, request.clone());
        })
    }

    pub(crate) fn link_layers(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(LinkMenu { menu, at }) = self.link_menu.as_ref() else {
            return Vec::new();
        };
        let (menu, shell) = (menu.clone(), cx.entity());
        let away = ui::click_away("link-menu-away", move |_, cx: &mut App| {
            shell.update(cx, Self::close_link_menu);
        });
        let popup = ui::popup_at("link-menu", *at, move |_, _| menu.clone());
        vec![away, popup]
    }

    fn close_link_menu(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.link_menu = None;
        cx.notify();
    }

    pub(crate) fn choose_link(
        &mut self,
        (pane, url, choice): (&PaneId, String, LinkChoice),
        cx: &mut Context<'_, Self>,
    ) {
        self.link_menu = None;
        cx.notify();
        match choice {
            LinkChoice::CopyLink => cx.write_to_clipboard(ClipboardItem::new_string(url)),
            _ if self.refuses_loopback(pane, &url, cx) => {},
            LinkChoice::DefaultBrowser => cx.open_url(&url),
            LinkChoice::CrowbarBrowser => self.browse_beside(pane, &url, cx),
        }
    }

    fn refuses_loopback(
        &mut self,
        pane: &PaneId,
        url: &str,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let loopback = browser::PageUrl::parse(url).is_ok_and(|url| url.is_loopback());
        let host = self
            .terminal_chat(pane)
            .and_then(|chat| self.data.chat_remote_host(&chat));
        let Some(host) = host.filter(|_| loopback) else {
            return false;
        };
        let text = format!(
            "This link is on {host}, not this computer — forward the port with ssh -L to open it here"
        );
        self.show_notice(NoticeKey::Terminal, (ToastKind::Info, text), cx);
        true
    }

    fn browse_beside(
        &mut self,
        pane: &PaneId,
        url: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("b{}", self.state.serials().next_pane());
        let state = protocol::PaneState::new(serde_json::json!({ "url": url }));
        let spec = PaneSpec::browser(&id).and_then(|spec| with_state(spec, state));
        self.state
            .apply(vec![Command::FocusPane { pane: pane.clone() }]);
        self.open_spec(spec, cx);
    }
}

fn with_state(
    spec: PaneSpec,
    state: protocol::Result<protocol::PaneState>,
) -> crate::error::Result<PaneSpec> {
    let state = state?;
    Ok(PaneSpec { state, ..spec })
}

fn send_choice(
    requests: &async_channel::Sender<ShellRequest>,
    request: ShellRequest,
) {
    if requests.try_send(request).is_err() {
        tracing::debug!("the shell is closing; a link choice goes unseen");
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::panes::kinds::core_panes::BROWSER;
    use crate::shell::shell_view::test_support::{panes_of, tell, with_terminal};

    fn page_of(
        shell: &ShellView,
        pane: &PaneId,
    ) -> Option<String> {
        let instance = shell.state.layout().pane(pane)?;
        instance
            .state
            .as_value()
            .get("url")?
            .as_str()
            .map(str::to_owned)
    }

    #[gpui::test]
    fn a_url_opens_its_menu_at_the_pointer(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let position = point(px(120.0), px(80.0));
        let url = "https://example.com".to_owned();
        tell(&mut opened, (&terminal, Event::OpenUrl { url, position }));
        let at = opened.shell.update(&mut opened.window, |shell, _| {
            shell.link_menu.as_ref().map(|menu| menu.at)
        });
        assert_eq!(at, Some(position));
    }

    fn row(label: &'static str) -> ui::NativeEntry {
        ui::NativeEntry::Item {
            label: label.into(),
            enabled: true,
            checked: false,
        }
    }

    #[gpui::test]
    fn a_url_asks_for_develops_native_link_menu_and_copy_link_copies_it(cx: &mut TestAppContext) {
        let popped = crate::test_support::answer_menus_with(cx, Some(vec![2]));
        let (mut opened, terminal) = with_terminal(cx);
        let position = point(px(120.0), px(80.0));
        let url = "https://example.com".to_owned();
        tell(&mut opened, (&terminal, Event::OpenUrl { url, position }));
        let want = ui::NativeRequest {
            entries: vec![
                row("Open in Default Browser"),
                row("Open in Crowbar Browser"),
                row("Copy Link"),
            ],
            at: position,
            edge: ui::MenuEdge::Start,
        };
        assert_eq!(*popped.borrow(), [want]);
        let copied = opened
            .window
            .read_from_clipboard()
            .and_then(|item| item.text());
        assert_eq!(copied.as_deref(), Some("https://example.com"));
        let drawn = opened
            .shell
            .update(&mut opened.window, |shell, _| shell.link_menu.is_some());
        assert!(!drawn, "no menu drawn in the window");
    }

    #[gpui::test]
    fn open_in_crowbar_browser_loads_the_page_beside_the_terminal(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let url = "http://localhost:8000/".to_owned();
        let shell = opened.shell.clone();
        let browsers = shell.update(&mut opened.window, |shell, cx| {
            shell.choose_link((&terminal, url, LinkChoice::CrowbarBrowser), cx);
            let browsers = panes_of(shell, BROWSER);
            browsers
                .iter()
                .filter_map(|pane| page_of(shell, pane))
                .collect::<Vec<_>>()
        });
        assert_eq!(browsers, ["http://localhost:8000/"]);
    }
}
