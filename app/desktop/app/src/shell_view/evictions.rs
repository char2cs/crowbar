use gpui::Context;
use ui::ToastKind;

use super::ShellView;
use crate::notice_key::NoticeKey;

fn message_for(names: &[String]) -> Option<String> {
    match names {
        [] => None,
        [only] => Some(format!("Closed {only} to stay within the open tab limit.")),
        many => Some(format!(
            "Closed {} tabs to stay within the open tab limit.",
            many.len()
        )),
    }
}

impl ShellView {
    pub(super) fn announce_evictions(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = message_for(&self.state.take_evicted()) else {
            return;
        };
        self.show_notice(NoticeKey::Command, (ToastKind::Info, text), cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_toast_names_one_tab_and_counts_several() {
        let names = |list: &[&str]| list.iter().map(ToString::to_string).collect::<Vec<_>>();
        assert_eq!(message_for(&[]), None);
        assert_eq!(
            message_for(&names(&["a.rs"])).as_deref(),
            Some("Closed a.rs to stay within the open tab limit.")
        );
        assert_eq!(
            message_for(&names(&["a", "b"])).as_deref(),
            Some("Closed 2 tabs to stay within the open tab limit.")
        );
    }
}
