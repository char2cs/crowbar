use gpui::Context;
use ui::ToastKind;

use super::ShellView;
use crate::notice_key::NoticeKey;

const CLEARED: &str = "Browsing data cleared";
const FAILED: &str = "Browsing data could not be cleared";

fn notice_for(cleared: &pane_host::Result<()>) -> (ToastKind, String) {
    match cleared {
        Ok(()) => (ToastKind::Success, CLEARED.to_owned()),
        Err(_) => (ToastKind::Error, FAILED.to_owned()),
    }
}

impl ShellView {
    pub(super) fn clear_browsing_data(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let cleared = self.state.host().clear_browsing_data();
        if let Err(error) = &cleared {
            tracing::warn!(%error, "clearing browsing data failed");
        }
        self.show_notice(NoticeKey::Command, notice_for(&cleared), cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_notice_says_whether_the_data_was_cleared() {
        let refused = Err(pane_host::Error::WebView(webview::Error::HostStopped));
        let cases = [
            (Ok(()), ToastKind::Success, CLEARED),
            (refused, ToastKind::Error, FAILED),
        ];
        for (result, kind, text) in cases {
            assert_eq!(notice_for(&result), (kind, text.to_owned()));
        }
    }
}
