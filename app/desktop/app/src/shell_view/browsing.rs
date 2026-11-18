use std::sync::Arc;

use browser::{PageUrl, SearchEngine, resolve_address};
use browseragent::{AgentCommand, run_peer};
use gpui::Context;
use panehost::BrowseAction;
use protocol::PaneId;

use settings::{Key, SettingsRead as _, Value};

use super::ShellView;
use crate::agent_requests::AgentRequests;
use crate::browser_homepage::{HOMEPAGE_KEY, homepage_state};
use crate::daemon_peer::DaemonPeer;
use crate::search_engine_setting::{CUSTOM_URL_KEY, ENGINE_KEY, engine_from};

impl ShellView {
    pub(super) fn attach_agent_peer(&mut self) {
        let commands = Arc::new(AgentRequests::new(self.requests.clone()));
        let session = self.state.peer_session(commands);
        let peer = DaemonPeer::new(Arc::clone(&self.link.daemon));
        self.link.handle().spawn(async move {
            run_peer(&peer, session).await;
        });
    }

    pub(super) fn run_agent(
        &mut self,
        command: &AgentCommand,
        cx: &mut Context<'_, Self>,
    ) {
        let parent = self.placement.map(|(parent, _)| parent);
        self.state.run_agent(command, parent);
        self.refresh(cx);
    }

    pub(super) fn run_deferred_agent(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let parent = self.placement.map(|(parent, _)| parent);
        self.state.run_deferred(pane, parent);
        self.refresh(cx);
    }

    pub(super) fn browse(
        &mut self,
        pane: &PaneId,
        event: browserbar::Event,
        cx: &mut Context<'_, Self>,
    ) {
        if event == browserbar::Event::TogglePick {
            self.toggle_pick(pane, cx);
            return;
        }
        let engine = self.search_engine();
        let Some(action) = action_for(event, &engine) else {
            return;
        };
        if let Err(error) = self.state.host().browse(pane, &action) {
            tracing::warn!(%error, "the browser pane could not be driven");
        }
    }

    pub(super) fn homepage_state(&mut self) -> protocol::PaneState {
        let engine = self.search_engine();
        let stored = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(HOMEPAGE_KEY)));
        homepage_state(stored.as_ref().and_then(Value::as_text), &engine)
    }

    fn search_engine(&mut self) -> SearchEngine {
        let Some(backend) = self.settings_backend() else {
            return SearchEngine::DuckDuckGo;
        };
        let engine = backend.get(&Key::new(ENGINE_KEY));
        let custom = backend.get(&Key::new(CUSTOM_URL_KEY));
        engine_from(
            engine.as_ref().and_then(Value::as_text),
            custom.as_ref().and_then(Value::as_text),
        )
    }
}

fn action_for(
    event: browserbar::Event,
    engine: &SearchEngine,
) -> Option<BrowseAction> {
    match event {
        browserbar::Event::Go(typed) => {
            let address = resolve_address(&typed, engine)?;
            PageUrl::parse(&address).ok().map(BrowseAction::Go)
        },
        browserbar::Event::Back => Some(BrowseAction::Back),
        browserbar::Event::Forward => Some(BrowseAction::Forward),
        browserbar::Event::Reload => Some(BrowseAction::Reload),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn typed_text_becomes_a_page_or_a_search_and_blank_input_does_nothing() {
        const CASES: &[(&str, bool)] = &[
            ("example.com", true),
            ("rust lifetimes", true),
            ("  ", false),
        ];
        for (typed, acts) in CASES {
            let action = action_for(
                browserbar::Event::Go((*typed).to_owned()),
                &SearchEngine::DuckDuckGo,
            );
            assert_eq!(action.is_some(), *acts, "{typed:?}");
        }
        assert_eq!(
            action_for(browserbar::Event::Back, &SearchEngine::DuckDuckGo),
            Some(BrowseAction::Back)
        );
    }
}
