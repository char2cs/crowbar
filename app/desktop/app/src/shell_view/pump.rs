use gpui::Context;
use gpui::Window;
use tab_bar::MemberView;

use super::ShellView;
use crate::chat_label::NEW_CHAT;

pub(super) fn member(
    titles: &crate::titles::Titles,
    pane: &protocol::PaneId,
) -> MemberView {
    let title = titles.get(pane);
    MemberView {
        pane: pane.clone(),
        label: title
            .as_ref()
            .map_or_else(|| NEW_CHAT.into(), |title| title.to_string().into()),
        provisional: title.is_none(),
        working: false,
    }
}

type Step<T> = fn(&mut ShellView, T, &mut Window, &mut Context<'_, ShellView>);

pub(super) async fn drain<T>(
    source: async_channel::Receiver<T>,
    this: gpui::WeakEntity<ShellView>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<T>,
) {
    while stepped(&source, &this, cx, step).await {}
}

async fn stepped<T>(
    source: &async_channel::Receiver<T>,
    this: &gpui::WeakEntity<ShellView>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<T>,
) -> bool {
    let Ok(item) = source.recv().await else {
        return false;
    };
    this.update_in(cx, |view, window, cx| step(view, item, window, cx))
        .is_ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::titles::Titles;

    #[test]
    fn a_member_is_idle_and_carries_its_pane_and_title() {
        let titles = Titles::default();
        let pane = protocol::PaneId::new("p7").unwrap();
        titles.set(pane.clone(), protocol::PaneTitle::new("Review").unwrap());
        let member = member(&titles, &pane);
        assert_eq!(member.pane, pane, "pane");
        assert_eq!(member.label.as_ref(), "Review", "title");
        assert!(!member.working, "idle");
    }
}
