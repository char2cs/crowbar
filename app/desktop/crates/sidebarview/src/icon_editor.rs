//! The icon popover of a space: preview, upload, emoji and reset.

use std::path::PathBuf;
use std::rc::Rc;

use gpui::StyledImage as _;
use gpui::{
    App, AppContext as _, Context, Div, FontWeight, IntoElement, ParentElement, PathPromptOptions,
    Render, Styled, Window, div, img, px,
};
use sidebar::SpaceIcon;
use ui::{
    Button, ButtonSize, ButtonVariant, Icon, IconName, Surface, TextInput, TextInputStyle, Theme,
    radius,
};

use crate::scale::{ICON_LARGE, SPACE_MD, SPACE_SM, TEXT_CAPTION};

const PREVIEW: f32 = 56.0;
const EMOJI_PREVIEW: f32 = 24.0;
const WIDTH: f32 = 224.0;
const LABEL: &str = "ICON";

pub(crate) type Pick = Rc<dyn Fn(SpaceIcon, &mut App)>;

pub(crate) struct IconEditor {
    pub(crate) current: SpaceIcon,
    emoji: gpui::Entity<TextInput>,
    typing: bool,
    pick: Pick,
}

impl IconEditor {
    pub(crate) fn new(
        pick: Pick,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let emoji = cx.new(|cx| {
            TextInput::new("Type an emoji…", cx)
                .on(Surface::Window)
                .style(TextInputStyle::Compact)
        });
        Self {
            current: SpaceIcon::Default,
            emoji,
            typing: false,
            pick,
        }
    }

    fn choose(
        &mut self,
        icon: SpaceIcon,
        cx: &mut Context<'_, Self>,
    ) {
        self.current = icon.clone();
        (self.pick)(icon, cx);
        cx.notify();
    }

    fn upload(cx: &mut Context<'_, Self>) {
        let options = PathPromptOptions {
            files: true,
            directories: false,
            multiple: false,
            prompt: None,
        };
        let picked = cx.prompt_for_paths(options);
        cx.spawn(async move |this, cx| {
            let paths = picked.await.ok().and_then(Result::ok).flatten();
            let icon = paths.and_then(|paths| first_image(&paths));
            this.update(cx, |this, cx| this.chosen(icon, cx))
        })
        .detach_and_log_err(cx);
    }

    fn chosen(
        &mut self,
        icon: Option<SpaceIcon>,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(icon) = icon {
            self.choose(icon, cx);
        }
    }

    fn set_emoji(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let text = self.emoji.read(cx).text().to_owned();
        if let Some(icon) = SpaceIcon::emoji(&text) {
            self.choose(icon, cx);
        }
    }
}

fn first_image(paths: &[PathBuf]) -> Option<SpaceIcon> {
    paths.first().and_then(|path| SpaceIcon::image(path))
}

fn preview(
    icon: &SpaceIcon,
    palette: &ui::Palette,
) -> Div {
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(PREVIEW))
        .rounded(radius::XL)
        .bg(palette.muted());
    match icon {
        SpaceIcon::Emoji(text) => frame.text_size(px(EMOJI_PREVIEW)).child(text.clone()),
        SpaceIcon::Image(path) => frame.child(
            img(PathBuf::from(path))
                .size(px(PREVIEW))
                .rounded(radius::XL)
                .object_fit(gpui::ObjectFit::Cover),
        ),
        SpaceIcon::Default => frame.child(
            Icon::new(IconName::Books)
                .size(ICON_LARGE)
                .color(palette.foreground()),
        ),
    }
}

impl IconEditor {
    fn action_row(cx: &mut Context<'_, Self>) -> Div {
        let upload = Button::new("icon-upload", "Upload")
            .icon(IconName::UploadSimple)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Outline)
            .on_click(cx.listener(|_, _, _, cx| Self::upload(cx)));
        let emoji = Button::new("icon-emoji", "Emoji")
            .icon(IconName::Smiley)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Outline)
            .on_click(cx.listener(|this, _, _, cx| {
                this.typing = !this.typing;
                cx.notify();
            }));
        div()
            .flex()
            .flex_wrap()
            .gap(SPACE_SM)
            .child(upload)
            .child(emoji)
    }

    fn typing_row(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Div> {
        let set = Button::new("icon-emoji-set", "Set")
            .size(ButtonSize::Xs)
            .on_click(cx.listener(|this, _, _, cx| this.set_emoji(cx)));
        self.typing.then(|| {
            div()
                .flex()
                .gap(SPACE_SM)
                .child(self.emoji.clone())
                .child(set)
        })
    }

    fn reset_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Button> {
        let reset = Button::new("icon-reset", "Reset to default")
            .icon(IconName::Trash)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|this, _, _, cx| this.choose(SpaceIcon::Default, cx)));
        (self.current != SpaceIcon::Default).then_some(reset)
    }
}

impl Render for IconEditor {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .flex()
            .flex_col()
            .gap(SPACE_MD)
            .w(px(WIDTH))
            .child(
                div()
                    .text_size(TEXT_CAPTION)
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(palette.muted_foreground())
                    .child(LABEL),
            )
            .child(preview(&self.current, palette))
            .child(Self::action_row(cx))
            .children(self.typing_row(cx))
            .children(self.reset_button(cx))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::TestAppContext;

    use super::*;

    fn editor(
        cx: &mut TestAppContext,
        seen: &Rc<RefCell<Vec<SpaceIcon>>>,
    ) -> gpui::Entity<IconEditor> {
        let sink = Rc::clone(seen);
        let pick: Pick = Rc::new(move |icon, _| sink.borrow_mut().push(icon));
        cx.update(|cx| cx.new(|cx| IconEditor::new(pick, cx)))
    }

    #[gpui::test]
    fn setting_an_emoji_and_resetting_report_the_choice(cx: &mut TestAppContext) {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let editor = editor(cx, &seen);
        editor.update(cx, |this, cx| {
            this.emoji.update(cx, |field, cx| field.set_text("🚀", cx));
            this.set_emoji(cx);
            this.chosen(Some(SpaceIcon::Default), cx);
            this.chosen(None, cx);
            this.emoji
                .update(cx, |field, cx| field.set_text("too long!", cx));
            this.set_emoji(cx);
        });
        let want = vec![SpaceIcon::Emoji("🚀".into()), SpaceIcon::Default];
        assert_eq!(*seen.borrow(), want);
        assert_eq!(
            editor.read_with(cx, |this, _| this.current.clone()),
            SpaceIcon::Default
        );
    }
}
