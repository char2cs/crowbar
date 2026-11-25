use browser::PageUpdate;
use gpui::{ClipboardItem, Context};
use protocol::PaneId;

use super::pick_effect::PickEffect;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;

impl ShellView {
    pub(crate) fn toggle_pick(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let action = self.source.bars().toggle_pick(pane, cx);
        if let Err(error) = self.state.host().browse(pane, &action) {
            tracing::warn!(%error, "the browser pane could not start a pick");
            self.source.bars().finish_pick(pane, cx);
        }
    }

    pub(crate) fn finish_picks(
        &mut self,
        picks: Vec<(PaneId, PageUpdate)>,
        cx: &mut Context<'_, Self>,
    ) {
        for (pane, update) in picks {
            self.finish_pick(&pane, &update, cx);
        }
    }

    fn finish_pick(
        &mut self,
        pane: &PaneId,
        update: &PageUpdate,
        cx: &mut Context<'_, Self>,
    ) {
        let asked = self.source.bars().finish_pick(pane, cx).then_some(update);
        let Some(effect) = effect_of(asked, pane.as_str()) else {
            return;
        };
        self.carry_out(effect, cx);
    }

    fn carry_out(
        &mut self,
        effect: PickEffect,
        cx: &mut Context<'_, Self>,
    ) {
        let PickEffect { copy, notice } = effect;
        if let Some(text) = copy {
            cx.write_to_clipboard(ClipboardItem::new_string(text));
        }
        if let Some(notice) = notice {
            self.show_notice(NoticeKey::Command, notice, cx);
        }
    }
}

fn effect_of(
    asked: Option<&PageUpdate>,
    tab: &str,
) -> Option<PickEffect> {
    asked.map(|update| PickEffect::of(update, tab))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_result_nobody_asked_for_has_no_effect() {
        let update = PageUpdate::PickFailed {
            message: "x".into(),
        };
        assert_eq!(effect_of(None, "p1"), None);
        assert!(effect_of(Some(&update), "p1").is_some());
    }
}
