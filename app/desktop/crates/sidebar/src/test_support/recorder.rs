//! A data source and command sink that remembers what it was sent.

use std::cell::RefCell;

use crate::changes::Changes;
use crate::command::Command;
use crate::command_sink::CommandSink;
use crate::demo_data::DemoData;
use crate::error::Error;
use crate::file_row::FileRow;
use crate::focus::Focus;
use crate::host::Host;
use crate::merge_box::MergeBox;
use crate::move_target::MoveTarget;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row_id::RowId;
use crate::running_agent::RunningAgent;
use crate::scope_sync::ScopeSync;
use crate::sidebar_data::SidebarData;

pub(crate) struct Recorder {
    data: DemoData,
    sent: RefCell<Vec<Command>>,
}

impl Recorder {
    pub(crate) fn new(data: DemoData) -> Self {
        Self {
            data,
            sent: RefCell::new(Vec::new()),
        }
    }

    pub(crate) fn sent(&self) -> Vec<Command> {
        self.sent.borrow().clone()
    }
}

impl CommandSink for Recorder {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
        self.sent.borrow_mut().push(command.clone());
        self.data.dispatch(command)
    }
}

impl SidebarData for Recorder {
    fn sync(&self) -> ScopeSync {
        self.data.sync()
    }

    fn hosts(&self) -> Vec<Host> {
        self.data.hosts()
    }

    fn projects(&self) -> Vec<Project> {
        self.data.projects()
    }

    fn active_project(&self) -> Option<ProjectId> {
        self.data.active_project()
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        self.data.tree(project)
    }

    fn focus(&self) -> Focus {
        self.data.focus()
    }

    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget> {
        self.data.move_targets(row)
    }

    fn can_start_terminal_thread(&self) -> bool {
        self.data.can_start_terminal_thread()
    }

    fn serves(
        &self,
        command: &Command,
    ) -> bool {
        self.data.serves(command)
    }

    fn can_remove_folder(
        &self,
        path: &str,
    ) -> bool {
        self.data.can_remove_folder(path)
    }

    fn workspace_root(&self) -> Option<String> {
        self.data.workspace_root()
    }

    fn files(&self) -> Vec<FileRow> {
        self.data.files()
    }

    fn changes(&self) -> Changes {
        self.data.changes()
    }

    fn merge_box(&self) -> Option<MergeBox> {
        self.data.merge_box()
    }

    fn running_agents(&self) -> Vec<RunningAgent> {
        self.data.running_agents()
    }
}
