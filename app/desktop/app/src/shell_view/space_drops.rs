use gpui::Context;
use layout::{Placement, TabId};
use ui::ToastKind;

use super::ShellView;
use crate::notice_key::NoticeKey;

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/components/sidebar/lib/pane-drop-actions.ts
const OTHER_SPACE: &str = "That chat belongs to a different space";

impl ShellView {
    pub(super) fn drop_chat(
        &mut self,
        chat: &str,
        (tab, placement): (TabId, &Placement),
        cx: &mut Context<'_, Self>,
    ) {
        let dragged: Vec<String> = self.data.chat_project(chat).into_iter().collect();
        if !self.refuses_other_space((&dragged, tab), cx) {
            self.state.open_chat_at(chat, (tab, placement.clone()));
        }
    }

    pub(super) fn drop_view(
        &mut self,
        tab: TabId,
        (onto, placement): (TabId, &Placement),
        cx: &mut Context<'_, Self>,
    ) {
        let dragged = self.tab_projects_of(tab);
        if !self.refuses_other_space((&dragged, onto), cx) {
            self.state.drop_tab(tab, (onto, placement.clone()));
        }
    }

    fn refuses_other_space(
        &mut self,
        (dragged, onto): (&[String], TabId),
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let target = self.tab_projects_of(onto);
        if !crosses(dragged, &target) {
            return false;
        }
        let said = (ToastKind::Error, OTHER_SPACE.to_owned());
        self.show_notice(NoticeKey::Command, said, cx);
        true
    }
}

fn crosses(
    dragged: &[String],
    target: &[String],
) -> bool {
    let Some(home) = target.first() else {
        return false;
    };
    dragged.iter().any(|project| project != home)
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static [&'static str], &'static [&'static str], bool);

    fn owned(projects: &[&str]) -> Vec<String> {
        projects.iter().map(|p| (*p).to_owned()).collect()
    }

    #[test]
    fn only_a_chat_of_a_known_other_space_is_refused() {
        let cases: [Case; 5] = [
            (&["p2"], &["p1"], true),
            (&["p1"], &["p1"], false),
            (&[], &["p1"], false),
            (&["p2"], &[], false),
            (&["p1", "p2"], &["p1"], true),
        ];
        for (dragged, target, refused) in cases {
            assert_eq!(
                crosses(&owned(dragged), &owned(target)),
                refused,
                "{dragged:?} onto {target:?}"
            );
        }
    }
}
