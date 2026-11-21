//! Inline rename, make-a-branch and fork-name inputs.

mod field;
mod field_look;
mod naming;
mod outcome;
mod settle;

pub(crate) use field::Field;

use gpui::{AppContext as _, Context, Focusable as _, Subscription, Window};
use sidebar::{Command, Editing, Error, Expansion, FileKind, NewEntry, PathText, RowId};
use ui::{Surface, TextInput, TextInputEvent};

use crate::panels::edit::outcome::Outcome;
use crate::sidebar::Sidebar;

const BRANCH_PLACEHOLDER: &str = "branch-name";

fn creation(
    path: String,
    entry: NewEntry,
) -> Command {
    match entry {
        NewEntry::File => Command::CreateFile { path },
        NewEntry::Folder => Command::CreateDirectory { path },
    }
}

impl Sidebar {
    pub(crate) fn start_branching(
        &mut self,
        id: &RowId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.start_branching(id);
        self.open_field((id.clone(), "", BRANCH_PLACEHOLDER), window, cx);
    }

    pub(crate) fn start_draft(
        &mut self,
        id: &RowId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.start_draft(id);
        self.open_field((id.clone(), "", BRANCH_PLACEHOLDER), window, cx);
    }

    pub(crate) fn start_file_rename(
        &mut self,
        path: String,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let name = PathText::new(&path).base().to_owned();
        let owner = RowId::from(path.as_str());
        self.tree.start_file_rename(path);
        self.open_field((owner, &name, ""), window, cx);
    }

    pub(crate) fn start_file_create(
        &mut self,
        (dir, entry): (String, NewEntry),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let collapsed = FileKind::Directory(Expansion::Collapsed);
        if self
            .data
            .files()
            .iter()
            .any(|row| row.path == dir && row.kind == collapsed)
        {
            self.send(Command::ToggleDirectory(dir.clone()), cx);
        }
        let owner = RowId::from(dir.as_str());
        self.tree.start_file_create(dir, entry);
        let placeholder = entry.placeholder();
        self.open_field((owner, "", placeholder), window, cx);
    }

    fn open_field(
        &mut self,
        seed: (RowId, &str, &str),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (row, initial, placeholder) = seed;
        let look = field_look::of(self.tree.editing());
        let field = cx.new(|cx| {
            TextInput::new(placeholder.to_owned(), cx)
                .on(Surface::Chrome)
                .style(look)
        });
        field.update(cx, |field, cx| field.set_text(initial, cx));
        field.update(cx, |field, cx| field.focus(window, cx));
        let subscriptions = Self::watch_field(&field, window, cx);
        self.inline = Some(Field {
            row,
            input: field,
            error: None,
            _subscriptions: subscriptions,
        });
        cx.notify();
    }

    fn watch_field(
        field: &gpui::Entity<TextInput>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Vec<Subscription> {
        let handle = field.read(cx).focus_handle(cx);
        let events =
            cx.subscribe_in(
                field,
                window,
                |this, _, event: &TextInputEvent, _, cx| match event {
                    TextInputEvent::Submitted => this.finish_edit(Outcome::Confirm, cx),
                    TextInputEvent::Cancelled => this.finish_edit(Outcome::Cancel, cx),
                    _ => {},
                },
            );
        let blur = cx.on_blur(&handle, window, |this, _, cx| {
            let outcome = match this.tree.editing() {
                Editing::Branching(_) => Outcome::Cancel,
                _ => Outcome::Confirm,
            };
            this.finish_edit(outcome, cx);
        });
        vec![events, blur]
    }

    pub(crate) fn finish_edit(
        &mut self,
        outcome: Outcome,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(edit) = self.inline.as_ref() else {
            return;
        };
        let name = edit.input.read(cx).text().trim().to_owned();
        if outcome == Outcome::Cancel || name.is_empty() {
            self.close_edit(cx);
            return;
        }
        let command = self.edit_command(&edit.row, name);
        match self.sink.dispatch(command) {
            Ok(()) if matches!(self.tree.editing(), Editing::Branching(_)) => {},
            Ok(()) => self.close_edit(cx),
            Err(Error::Refused { reason } | Error::Failed { reason, .. }) => {
                self.refuse_edit(reason, cx);
            },
            Err(error) => self.refuse_edit(error.to_string(), cx),
        }
    }

    fn edit_command(
        &self,
        row: &RowId,
        name: String,
    ) -> Command {
        match self.tree.editing() {
            Editing::Branching(target) => Command::MakeBranch {
                row: target.clone(),
                name,
            },
            Editing::Drafting(parent) => Command::Fork {
                parent: parent.clone(),
                name,
            },
            Editing::RenamingFile(path) => Command::RenameFile {
                path: path.clone(),
                new_path: PathText::new(PathText::new(path).parent()).join(&name),
            },
            Editing::Creating { dir, entry } => creation(PathText::new(dir).join(&name), *entry),
            _ => Command::Rename {
                row: row.clone(),
                name,
            },
        }
    }

    fn close_edit(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.stop_editing();
        self.inline = None;
        cx.notify();
    }

    fn refuse_edit(
        &mut self,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(edit) = self.inline.as_mut() {
            edit.error = Some(reason);
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    fn type_name(
        sidebar: &gpui::Entity<Sidebar>,
        name: &str,
        cx: &mut gpui::VisualTestContext,
    ) {
        let field = sidebar.read_with(cx, |this, _| {
            this.inline.as_ref().map(|edit| edit.input.clone())
        });
        let field = field.expect("an input is open");
        field.update(cx, |field, cx| field.set_text(name, cx));
    }

    fn start(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        sidebar.update_in(cx, |this, window, cx| {
            this.start_rename(&RowId::from("chat"), window, cx);
        });
    }

    #[gpui::test]
    fn confirming_a_rename_sends_it_and_closes_the_input(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        start(&sidebar, cx);
        type_name(&sidebar, "Fresh name", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::Rename {
            row: RowId::from("chat"),
            name: "Fresh name".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
        assert!(sidebar.read_with(cx, |this, _| this.inline.is_none()));
    }

    #[gpui::test]
    fn a_refused_name_keeps_the_input_open_with_the_reason(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        start(&sidebar, cx);
        type_name(&sidebar, "main", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let error = sidebar.read_with(cx, |this, _| {
            this.inline.as_ref().and_then(|edit| edit.error.clone())
        });
        assert!(error.is_some_and(|reason| reason.contains("already exists")));
    }

    #[gpui::test]
    fn cancelling_sends_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        start(&sidebar, cx);
        type_name(&sidebar, "ignored", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Cancel, cx));
        assert_eq!(recorder.sent(), Vec::<Command>::new());
        assert!(sidebar.read_with(cx, |this, _| this.inline.is_none()));
    }

    #[gpui::test]
    fn branching_sends_make_branch(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.start_branching(&RowId::from("chat"), window, cx);
        });
        type_name(&sidebar, "feature", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::MakeBranch {
            row: RowId::from("chat"),
            name: "feature".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
    }

    #[gpui::test]
    fn drafting_sends_a_fork(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.start_draft(&RowId::from("main"), window, cx);
        });
        type_name(&sidebar, "spike", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::Fork {
            parent: RowId::from("main"),
            name: "spike".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
    }
    #[gpui::test]
    fn renaming_a_file_sends_the_new_path_beside_the_old_one(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.start_file_rename("src/lib.rs".into(), window, cx);
        });
        type_name(&sidebar, "core.rs", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::RenameFile {
            path: "src/lib.rs".into(),
            new_path: "src/core.rs".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
    }

    fn create_named(
        sidebar: &gpui::Entity<Sidebar>,
        (entry, name): (NewEntry, &str),
        cx: &mut gpui::VisualTestContext,
    ) {
        sidebar.update_in(cx, |this, window, cx| {
            this.start_file_create(("src".into(), entry), window, cx);
        });
        type_name(sidebar, name, cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
    }

    #[gpui::test]
    fn creating_in_a_directory_sends_a_file_or_a_folder_path(cx: &mut TestAppContext) {
        let cases = [
            (
                NewEntry::File,
                Command::CreateFile {
                    path: "src/new".into(),
                },
            ),
            (
                NewEntry::Folder,
                Command::CreateDirectory {
                    path: "src/new".into(),
                },
            ),
        ];
        for (entry, expected) in cases {
            let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
            create_named(&sidebar, (entry, "new"), cx);
            assert_eq!(recorder.sent(), vec![expected], "{entry:?}");
        }
    }

    #[gpui::test]
    fn creating_at_the_root_uses_the_bare_name_and_an_empty_name_creates_nothing(
        cx: &mut TestAppContext
    ) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.start_file_create((String::new(), NewEntry::File), window, cx);
        });
        type_name(&sidebar, "  ", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        assert_eq!(recorder.sent(), Vec::<Command>::new());
        sidebar.update_in(cx, |this, window, cx| {
            this.start_file_create((String::new(), NewEntry::File), window, cx);
        });
        type_name(&sidebar, "top.rs", cx);
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        assert_eq!(
            recorder.sent(),
            vec![Command::CreateFile {
                path: "top.rs".into()
            }]
        );
    }
}
