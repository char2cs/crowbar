use gpui::prelude::FluentBuilder as _;
use gpui::{
    Context, Div, FontWeight, InteractiveElement as _, ParentElement, SharedString, Styled, div,
};
use ui::{Button, ButtonVariant, IconName, Palette, Surface, Theme};

use crate::folder_text::{NOT_ABSOLUTE, absolute};
use crate::import_event::ImportEvent;
use crate::look::{ADDING, BODY_X, BODY_Y, CANCEL, LABEL, LABEL_GAP, NOTE, ROW_GAP, SECTION_GAP};
use crate::view::View;

const REFUSED: &str = "folder-import-refused";

impl View {
    pub(super) fn body(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let path = self.path(cx);
        let refused = (!path.is_empty() && !absolute(&path)).then(|| {
            div()
                .debug_selector(|| REFUSED.into())
                .text_size(NOTE)
                .text_color(palette.destructive())
                .child(NOT_ABSOLUTE)
        });
        let browse = Button::icon_only("folder-import-browse", IconName::FolderOpen)
            .variant(ButtonVariant::Outline)
            .on_click(cx.listener(|_, _, _, cx| cx.emit(ImportEvent::Browse)));
        let folder_row = div()
            .flex()
            .gap(ROW_GAP)
            .child(div().flex_1().child(self.folder.clone()))
            .child(browse);
        let folder = section(label(self.kind.folder_label(), None, &palette))
            .child(folder_row)
            .children(refused);
        let name = section(label(
            self.kind.name_label(),
            self.kind.name_note(),
            &palette,
        ))
        .child(self.name.clone());
        div()
            .flex()
            .flex_col()
            .gap(SECTION_GAP)
            .px(BODY_X)
            .py(BODY_Y)
            .child(folder)
            .child(name)
    }

    pub(super) fn footer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let busy = self.busy(cx);
        let ready = absolute(&self.path(cx)) && !busy;
        let submit = Button::new("folder-import-add", self.submit_text(cx))
            .on_click(cx.listener(|this, _, _, cx| this.submit(cx)))
            .when(!ready, Button::disabled);
        let cancel = Button::new("folder-import-cancel", CANCEL)
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|_, _, _, cx| cx.emit(ImportEvent::Close)));
        div().flex().gap(ROW_GAP).child(cancel).child(submit)
    }

    fn submit_text(
        &self,
        cx: &Context<'_, Self>,
    ) -> &'static str {
        if self.busy(cx) {
            return ADDING;
        }
        self.kind.submit()
    }
}

fn section(label: Div) -> Div {
    div().flex().flex_col().gap(LABEL_GAP).child(label)
}

fn label(
    text: &'static str,
    note: Option<&'static str>,
    palette: &Palette,
) -> Div {
    let note = note.map(|note| {
        div()
            .font_weight(FontWeight::NORMAL)
            .text_color(palette.muted_foreground())
            .child(SharedString::from(note))
    });
    div()
        .flex()
        .gap_1()
        .text_size(LABEL)
        .font_weight(FontWeight::MEDIUM)
        .text_color(palette.foreground())
        .child(text)
        .children(note)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::kind::Kind;
    use crate::view::tests::open;

    #[gpui::test]
    fn a_relative_folder_shows_develop_s_absolute_path_line(cx: &mut TestAppContext) {
        let (view, _, cx) = open(cx, (Kind::Repo, ""));
        let refused = |path: &str, cx: &mut gpui::VisualTestContext| {
            view.update(cx, |view, cx| view.set_folder(path, cx));
            cx.run_until_parked();
            cx.debug_bounds(REFUSED).is_some()
        };
        assert!(refused("code/x", cx));
        assert!(!refused("/srv/app", cx));
        assert!(!refused("", cx));
    }
}
