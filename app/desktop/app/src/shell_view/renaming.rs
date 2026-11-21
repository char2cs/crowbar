use gpui::{AppContext as _, Context, Window};
use protocol::PaneId;

use super::ShellView;
use crate::name_event::NameEvent;
use crate::name_prompt::{NamePrompt, RENAME};

impl ShellView {
    pub(super) fn ask_rename(
        &mut self,
        (pane, current): (&PaneId, &str),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.name_prompt.is_some() {
            return;
        }
        let prompt = cx.new(|cx| NamePrompt::new((current, RENAME), window, cx));
        self.subscriptions.push(
            cx.subscribe_in(&prompt, window, |this, _, event, window, cx| {
                this.on_rename_prompt(event, window, cx);
            }),
        );
        self.name_prompt = Some((pane.clone(), prompt));
        cx.notify();
    }

    fn on_rename_prompt(
        &mut self,
        event: &NameEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let asked = self.name_prompt.take().map(|(pane, _)| pane);
        window.focus(&self.focus, cx);
        cx.notify();
        let Some((pane, name)) = confirmed(asked.as_ref(), event) else {
            return;
        };
        self.state.rename_pane(pane, name);
        self.refresh(cx);
    }
}

fn confirmed<'a>(
    asked: Option<&'a PaneId>,
    event: &'a NameEvent,
) -> Option<(&'a PaneId, &'a str)> {
    let NameEvent::Confirmed(name) = event else {
        return None;
    };
    asked.map(|pane| (pane, name.as_str()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_confirmed_name_for_a_pane_that_was_asked_about_renames() {
        let pane = PaneId::new("t1").unwrap();
        let named = NameEvent::Confirmed("logs".into());
        assert_eq!(
            confirmed(Some(&pane), &named),
            Some((&pane, "logs")),
            "named"
        );
        assert_eq!(confirmed(None, &named), None, "nobody asked");
        assert_eq!(
            confirmed(Some(&pane), &NameEvent::Dismissed),
            None,
            "dismissed"
        );
    }
}
