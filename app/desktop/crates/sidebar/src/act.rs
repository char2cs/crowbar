//! Turning row interactions into view-state changes and commands.

use std::time::Instant;

use gpui::{Context, Pixels, Point, Window};
use ui::ToastKind;

use crate::command::Command;
use crate::error::Error;
use crate::file_dialog::FileDialog;
use crate::file_properties::FileProperties;
use crate::foot_menu::foot_menu;
use crate::intent::Intent;
use crate::menu_context::MenuContext;
use crate::project_id::ProjectId;
use crate::row::Row;
use crate::row_action::RowAction;
use crate::row_id::RowId;
use crate::row_menu::row_menu;
use crate::sidebar::Sidebar;
use crate::terminal_threads::TerminalThreads;

impl Sidebar {
    pub(crate) fn act(
        &mut self,
        action: RowAction,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match action {
            RowAction::Open(row) => self.send(Command::OpenRow(row), cx),
            RowAction::Rename(row) => self.apply(Intent::StartRename(row), window, cx),
            RowAction::Menu { row, at } | RowAction::RepoMenu { row, at } => {
                self.open_row_menu((&row, at), window, cx);
            },
            RowAction::FootMenu { project, at } => {
                self.open_menu((foot_menu(&project), at), window, cx);
            },
            RowAction::Intent(intent) => self.apply(intent, window, cx),
            RowAction::Edge(pull) => self.drag.pull = pull,
            RowAction::Press(press) => self.press_row(press),
            RowAction::DragOver(point) => self.drag_over(&point, cx),
            RowAction::Dropped(subject, target) => self.drop_on((subject, target), cx),
        }
    }

    fn open_row_menu(
        &mut self,
        (id, at): (&RowId, Point<Pixels>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(row) = self.row(id) else {
            return;
        };
        let entries = row_menu(&row, &self.menu_context(id));
        self.open_menu((entries, at), window, cx);
    }

    pub(crate) fn apply(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match intent {
            Intent::StartRename(id) => self.start_rename(&id, window, cx),
            Intent::StartBranching(id) => self.start_branching(&id, window, cx),
            Intent::StartDraft(id) => self.start_draft(&id, window, cx),
            Intent::RequestRemoval(id) => self.request_removal(&id, cx),
            Intent::RequestProjectRemoval(project) => self.request_project_removal(project),
            Intent::Keep(id) => self.tree.removals.keep(&id),
            Intent::ToggleFold(id) => {
                self.tree.toggle_fold(&id);
                Self::changed(cx);
            },
            Intent::StartCreateSpace => self.creating_space = true,
            Intent::StartFileRename(path) => self.start_file_rename(path, window, cx),
            Intent::StartFileCreate { dir, entry } => {
                self.start_file_create((dir, entry), window, cx);
            },
            Intent::RequestFileDelete { path, is_dir } => {
                self.tree.files.show(FileDialog::Delete { path, is_dir });
            },
            Intent::ShowProperties(path) => {
                let properties = FileProperties::of(&path);
                self.tree.files.show(FileDialog::Properties(properties));
            },
            Intent::StageFile { path, mode } => self.stage_file(path, mode),
            Intent::PasteFiles { dir } => self.paste_files(&dir, cx),
            Intent::SelectFile(path) => {
                self.files_focus.focus(window);
                self.tree.files.selected = Some(path);
            },
            Intent::Send(command) => self.send(command, cx),
        }
        cx.notify();
    }

    pub(crate) fn send(
        &mut self,
        command: Command,
        cx: &mut Context<'_, Self>,
    ) {
        match self.sink.dispatch(command) {
            Ok(()) => cx.notify(),
            Err(Error::Refused { reason }) => self.toast((ToastKind::Error, reason), None, cx),
            Err(_) => {},
        }
    }

    pub(crate) fn row(
        &self,
        id: &RowId,
    ) -> Option<Row> {
        let project = self.data.active_project()?;
        self.data
            .tree(&project)
            .rows
            .into_iter()
            .find(|row| &row.id == id)
    }

    fn menu_context(
        &self,
        row: &RowId,
    ) -> MenuContext {
        MenuContext {
            targets: self.data.move_targets(row),
            terminal: TerminalThreads::of(self.data.as_ref()),
        }
    }

    fn request_project_removal(
        &mut self,
        project: ProjectId,
    ) {
        let name = self
            .data
            .projects()
            .into_iter()
            .find(|candidate| candidate.id == project)
            .map(|found| found.name);
        if let Some(name) = name {
            self.tree.removals.request_project(project, &name);
        }
    }

    fn request_removal(
        &mut self,
        id: &RowId,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(row) = self.row(id) else {
            return;
        };
        self.tree.request_removal(&row, Instant::now());
        self.schedule_removals(cx);
    }

    pub(crate) fn schedule_removals(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(deadline) = self.tree.removals.tray().next_deadline() else {
            return;
        };
        let wait = deadline.saturating_duration_since(Instant::now());
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(wait).await;
            this.update(cx, Self::send_due_removals)
        })
        .detach_and_log_err(cx);
    }

    fn send_due_removals(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        for row in self.tree.removals.take_due(Instant::now()) {
            self.send(Command::CommitRemoval(row), cx);
        }
        self.schedule_removals(cx);
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn opening_a_row_sends_the_open_command(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.act(RowAction::Open(RowId::from("chat")), window, cx);
        });
        assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("chat"))]);
    }

    #[gpui::test]
    fn the_row_menu_opens_at_the_pointer(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let at = point(px(40.0), px(200.0));
        let action = RowAction::Menu {
            row: RowId::from("chat"),
            at,
        };
        sidebar.update_in(cx, |this, window, cx| this.act(action, window, cx));
        let menu_at = sidebar.read_with(cx, |this, _| this.menu.as_ref().map(|menu| menu.at));
        assert_eq!(menu_at, Some(at));
    }

    #[gpui::test]
    fn a_menu_for_a_missing_row_stays_closed(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let action = RowAction::Menu {
            row: RowId::from("gone"),
            at: point(px(0.0), px(0.0)),
        };
        sidebar.update_in(cx, |this, window, cx| this.act(action, window, cx));
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }

    #[gpui::test]
    fn toggling_a_fold_hides_the_children_and_is_remembered(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::ToggleFold(RowId::from("demo")), window, cx);
        });
        let folded = sidebar.read_with(cx, |this, _| this.tree.folded().count());
        assert_eq!(folded, 1);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.preferences().folded.len()),
            1
        );
    }

    #[gpui::test]
    fn a_refused_command_leaves_the_sidebar_unchanged(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        let command = Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        };
        sidebar.update(cx, |this, cx| this.send(command.clone(), cx));
        assert_eq!(recorder.sent(), vec![command]);
    }
}
