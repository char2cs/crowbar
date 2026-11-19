use gpui::{Context, Window};
use layout::Command;

use super::ShellView;

mod files;
use crate::focused_chat::focused_chat;
use crate::pane_spec::PaneSpec;

impl ShellView {
    pub(super) fn reopen_closed_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.state.reopen_closed() {
            self.refresh(cx);
        }
    }

    pub(super) fn new_terminal(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("t{}", self.state.serials().next_pane());
        let chat = focused_chat(self.state.layout());
        let spec = terminal_spec(&id, chat.as_deref(), self.data.active_project());
        self.open_spec(spec, cx);
        let layout = self.state.layout();
        let opened = layout.active_tab().and_then(layout::Tab::focused).cloned();
        if let Some(pane) = opened {
            self.source.native().focus_terminal(&pane, window, cx);
        }
    }

    pub(super) fn new_browser(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("b{}", self.state.serials().next_pane());
        let state = self.homepage_state();
        let spec = PaneSpec::browser(&id).map(|spec| PaneSpec { state, ..spec });
        self.open_spec(spec, cx);
    }

    pub(super) fn new_file(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a new file") else {
            return;
        };
        let id = self.state.serials().next_pane();
        self.open_spec(PaneSpec::untitled(&chat, &id), cx);
    }

    pub(super) fn open_review(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a review") else {
            return;
        };
        self.open_spec(PaneSpec::review(&chat), cx);
    }

    pub(super) fn toggle_console(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let spec = match PaneSpec::console() {
            Ok(spec) => spec,
            Err(error) => return tracing::warn!(%error, "the console could not be described"),
        };
        let layout = self.state.layout();
        let open = layout.find_pane(&spec.type_id, &spec.params);
        match open.map(|pane| pane.pane_id.clone()) {
            Some(pane) => self.state.apply(vec![Command::ClosePane { pane }]),
            None => self.state.open_spec(spec),
        }
        self.refresh(cx);
    }

    fn chat_for(
        &self,
        what: &str,
    ) -> Option<String> {
        let chat = focused_chat(self.state.layout());
        if chat.is_none() {
            tracing::info!("{what} needs a chat to be focused first");
        }
        chat
    }

    pub(super) fn open_spec(
        &mut self,
        spec: crate::error::Result<PaneSpec>,
        cx: &mut Context<'_, Self>,
    ) {
        match spec {
            Ok(spec) => {
                self.state.open_spec(spec);
                self.refresh(cx);
            },
            Err(error) => tracing::warn!(%error, "the pane could not be described"),
        }
    }
}

fn terminal_spec(
    id: &str,
    chat: Option<&str>,
    project: Option<String>,
) -> crate::error::Result<PaneSpec> {
    match (chat, project) {
        (None, Some(project)) => PaneSpec::home_terminal(id, &project),
        (chat, _) => PaneSpec::terminal(id, chat, None),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Case {
        name: &'static str,
        chat: Option<&'static str>,
        project: Option<&'static str>,
        param: (&'static str, Option<&'static str>),
    }

    const CASES: &[Case] = &[
        Case {
            name: "no chat but a project opens the project's home terminal",
            chat: None,
            project: Some("p1"),
            param: ("projectId", Some("p1")),
        },
        Case {
            name: "a focused chat owns its terminal",
            chat: Some("c1"),
            project: Some("p1"),
            param: ("threadId", Some("c1")),
        },
        Case {
            name: "neither chat nor project opens a bare terminal",
            chat: None,
            project: None,
            param: ("threadId", None),
        },
    ];

    #[test]
    fn a_new_terminal_belongs_to_the_focused_chat_or_else_the_project() {
        for case in CASES {
            let project = case.project.map(str::to_owned);
            let spec = terminal_spec("t1", case.chat, project).unwrap();
            let (key, expected) = case.param;
            assert_eq!(spec.params.get(key), expected, "{}", case.name);
        }
    }
}
