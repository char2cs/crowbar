//! The create-space panel that replaces the panels while a space is being made.

use std::path::PathBuf;
use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, AppContext as _, Context, Div, IntoElement, ParentElement, PathPromptOptions,
    Styled, Window, div, px,
};
use sidebar::{Command, HostKind, Mode, SpaceFacts, SpaceIcon};
use ui::{
    Button, ButtonVariant, IconName, Palette, Placement, Popover, Select, Surface, TextInput,
    TextInputEvent, Theme,
};

use crate::icon_editor::IconEditor;
use crate::scale::{SPACE_LG, SPACE_MD, SPACE_XL, TEXT_SMALL};
use crate::sidebar::Sidebar;
use crate::space_draft::{SpaceDraft, create_command};
use crate::space_panel_parts::{field_row, heading, icon_tile};

const HEAD_GAP: f32 = 10.0;
const SELECT_WIDTH: f32 = 150.0;
const CHOOSE_FOLDER: &str = "Choose folder";

impl Sidebar {
    pub(crate) fn start_create_space(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let name = cx.new(|cx| TextInput::new("Space Name", cx).on(Surface::Chrome));
        let sidebar = cx.weak_entity();
        let staged = Rc::new(move |icon: SpaceIcon, cx: &mut gpui::App| {
            sidebar
                .update(cx, |this, cx| this.stage_space_icon(icon, cx))
                .unwrap_or_else(|error| tracing::debug!(%error, "sidebar closed before staging"));
        });
        let editor = cx.new(|cx| IconEditor::new(staged, cx));
        let subscription = cx.subscribe_in(
            &name,
            window,
            |this, _, event: &TextInputEvent, window, cx| match event {
                TextInputEvent::Submitted => this.create_space(cx),
                TextInputEvent::Cancelled => this.cancel_create_space(window, cx),
                _ => cx.notify(),
            },
        );
        name.read(cx).focus(window);
        self.mode = Mode::Chats;
        self.space = Some(SpaceDraft {
            name,
            folder: String::new(),
            host: 0,
            icon: SpaceIcon::Default,
            editor,
            _subscriptions: vec![subscription],
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
            draft.folder.clear();
        }
        cx.notify();
    }

    /// Fills the create-space form's folder; the answer to [`Command::ChooseSpaceFolder`].
    pub fn set_space_folder(
        &mut self,
        folder: String,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(draft) = self.space.as_mut() {
            draft.folder = folder;
        }
        cx.notify();
    }

    fn choose_space_folder(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let hosts = self.data.hosts();
        let Some(host) = self.space.as_ref().and_then(|draft| hosts.get(draft.host)) else {
            return;
        };
        if host.kind == HostKind::Remote {
            self.send_checked(Command::ChooseSpaceFolder(host.id.clone()), cx);
            return;
        }
        let options = PathPromptOptions {
            files: false,
            directories: true,
            multiple: false,
            prompt: None,
        };
        let picked = cx.prompt_for_paths(options);
        cx.spawn(async move |this, cx| {
            let paths = picked.await.ok().and_then(Result::ok).flatten();
            let folder = paths.and_then(|paths| paths.into_iter().next());
            this.update(cx, |this, cx| this.folder_picked(folder, cx))
        })
        .detach_and_log_err(cx);
    }

    fn folder_picked(
        &mut self,
        folder: Option<PathBuf>,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(folder) = folder {
            self.set_space_folder(folder.to_string_lossy().into_owned(), cx);
        }
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
        let folder = draft.folder.clone();
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
        let facts = SpaceFacts::of(host, &draft.folder);
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
                        .child(folder_button(&draft.folder, cx)),
                )
                .child(div().flex_1())
                .child(footer)
                .into_any_element(),
        )
    }
}

fn folder_button(
    folder: &str,
    cx: &mut Context<'_, Sidebar>,
) -> Button {
    let label = Some(folder)
        .filter(|folder| !folder.is_empty())
        .unwrap_or(CHOOSE_FOLDER);
    Button::new("space-folder", label.to_owned())
        .variant(ButtonVariant::Ghost)
        .on(Surface::Chrome)
        .on_click(cx.listener(|this, _, _, cx| this.choose_space_folder(cx)))
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
    )
    .placement(Placement::Right);
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

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Command, Intent};

    use crate::test_support::prelude::*;

    #[gpui::test]
    fn creating_a_space_needs_a_folder_then_sends_and_closes(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::StartCreateSpace, window, cx);
        });
        sidebar.update(cx, Sidebar::create_space);
        assert!(recorder.sent().is_empty(), "no folder, nothing sent");
        sidebar.update(cx, |this, cx| this.set_space_folder("/w/app".into(), cx));
        sidebar.update(cx, Sidebar::create_space);
        let want = Command::CreateSpace {
            name: "app".into(),
            folder: "/w/app".into(),
            host: sidebar::HostId::from("local"),
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

    #[gpui::test]
    fn a_remote_host_asks_the_shell_to_browse_its_disk_and_the_answer_fills_the_folder(
        cx: &mut TestAppContext
    ) {
        let (recorder, sidebar, cx) = open(cx, DemoData::rich());
        assert!(start(&sidebar, cx));
        let remote = sidebar.update(cx, |this, cx| {
            let remote = this.data.hosts()[1].id.clone();
            this.select_space_host(1, cx);
            this.choose_space_folder(cx);
            remote
        });
        assert_eq!(recorder.sent(), vec![Command::ChooseSpaceFolder(remote)]);
        sidebar.update(cx, |this, cx| this.set_space_folder("/srv/app".into(), cx));
        let folder = sidebar.read_with(cx, |this, _| this.space.as_ref().map(|d| d.folder.clone()));
        assert_eq!(folder.as_deref(), Some("/srv/app"));
    }
}
