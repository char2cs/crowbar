use browser_chrome::BrowserBarEvent;
use browser_nav::{BrowserUrl, SearchEngine, resolve_address};
use gpui::Context;
use pane_host::BrowseAction;
use protocol::PaneId;

use settings_ui::{Key, SettingsBackend as _, Value};

use super::ShellView;
use crate::browser_homepage::{HOMEPAGE_KEY, homepage_state};
use crate::search_engine_setting::{CUSTOM_URL_KEY, ENGINE_KEY, engine_from};

impl ShellView {
    pub(super) fn browse(
        &mut self,
        pane: &PaneId,
        event: BrowserBarEvent,
        _cx: &mut Context<'_, Self>,
    ) {
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
    event: BrowserBarEvent,
    engine: &SearchEngine,
) -> Option<BrowseAction> {
    match event {
        BrowserBarEvent::Go(typed) => {
            let address = resolve_address(&typed, engine)?;
            BrowserUrl::parse(&address).ok().map(BrowseAction::Go)
        },
        BrowserBarEvent::Back => Some(BrowseAction::Back),
        BrowserBarEvent::Forward => Some(BrowseAction::Forward),
        BrowserBarEvent::Reload => Some(BrowseAction::Reload),
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
                BrowserBarEvent::Go((*typed).to_owned()),
                &SearchEngine::DuckDuckGo,
            );
            assert_eq!(action.is_some(), *acts, "{typed:?}");
        }
        assert_eq!(
            action_for(BrowserBarEvent::Back, &SearchEngine::DuckDuckGo),
            Some(BrowseAction::Back)
        );
    }
}
