use gpui::{AnyElement, Context, Div, IntoElement, ParentElement, Styled, Window, div, px};
use sidebar::{Command, FileDialog, FileProperties};
use ui::{
    AlertButton, AlertRequest, AlertRole, Dialog, DialogSize, IconName, NativeAlert, Palette,
    Surface, Theme,
};

use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_MD, SPACE_SM, TEXT_LARGE};

const LABEL_WIDTH: f32 = 72.0;

fn property(
    label: &'static str,
    value: &str,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .gap(SPACE_MD)
        .child(
            div()
                .w(px(LABEL_WIDTH))
                .flex_none()
                .text_color(palette.muted_foreground())
                .child(label),
        )
        .child(
            div()
                .min_w_0()
                .text_color(palette.foreground())
                .child(value.to_owned()),
        )
}

fn properties_body(
    properties: &FileProperties,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(SPACE_SM)
        .text_size(TEXT_LARGE)
        .child(property("File", &properties.file_name, palette))
        .child(property("Path", &properties.path, palette))
        .child(property("Type", &properties.kind, palette))
}

fn delete_request(dialog: &FileDialog) -> AlertRequest {
    let go = AlertButton::new("Delete", AlertRole::Destructive);
    AlertRequest::confirm(
        dialog.title(),
        dialog.question().unwrap_or_default(),
        "Cancel",
        go,
    )
}

impl Sidebar {
    pub(crate) fn file_dialog_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let dialog = self.tree.files().dialog()?;
        let FileDialog::Properties(properties) = dialog else {
            return None;
        };
        let palette = Theme::of(cx).palette(Surface::Window);
        let sidebar = cx.entity();
        let frame = Dialog::new("file-dialog")
            .title(dialog.title())
            .size(DialogSize::Md)
            .icon(IconName::Info)
            .body(properties_body(properties, palette))
            .on_close(move |_, cx| sidebar.update(cx, Self::close_file_dialog));
        Some(frame.into_any_element())
    }

    pub(crate) fn ask_file_delete(
        dialog: FileDialog,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let request = delete_request(&dialog);
        let FileDialog::Delete { path, .. } = dialog else {
            return;
        };
        let command = Command::DeleteFile { path };
        let answer = NativeAlert::ask(request, window, cx);
        Self::send_when_accepted((answer, command), window, cx);
    }

    fn close_file_dialog(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.close_file_dialog();
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Intent;

    use crate::test_support::prelude::*;

    #[gpui::test]
    fn with_a_native_dialog_a_delete_is_asked_there_in_develop_s_words(cx: &mut TestAppContext) {
        let asked = crate::test_support::answer_alerts_with(cx, Some(1));
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        let ask = Intent::RequestFileDelete {
            path: "src/lib.rs".into(),
            is_dir: false,
        };
        sidebar.update_in(cx, |this, window, cx| this.apply(ask, window, cx));
        cx.run_until_parked();
        let question = FileDialog::Delete {
            path: "src/lib.rs".into(),
            is_dir: false,
        }
        .question()
        .unwrap_or_default();
        let request = asked.borrow().first().cloned();
        let words = request.map(|request| (request.title, request.message));
        assert_eq!(words, Some((Some("Delete File".into()), question.into())));
        assert!(sidebar.read_with(cx, |this, _| this.tree.files().dialog().is_none()));
        let want = Command::DeleteFile {
            path: "src/lib.rs".into(),
        };
        assert_eq!(recorder.sent(), vec![want]);
    }

    #[gpui::test]
    fn properties_open_and_close_without_asking_the_shell(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::ShowProperties("src/main.rs".into()), window, cx);
        });
        let shown = sidebar.read_with(cx, |this, _| this.tree.files().dialog().cloned());
        let want = FileDialog::Properties(FileProperties::of("src/main.rs"));
        assert_eq!(shown, Some(want));
        sidebar.update(cx, Sidebar::close_file_dialog);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.tree.files().dialog().cloned()),
            None
        );
    }

    fn ask_delete(floating: &mut crate::test_support::Floating) {
        let ask = Intent::RequestFileDelete {
            path: "src/lib.rs".into(),
            is_dir: false,
        };
        let sidebar = floating.sidebar.clone();
        floating.window.update(|window, cx| {
            sidebar.update(cx, |this, cx| this.apply(ask, window, cx));
        });
        floating.window.run_until_parked();
    }

    #[gpui::test]
    fn without_a_native_dialog_a_delete_is_cancelled_and_nothing_is_drawn(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        ask_delete(&mut floating);
        assert_eq!(floating.window.debug_bounds("ui-dialog"), None);
        assert!(floating.recorder.sent().is_empty(), "nothing is deleted");
        let open = floating.sidebar.read_with(&floating.window, |this, _| {
            this.tree.files().dialog().is_some()
        });
        assert!(!open, "no dialog is left pending");
    }
}
