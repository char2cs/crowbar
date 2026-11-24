//! The icon popover of a space or a repository: preview, upload, emoji, GitHub and reset.

mod github;
mod preview;

use std::path::PathBuf;
use std::rc::Rc;

use gpui::TaskExt as _;
use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, AppContext as _, Context, Div, InteractiveElement as _, IntoElement, ParentElement,
    PathPromptOptions, Render, Styled, Subscription, Window, div, px,
};
use sidebar::{Mark, SpaceIcon};
use ui::{
    Button, ButtonSize, ButtonVariant, IconName, Surface, TextInput, TextInputEvent,
    TextInputStyle, Theme,
};

use crate::column::switcher::icon_label::section_label;
use crate::sidebar::scale::{SPACE_LG, SPACE_SM};

const WIDTH: f32 = 222.0;

pub(crate) type Pick = Rc<dyn Fn(SpaceIcon, &mut App)>;
pub(crate) type Act = Rc<dyn Fn(&mut App)>;

pub(crate) struct IconEditor {
    pub(crate) current: SpaceIcon,
    emoji: gpui::Entity<TextInput>,
    typing: bool,
    pick: Pick,
    fallback: Mark,
    github: Option<Act>,
    _typed: Subscription,
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
        let typed = cx.subscribe(&emoji, |this, _, event: &TextInputEvent, cx| match event {
            TextInputEvent::Submitted => this.set_emoji(cx),
            _ => cx.notify(),
        });
        Self {
            current: SpaceIcon::Default,
            emoji,
            typing: false,
            pick,
            fallback: Mark::Fallback,
            github: None,
            _typed: typed,
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
        let text = self.emoji.read(cx).text().trim().to_owned();
        let Some(icon) = SpaceIcon::emoji(&text) else {
            return;
        };
        self.emoji.update(cx, |field, cx| field.set_text("", cx));
        self.typing = false;
        self.choose(icon, cx);
    }
}

impl IconEditor {
    fn toggle_typing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.typing = !self.typing;
        if self.typing {
            let field = self.emoji.clone();
            field.update(cx, |field, cx| field.focus(window, cx));
        }
        cx.notify();
    }
}

fn first_image(paths: &[PathBuf]) -> Option<SpaceIcon> {
    paths.first().and_then(|path| SpaceIcon::image(path))
}

impl IconEditor {
    fn action_row(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let upload = Button::new("icon-upload", "Upload")
            .icon(IconName::UploadSimple)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Muted)
            .grow()
            .on_click(cx.listener(|_, _, _, cx| Self::upload(cx)));
        let emoji = Button::new("icon-emoji", "Emoji")
            .icon(IconName::Smiley)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Muted)
            .grow()
            .when(self.typing, |emoji| emoji.variant(ButtonVariant::Secondary))
            .on_click(cx.listener(|this, _, window, cx| this.toggle_typing(window, cx)));
        div()
            .flex()
            .gap(SPACE_SM)
            .debug_selector(|| "icon-actions".into())
            .child(upload)
            .child(emoji)
            .children(self.github_button(cx))
    }

    fn typing_row(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Div> {
        let set = Button::new("icon-emoji-set", "Set")
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.set_emoji(cx)));
        let set = set.when(
            self.emoji.read(cx).text().trim().is_empty(),
            Button::disabled,
        );
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
    ) -> Option<Div> {
        let reset = Button::new("icon-reset", "Reset to default")
            .icon(IconName::Trash)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Faint)
            .grow()
            .on_click(cx.listener(|this, _, _, cx| this.choose(SpaceIcon::Default, cx)));
        (self.current != SpaceIcon::Default).then(|| div().flex().child(reset))
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
            .gap(SPACE_LG)
            .p(SPACE_LG)
            .w(px(WIDTH))
            .debug_selector(|| "icon-editor".into())
            .child(section_label(palette))
            .child(self.preview(palette))
            .child(self.action_row(cx))
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
    #[gpui::test]
    fn the_emoji_button_opens_a_focused_field_that_enter_sets_and_closes(cx: &mut TestAppContext) {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&seen);
        let pick: Pick = Rc::new(move |icon, _| sink.borrow_mut().push(icon));
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (editor, cx) = cx.add_window_view(|_, cx| IconEditor::new(pick, cx));
        editor.update_in(cx, IconEditor::toggle_typing);
        let focused = editor.update_in(cx, |this, window, cx| {
            gpui::Focusable::focus_handle(this.emoji.read(cx), cx).is_focused(window)
        });
        assert!(
            focused,
            "the field takes the keyboard, as develop's autoFocus"
        );
        let field = editor.read_with(cx, |this, _| this.emoji.clone());
        field.update(cx, |field, cx| {
            field.set_text("🦀", cx);
            cx.emit(TextInputEvent::Submitted);
        });
        cx.run_until_parked();
        assert_eq!(*seen.borrow(), vec![SpaceIcon::Emoji("🦀".into())]);
        let after = editor.read_with(cx, |this, cx| {
            (this.typing, this.emoji.read(cx).text().to_owned())
        });
        assert_eq!(after, (false, String::new()), "the field closes empty");
    }

    #[gpui::test]
    fn the_editor_fills_develops_popover_with_a_centred_preview_over_buttons_sharing_the_row(
        cx: &mut TestAppContext
    ) {
        let pick: Pick = Rc::new(|_, _| {});
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (editor, cx) = cx.add_window_view(|_, cx| IconEditor::new(pick, cx));
        cx.run_until_parked();
        let root = cx.debug_bounds("icon-editor").unwrap_or_default();
        assert_eq!(
            f32::from(root.size.width),
            222.0,
            "w-64 less its border and the viewport's px-4"
        );
        let preview = cx.debug_bounds("icon-preview").unwrap_or_default();
        assert_eq!(
            preview.center().x,
            root.center().x,
            "the preview is centred"
        );
        let row = cx.debug_bounds("icon-actions").unwrap_or_default();
        let end = gpui::point(row.right() - px(4.0), row.center().y);
        cx.simulate_click(end, gpui::Modifiers::none());
        assert!(
            editor.read_with(cx, |this, _| this.typing),
            "the last flex-1 button, Emoji, reaches the row's end"
        );
    }
}
