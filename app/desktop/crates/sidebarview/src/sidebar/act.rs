//! Turning row interactions into view-state changes and commands.

mod refusal;

use gpui::TaskExt as _;
use gpui::{Context, Pixels, Point, Window};
use sidebar::{
    Command, Deadline, FileDialog, FileProperties, Intent, MenuContext, ProjectId, Row, RowId,
};
use ui::ToastLifetime;

use crate::panels::foot_menu;
use crate::panels::row_menu;
use crate::sidebar::Sidebar;
use crate::sidebar::act::refusal::refusal_toast;
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
                self.pop_menu((foot_menu(&project), at), window, cx);
            },
            RowAction::Intent(intent) => self.apply(intent, window, cx),
            RowAction::Copied(path, note) => self.flash_copied((path, note), cx),
            RowAction::Edge(pull) => self.pull_edge(pull, cx),
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
        self.pop_menu((entries, at), window, cx);
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
            Intent::RequestRemoval(id) => self.request_removal(&id, window, cx),
            Intent::RequestProjectRemoval(project) => {
                self.request_project_removal(project, window, cx);
            },
            Intent::Keep(id) => self.tree.keep(&id),
            Intent::ToggleFold(id) => {
                self.tree.toggle_fold(&id);
                self.changed(cx);
            },
            Intent::StartCreateSpace => self.start_create_space(window, cx),
            Intent::StartFileRename(path) => self.start_file_rename(path, window, cx),
            Intent::StartFileCreate { dir, entry } => {
                self.start_file_create((dir, entry), window, cx);
            },
            Intent::RequestFileDelete { path, is_dir } => {
                Self::ask_file_delete(FileDialog::Delete { path, is_dir }, window, cx);
            },
            Intent::ShowProperties(path) => {
                let properties = FileProperties::of(&path);
                self.tree
                    .show_file_dialog(FileDialog::Properties(properties));
            },
            Intent::StageFile { path, mode } => self.stage_file(path, mode),
            Intent::PasteFiles { dir } => self.paste_files(&dir, cx),
            Intent::SelectFile(path) => {
                self.files_focus.focus(window, cx);
                self.tree.select_file(Some(path));
            },
            Intent::ToggleChangedFold(path) => self.tree.toggle_changed_fold(path),
            Intent::Ask(question) => Self::ask(question, window, cx),
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
        let Err(error) = self.sink.dispatch(command) else {
            cx.notify();
            return true;
        };
        if let Some(content) = refusal_toast(error) {
            self.toast(content, (None, ToastLifetime::Auto), cx);
        }
        false
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
        window: &mut Window,
        cx: &mut Context<'_, Self>,
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
        self.ask_confirmation(window, cx);
    }

    fn request_removal(
        &mut self,
        id: &RowId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(project) = self.data.active_project() else {
            return;
        };
        let tree = self.data.tree(&project);
        let Some(row) = tree.rows.iter().find(|row| &row.id == id) else {
            return;
        };
        let held = tree.subtree(id);
        self.tree.request_removal(row, self.clock.now());
        self.ask_confirmation(window, cx);
        self.schedule_removals(cx);
        let counting = self
            .tree
            .removals()
            .tray()
            .get(id)
            .is_some_and(|hold| matches!(hold.deadline, Deadline::At(_)));
        if counting {
            cx.emit(crate::Event::Held(held));
        }
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
    fn holding_a_row_names_it_and_every_row_beneath_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let held = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
        let seen = std::rc::Rc::clone(&held);
        let record =
            move |_, event: &crate::Event, _: &mut gpui::App| seen.borrow_mut().push(event.clone());
        let _subscription = cx.update(|_, cx| cx.subscribe(&sidebar, record));
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::RequestRemoval(RowId::from("folder")), window, cx);
        });
        let expected = vec![crate::Event::Held(vec![
            RowId::from("folder"),
            RowId::from("child"),
        ])];
        assert_eq!(*held.borrow(), expected);
    }

    #[gpui::test]
    fn a_repository_asking_to_be_deleted_closes_nothing(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let events = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
        let seen = std::rc::Rc::clone(&events);
        let record =
            move |_, event: &crate::Event, _: &mut gpui::App| seen.borrow_mut().push(event.clone());
        let _subscription = cx.update(|_, cx| cx.subscribe(&sidebar, record));
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::RequestRemoval(RowId::from("demo")), window, cx);
        });
        let asking = sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some());
        assert!(asking, "the dialog asks");
        assert_eq!(
            *events.borrow(),
            Vec::new(),
            "no view closes before the answer"
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
