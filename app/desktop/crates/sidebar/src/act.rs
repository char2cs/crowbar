//! Turning row interactions into view-state changes and commands.

use std::time::Instant;

use gpui::{Context, Pixels, Point, Window};

use crate::command::Command;
use crate::foot_menu::foot_menu;
use crate::intent::Intent;
use crate::menu_context::MenuContext;
use crate::project_id::ProjectId;
use crate::row::Row;
use crate::row_action::RowAction;
use crate::row_id::RowId;
use crate::row_menu::row_menu;
use crate::sidebar::Sidebar;

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
            Intent::Keep(id) => self.tree.keep(&id),
            Intent::ToggleFold(id) => {
                self.tree.toggle_fold(&id);
                Self::changed(cx);
            },
            Intent::StartCreateSpace => self.creating_space = true,
            Intent::Send(command) => self.send(command, cx),
        }
        cx.notify();
    }

    pub(crate) fn send(&mut self, command: Command, cx: &mut Context<'_, Self>) {
        if self.sink.dispatch(command).is_ok() {
            cx.notify();
        }
    }

    pub(crate) fn row(&self, id: &RowId) -> Option<Row> {
        let project = self.data.active_project()?;
        self.data
            .tree(&project)
            .rows
            .into_iter()
            .find(|row| &row.id == id)
    }

    fn menu_context(&self, row: &RowId) -> MenuContext {
        MenuContext {
            targets: self.data.move_targets(row),
            terminal: self.data.can_start_terminal_thread().into(),
        }
    }

    fn request_project_removal(&mut self, project: ProjectId) {
        let name = self
            .data
            .projects()
            .into_iter()
            .find(|candidate| candidate.id == project)
            .map(|found| found.name);
        if let Some(name) = name {
            self.tree.request_project_removal(project, &name);
        }
    }

    fn request_removal(&mut self, id: &RowId, cx: &mut Context<'_, Self>) {
        let Some(row) = self.row(id) else {
            return;
        };
        self.tree.request_removal(&row, Instant::now());
        self.schedule_removals(cx);
    }

    pub(crate) fn schedule_removals(&mut self, cx: &mut Context<'_, Self>) {
        let Some(deadline) = self.tree.tray().next_deadline() else {
            return;
        };
        let wait = deadline.saturating_duration_since(Instant::now());
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(wait).await;
            this.update(cx, Self::send_due_removals)
        })
        .detach_and_log_err(cx);
    }

    fn send_due_removals(&mut self, cx: &mut Context<'_, Self>) {
        for row in self.tree.take_due(Instant::now()) {
            self.send(Command::CommitRemoval(row), cx);
        }
        self.schedule_removals(cx);
        cx.notify();
    }
}

#[cfg(test)]
#[path = "act_tests.rs"]
mod tests;
