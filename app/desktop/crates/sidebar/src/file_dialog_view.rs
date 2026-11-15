//! The delete confirmation and Properties dialogs of the Files panel.

use gpui::{AnyElement, Context, Div, IntoElement, ParentElement, Styled, div, px};
use ui::{Button, ButtonVariant, Dialog, DialogSize, IconName, Palette, Surface, Theme};

use crate::command::Command;
use crate::file_dialog::FileDialog;
use crate::file_properties::FileProperties;
use crate::scale::{SPACE_MD, SPACE_SM, TEXT_LARGE};
use crate::sidebar::Sidebar;

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

impl Sidebar {
    pub(crate) fn file_dialog_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let dialog = self.tree.files.dialog()?.clone();
        let palette = Theme::of(cx).palette(Surface::Window);
        let sidebar = cx.entity();
        let frame = Dialog::new("file-dialog")
            .title(dialog.title())
            .size(DialogSize::Md)
            .on_close(move |_, cx| sidebar.update(cx, Self::close_file_dialog));
        let frame = match &dialog {
            FileDialog::Properties(properties) => frame
                .icon(IconName::Info)
                .body(properties_body(properties, palette)),
            FileDialog::Delete { .. } => {
                let question = dialog.question().unwrap_or_default();
                let cancel = Button::new("file-delete-cancel", "Cancel")
                    .variant(ButtonVariant::Ghost)
                    .on_click(cx.listener(|this, _, _, cx| this.close_file_dialog(cx)));
                let delete = Button::new("file-delete-confirm", "Delete")
                    .variant(ButtonVariant::Destructive)
                    .on_click(cx.listener(|this, _, _, cx| this.accept_file_delete(cx)));
                frame
                    .icon(IconName::Warning)
                    .body(
                        div()
                            .text_size(TEXT_LARGE)
                            .text_color(palette.foreground())
                            .child(question),
                    )
                    .footer(div().flex().gap(SPACE_MD).child(cancel).child(delete))
            },
        };
        Some(frame.into_any_element())
    }

    fn close_file_dialog(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.files.close();
        cx.notify();
    }

    fn accept_file_delete(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(FileDialog::Delete { path, .. }) = self.tree.files.close() {
            self.send(Command::DeleteFile { path }, cx);
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::intent::Intent;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn deleting_asks_first_and_sends_only_when_confirmed(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        let ask = Intent::RequestFileDelete {
            path: "src/lib.rs".into(),
            is_dir: false,
        };
        sidebar.update_in(cx, |this, window, cx| this.apply(ask, window, cx));
        assert!(recorder.sent().is_empty(), "nothing is deleted by asking");
        sidebar.update(cx, Sidebar::close_file_dialog);
        assert!(recorder.sent().is_empty(), "cancelling deletes nothing");
        let ask = Intent::RequestFileDelete {
            path: "src/lib.rs".into(),
            is_dir: false,
        };
        sidebar.update_in(cx, |this, window, cx| this.apply(ask, window, cx));
        sidebar.update(cx, Sidebar::accept_file_delete);
        let want = Command::DeleteFile {
            path: "src/lib.rs".into(),
        };
        assert_eq!(recorder.sent(), vec![want]);
        assert!(sidebar.read_with(cx, |this, _| this.tree.files.dialog().is_none()));
    }

    #[gpui::test]
    fn properties_open_and_close_without_asking_the_shell(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::ShowProperties("src/main.rs".into()), window, cx);
        });
        let shown = sidebar.read_with(cx, |this, _| this.tree.files.dialog().cloned());
        let want = FileDialog::Properties(FileProperties::of("src/main.rs"));
        assert_eq!(shown, Some(want));
        sidebar.update(cx, Sidebar::close_file_dialog);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.tree.files.dialog().cloned()),
            None
        );
    }
}
