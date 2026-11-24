use gpui::{AppContext as _, Context, Window};
use protocol::{DocumentPath, PaneId};
use ui::{ToastContent, ToastKind, ToastLifetime};

use super::ShellView;
use crate::name_event::NameEvent;
use crate::name_prompt::{NamePrompt, SAVE_AS};
use crate::saved_file::SavedFile;
use crate::tab_looks::UNTITLED;

impl ShellView {
    pub(super) fn save_document(
        &mut self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.source.native().editors().untitled_chat(pane).is_some() {
            return self.ask_save_as(pane, window, cx);
        }
        self.source.native().editors().save(pane, cx);
    }

    fn ask_save_as(
        &mut self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.name_prompt.is_some() {
            return;
        }
        let prompt = cx.new(|cx| NamePrompt::new((UNTITLED, SAVE_AS), window, cx));
        self.subscriptions.push(
            cx.subscribe_in(&prompt, window, |this, _, event, window, cx| {
                this.on_save_as_prompt(event, window, cx);
            }),
        );
        self.name_prompt = Some((pane.clone(), prompt));
        cx.notify();
    }

    fn on_save_as_prompt(
        &mut self,
        event: &NameEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((pane, _)) = self.name_prompt.take() else {
            return;
        };
        self.source.native().focus(&pane, window, cx);
        cx.notify();
        let NameEvent::Confirmed(target) = event else {
            return;
        };
        if target.is_empty() {
            return;
        }
        self.save_untitled_as(&pane, target, cx);
    }

    fn save_untitled_as(
        &mut self,
        pane: &PaneId,
        target: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.source.native().editors().untitled_chat(pane) else {
            return;
        };
        let path = match DocumentPath::new(target) {
            Ok(path) => path,
            Err(error) => return self.refuse_save(pane, error.to_string(), cx),
        };
        let holders = self.source.native().editors().holders((&chat, &path));
        if holders
            .iter()
            .any(|holder| self.state.dirty().contains(holder))
        {
            let said = ToastContent::new(ToastKind::Error, held_by(&path));
            return self.show_keyed(&format!("save:{pane}"), (said, ToastLifetime::Auto), cx);
        }
        self.source.native().editors().save_as(pane, &path, cx);
    }

    pub(super) fn adopt_saved_as(
        &mut self,
        (pane, saved): (&PaneId, &SavedFile),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
        let Some(chat) = editors.untitled_chat(pane) else {
            return;
        };
        let target = (chat.as_str(), &saved.path);
        let holder = self.state.file_pane(target);
        let Some(shown) = self.state.replace_untitled(pane, target) else {
            return;
        };
        let Some(holder) = holder else {
            editors.rebind((pane, shown), saved, cx);
            return self.refresh(cx);
        };
        let took = editors.adopt((&holder, saved), window, cx);
        self.disk_taken(&holder, took, cx);
        self.refresh(cx);
    }
}

fn held_by(path: &DocumentPath) -> String {
    let name = path.as_str().rsplit('/').next().unwrap_or(path.as_str());
    format!("Save or discard changes in {name} first")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_refusal_names_the_open_file_by_its_name() {
        let cases = [
            (
                "src/pricing.ts",
                "Save or discard changes in pricing.ts first",
            ),
            ("notes.md", "Save or discard changes in notes.md first"),
        ];
        for (path, expected) in cases {
            assert_eq!(held_by(&DocumentPath::new(path).unwrap()), expected);
        }
    }
}
