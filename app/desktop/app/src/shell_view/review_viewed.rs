//! The review's viewed marks between the store, which owns them, and the review panes.

use std::collections::BTreeMap;

use gpui::Context;
use protocol::{PaneId, ShellFrame};

use crate::pane_spec::PaneSpec;
use crate::review_panes::{ViewedChanges, review_chat, review_panes};
use crate::shell_view::ShellView;

fn panes_of_workspace(
    panes: Vec<(PaneId, String)>,
    workspace: &str,
    workspace_of: impl Fn(&str) -> Option<String>,
) -> Vec<PaneId> {
    panes
        .into_iter()
        .filter(|(_, chat)| workspace_of(chat).as_deref() == Some(workspace))
        .map(|(pane, _)| pane)
        .collect()
}

impl ShellView {
    pub(super) fn serve_review_viewed(
        &self,
        mounted: &[PaneId],
        viewed: Vec<(PaneId, ViewedChanges)>,
    ) {
        let layout = self.state.layout();
        let marked = viewed
            .into_iter()
            .filter_map(|(pane, changes)| Some((review_chat(layout, &pane)?, changes)));
        for (chat, changes) in marked {
            self.data.commands().set_review_viewed(&chat, &changes);
        }
        for pane in mounted {
            self.send_viewed_to(pane);
        }
    }

    fn send_viewed_to(
        &self,
        pane: &PaneId,
    ) {
        let chat = review_chat(self.state.layout(), pane);
        let Some((workspace, viewed)) =
            chat.and_then(|chat| self.data.commands().review_viewed(&chat))
        else {
            return;
        };
        self.send_review_frame(pane, &ShellFrame::ReviewViewed { workspace, viewed });
    }

    pub(super) fn push_review_viewed(
        &self,
        workspace: &str,
        viewed: &BTreeMap<String, String>,
    ) {
        let frame = ShellFrame::ReviewViewed {
            workspace: workspace.to_owned(),
            viewed: viewed.clone(),
        };
        let panes = review_panes(self.state.layout());
        let workspace_of = |chat: &str| self.data.commands().review_viewed(chat).map(|(of, _)| of);
        for pane in panes_of_workspace(panes, workspace, workspace_of) {
            self.send_review_frame(&pane, &frame);
        }
    }

    pub(super) fn open_review_at(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a review") else {
            return;
        };
        let open = review_panes(self.state.layout())
            .into_iter()
            .find(|(_, of)| *of == chat);
        let Some((pane, _)) = open else {
            self.open_spec(PaneSpec::review(&chat, Some(path)), cx);
            return;
        };
        self.open_spec(PaneSpec::review(&chat, None), cx);
        let reveal = ShellFrame::ReviewReveal {
            path: path.to_owned(),
        };
        self.send_review_frame(&pane, &reveal);
    }

    fn send_review_frame(
        &self,
        pane: &PaneId,
        frame: &ShellFrame,
    ) {
        if let Err(error) = self.state.host().send(pane, frame) {
            tracing::warn!(%error, "a review pane could not be told");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_change_reaches_only_the_review_panes_of_its_workspace() {
        let pane = |id: &str| PaneId::new(id).unwrap();
        let panes = vec![
            (pane("r1"), "c1".to_owned()),
            (pane("r2"), "c2".to_owned()),
            (pane("r3"), "unknown".to_owned()),
        ];
        let workspace_of = |chat: &str| (chat != "unknown").then(|| format!("w-{chat}"));
        assert_eq!(
            panes_of_workspace(panes, "w-c2", workspace_of),
            [pane("r2")]
        );
    }
}
