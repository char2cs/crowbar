//! The create-space panel that replaces the panels while a space is being made.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, AppContext as _, Context, Div, FontWeight, IntoElement, ParentElement, Styled,
    Window, div, px,
};
use ui::{
    Button, ButtonVariant, Icon, IconName, Palette, Popover, Select, Surface, TextInput,
    TextInputEvent, Theme, radius,
};

use crate::icon_editor::IconEditor;
use crate::mode::Mode;
use crate::scale::{ICON_LARGE, SPACE_LG, SPACE_MD, SPACE_XL, SPACE_XS, TEXT_LARGE, TEXT_SMALL};
use crate::sidebar::Sidebar;
use crate::space_draft::{SpaceDraft, create_command};
use crate::space_facts::SpaceFacts;
use crate::space_icon::SpaceIcon;

const TITLE_SIZE: f32 = 16.0;
const TILE: f32 = 36.0;
const HEAD_GAP: f32 = 10.0;
const ROW_PADDING: f32 = 10.0;
const SELECT_WIDTH: f32 = 150.0;
const TITLE: &str = "Create a Space";
const SUBTITLE: &str = "Spaces are used to organize your tabs and sessions.";

fn field_row(
    icon: IconName,
    label: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .items_center()
        .justify_between()
        .gap(SPACE_MD)
        .px(SPACE_LG)
        .py(px(ROW_PADDING))
        .rounded(radius::LG)
        .border_1()
        .border_color(palette.border())
        .bg(palette.muted())
        .text_size(TEXT_LARGE)
        .child(
            div()
                .flex()
                .items_center()
                .gap(SPACE_MD)
                .child(
                    Icon::new(icon)
                        .size(ICON_LARGE)
                        .color(palette.muted_foreground()),
                )
                .child(label),
        )
}

impl Sidebar {
    pub(crate) fn start_create_space(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let name = cx.new(|cx| TextInput::new("Space Name", cx).on(Surface::Chrome));
        let folder = cx.new(|cx| {
            TextInput::new("/absolute/path/to/folder", cx)
                .on(Surface::Chrome)
                .leading_icon(IconName::FolderOpen)
        });
        let sidebar = cx.weak_entity();
        let staged = Rc::new(move |icon: SpaceIcon, cx: &mut gpui::App| {
            sidebar
                .update(cx, |this, cx| this.stage_space_icon(icon, cx))
                .ok();
        });
        let editor = cx.new(|cx| IconEditor::new(staged, cx));
        let subscriptions = [&name, &folder]
            .into_iter()
            .map(|field| {
                cx.subscribe_in(
                    field,
                    window,
                    |this, _, event: &TextInputEvent, window, cx| match event {
                        TextInputEvent::Submitted => this.create_space(cx),
                        TextInputEvent::Cancelled => this.cancel_create_space(window, cx),
                        _ => cx.notify(),
                    },
                )
            })
            .collect();
        name.read(cx).focus(window);
        self.mode = Mode::Chats;
        self.space = Some(SpaceDraft {
            name,
            folder,
            host: 0,
            icon: SpaceIcon::Default,
            editor,
            _subscriptions: subscriptions,
        });
        cx.notify();
    }

    pub(crate) fn cancel_create_space(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.space = None;
        cx.notify();
    }

    fn stage_space_icon(
        &mut self,
        icon: SpaceIcon,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(draft) = self.space.as_mut() {
            draft.icon = icon;
        }
        cx.notify();
    }

    fn select_space_host(
        &mut self,
        index: usize,
        cx: &mut Context<'_, Self>,
    ) {
        let hosts = self.data.hosts();
        let changed = self.space.as_ref().is_some_and(|draft| {
            hosts.get(draft.host).map(|host| host.kind) != hosts.get(index).map(|host| host.kind)
        });
        let Some(draft) = self.space.as_mut() else {
            return;
        };
        draft.host = index;
        if changed {
            draft.folder.update(cx, |field, cx| field.set_text("", cx));
        }
        cx.notify();
    }

    pub(crate) fn create_space(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(draft) = self.space.as_ref() else {
            return;
        };
        let hosts = self.data.hosts();
        let Some(host) = hosts.get(draft.host) else {
            return;
        };
        let folder = draft.folder.read(cx).text().to_owned();
        if !SpaceFacts::of(host, &folder).can_create {
            return;
        }
        let name = draft.name.read(cx).text().to_owned();
        let command = create_command((&name, &folder), host.id.clone(), draft.icon.clone());
        if self.send_checked(command, cx) {
            self.space = None;
        }
        cx.notify();
    }

    pub(crate) fn create_space_panel(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let draft = self.space.as_ref()?;
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let hosts = self.data.hosts();
        let host = hosts.get(draft.host)?;
        let folder = draft.folder.read(cx).text().to_owned();
        let facts = SpaceFacts::of(host, &folder);
        let names = hosts.iter().map(|host| host.name.clone().into()).collect();
        let sidebar = cx.entity();
        let select = Select::new("space-host", names, Some(draft.host))
            .on(Surface::Chrome)
            .compact()
            .width(px(SELECT_WIDTH))
            .on_change(move |index, _, cx| {
                sidebar.update(cx, |this, cx| this.select_space_host(index, cx));
            });
        let head = space_head(draft, palette);
        let footer = space_footer(&facts, palette, cx);
        Some(
            div()
                .flex()
                .flex_col()
                .size_full()
                .gap(SPACE_XL)
                .px(SPACE_LG)
                .pt(SPACE_XL)
                .child(heading(palette))
                .child(head)
                .child(field_row(IconName::HardDrives, "Daemon", palette).child(select))
                .child(
                    field_row(IconName::FolderOpen, "Folder", palette)
                        .child(div().flex_1().child(draft.folder.clone())),
                )
                .child(div().flex_1())
                .child(footer)
                .into_any_element(),
        )
    }
}

fn heading(palette: &Palette) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(SPACE_XS)
        .child(
            div()
                .text_size(px(TITLE_SIZE))
                .font_weight(FontWeight::SEMIBOLD)
                .child(TITLE),
        )
        .child(
            div()
                .text_size(TEXT_LARGE)
                .text_color(palette.muted_foreground())
                .child(SUBTITLE),
        )
}

fn space_head(
    draft: &SpaceDraft,
    palette: &Palette,
) -> Div {
    let editor = draft.editor.clone();
    let tile = Popover::new(
        "create-space-icon",
        icon_tile(&draft.icon, palette),
        move |_, _| editor.clone(),
    );
    div()
        .flex()
        .items_center()
        .gap(px(HEAD_GAP))
        .child(tile)
        .child(div().flex_1().child(draft.name.clone()))
}

fn space_footer(
    facts: &SpaceFacts,
    palette: &Palette,
    cx: &mut Context<'_, Sidebar>,
) -> Div {
    let create = Button::new("space-create", "Create Space")
        .on_click(cx.listener(|this, _, _, cx| this.create_space(cx)))
        .when(!facts.can_create, Button::disabled);
    let cancel = Button::new("space-cancel", "Cancel")
        .variant(ButtonVariant::Ghost)
        .on_click(cx.listener(|this, _, window, cx| this.cancel_create_space(window, cx)));
    let hint = facts.hint.clone().map(|text| {
        div()
            .text_size(TEXT_SMALL)
            .text_color(palette.muted_foreground())
            .child(text)
    });
    div()
        .flex()
        .flex_col()
        .gap(SPACE_MD)
        .pb(SPACE_XL)
        .children(hint)
        .child(create)
        .child(cancel)
}

fn icon_tile(
    icon: &SpaceIcon,
    palette: &Palette,
) -> Div {
    let glyph = match icon {
        SpaceIcon::Emoji(text) => div().child(text.clone()),
        SpaceIcon::Image(_) | SpaceIcon::Default => div().child(
            Icon::new(IconName::Books)
                .size(ICON_LARGE)
                .color(palette.muted_foreground()),
        ),
    };
    div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .rounded(radius::LG)
        .border_1()
        .border_dashed()
        .border_color(palette.border())
        .child(glyph)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::command::Command;
    use crate::intent::Intent;
    use crate::test_support::prelude::*;

    fn type_into(
        field: &gpui::Entity<TextInput>,
        text: &str,
        cx: &mut gpui::VisualTestContext,
    ) {
        field.update(cx, |field, cx| field.set_text(text, cx));
    }

    #[gpui::test]
    fn creating_a_space_needs_a_folder_then_sends_and_closes(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::StartCreateSpace, window, cx);
        });
        sidebar.update(cx, Sidebar::create_space);
        assert!(recorder.sent().is_empty(), "no folder, nothing sent");
        let folder = sidebar.read_with(cx, |this, _| this.space.as_ref().map(|d| d.folder.clone()));
        type_into(&folder.expect("panel open"), "/w/app", cx);
        sidebar.update(cx, Sidebar::create_space);
        let want = Command::CreateSpace {
            name: "app".into(),
            folder: "/w/app".into(),
            host: crate::host_id::HostId::from("local"),
        };
        assert_eq!(recorder.sent(), vec![want]);
        assert!(sidebar.read_with(cx, |this, _| this.space.is_none()));
    }

    fn start(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::StartCreateSpace, window, cx);
        });
        sidebar.read_with(cx, |this, _| this.space.is_some())
    }

    #[gpui::test]
    fn cancelling_closes_the_panel(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        assert!(start(&sidebar, cx));
        sidebar.update_in(cx, Sidebar::cancel_create_space);
        assert!(sidebar.read_with(cx, |this, _| this.space.is_none()));
    }

    #[gpui::test]
    fn selecting_a_space_closes_the_panel(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        assert!(start(&sidebar, cx));
        let select = Command::SelectProject(ProjectId::from("website"));
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::Send(select), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.space.is_none()));
    }
}
