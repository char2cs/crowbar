//! Turning row interactions into view-state changes and commands.

use gpui::{Context, Pixels, Point, Window};
use sidebar::{
    Command, Error, FileDialog, FileProperties, Intent, MenuContext, ProjectId, Row, RowId,
    TerminalThreads,
};
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::panels::foot_menu;
use crate::panels::row_menu;
use crate::sidebar::Sidebar;
use crate::sidebar::row_action::RowAction;

impl Sidebar {
    pub(crate) fn act(
        &mut self,
        action: RowAction,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match action {
            RowAction::Open(row) => self.open_row(row, cx),
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
            Intent::Keep(id) => self.tree.keep(&id),
            Intent::ToggleFold(id) => {
                self.tree.toggle_fold(&id);
                Self::changed(cx);
            },
            Intent::StartCreateSpace => self.start_create_space(window, cx),
            Intent::StartFileRename(path) => self.start_file_rename(path, window, cx),
            Intent::StartFileCreate { dir, entry } => {
                self.start_file_create((dir, entry), window, cx);
            },
            Intent::RequestFileDelete { path, is_dir } => {
                self.tree
                    .show_file_dialog(FileDialog::Delete { path, is_dir });
            },
            Intent::ShowProperties(path) => {
                let properties = FileProperties::of(&path);
                self.tree
                    .show_file_dialog(FileDialog::Properties(properties));
            },
            Intent::StageFile { path, mode } => self.stage_file(path, mode),
            Intent::PasteFiles { dir } => self.paste_files(&dir, cx),
            Intent::SelectFile(path) => {
                self.files_focus.focus(window);
                self.tree.select_file(Some(path));
            },
            Intent::Ask(question) => self.question = Some(question),
            Intent::Send(command) => self.send_intent(command, cx),
        }
        cx.notify();
    }

    fn send_intent(
        &mut self,
        command: Command,
        cx: &mut Context<'_, Self>,
    ) {
        if matches!(command, Command::SelectProject(_)) {
            self.space = None;
        }
        self.send(command, cx);
    }

    pub(crate) fn send(
        &mut self,
        command: Command,
        cx: &mut Context<'_, Self>,
    ) {
        self.send_checked(command, cx);
    }

    pub(crate) fn send_checked(
        &mut self,
        command: Command,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        match self.sink.dispatch(command) {
            Ok(()) => {
                cx.notify();
                true
            },
            Err(Error::Refused { reason }) => {
                self.toast(
                    ToastContent::new(ToastKind::Error, reason),
                    (None, ToastLifetime::Auto),
                    cx,
                );
                false
            },
            Err(error) => {
                tracing::warn!(%error, "sidebar command failed");
                false
            },
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
            running: self
                .data
                .running_agents()
                .into_iter()
                .find(|agent| &agent.row == row)
                .map(|agent| agent.title),
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
            self.tree.request_project_removal(project, &name);
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
        self.tree.request_removal(&row, self.clock.now());
        self.schedule_removals(cx);
    }

    pub(crate) fn schedule_removals(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(deadline) = self.tree.removals().tray().next_deadline() else {
            return;
        };
        let wait = deadline.saturating_duration_since(self.clock.now());
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
        for row in self.tree.take_due_removals(self.clock.now()) {
            self.send_removal((&row, Command::CommitRemoval(row.clone())), cx);
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
    fn the_row_menu_opens_at_the_pointer(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
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
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let action = RowAction::Menu {
            row: RowId::from("gone"),
            at: point(px(0.0), px(0.0)),
        };
        sidebar.update_in(cx, |this, window, cx| this.act(action, window, cx));
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }

    #[gpui::test]
    fn toggling_a_fold_hides_the_children_and_is_remembered(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
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
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        let command = Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        };
        sidebar.update(cx, |this, cx| this.send(command.clone(), cx));
        assert_eq!(recorder.sent(), vec![command]);
    }
}
