use std::cell::RefCell;

use sidebar::{
    ActiveSpace, Changes, ChangesSource, Command, DemoShell, Error, FileAccess, FileRow,
    FilesSource, Focus, FolderPhase, FolderSource, Host, HostChoice, HostId, MergeBox, MoveTarget,
    PaneFocus, Project, ProjectId, ProjectTree, RowActions, RowId, RunningAgent, ScopeSync,
    SpaceList,
};

fn refusal(reason: &str) -> Error {
    Error::Refused {
        reason: reason.to_owned(),
    }
}

pub(crate) struct Recorder {
    data: DemoShell,
    sent: RefCell<Vec<Command>>,
    refused: RefCell<Option<(Command, &'static str)>>,
}

impl Recorder {
    pub(crate) fn new(data: DemoShell) -> Self {
        Self {
            data,
            sent: RefCell::new(Vec::new()),
            refused: RefCell::new(None),
        }
    }

    pub(crate) fn sent(&self) -> Vec<Command> {
        self.sent.borrow().clone()
    }

    pub(crate) fn refuse(
        &self,
        command: Command,
        reason: &'static str,
    ) {
        *self.refused.borrow_mut() = Some((command, reason));
    }
}

impl sidebar::Sink for Recorder {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
        self.sent.borrow_mut().push(command.clone());
        let refused = self.refused.borrow().clone();
        if let Some((_, reason)) = refused.filter(|(held, _)| held == &command) {
            return Err(refusal(reason));
        }
        self.data.dispatch(command)
    }
}

impl SpaceList for Recorder {
    fn sync(&self) -> ScopeSync {
        self.data.sync()
    }

    fn projects(&self) -> Vec<Project> {
        self.data.projects()
    }
}

impl HostChoice for Recorder {
    fn hosts(&self) -> Vec<Host> {
        self.data.hosts()
    }

    fn shown_host(&self) -> Option<HostId> {
        self.data.shown_host()
    }
}

impl ActiveSpace for Recorder {
    fn active_project(&self) -> Option<ProjectId> {
        self.data.active_project()
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        self.data.tree(project)
    }
}

impl sidebar::Requests for Recorder {
    fn icon_busy(
        &self,
        target: &sidebar::IconTarget,
    ) -> bool {
        self.data.icon_busy(target)
    }

    fn adding(
        &self,
        folder: &str,
    ) -> bool {
        self.data.adding(folder)
    }
}

impl PaneFocus for Recorder {
    fn focus(&self) -> Focus {
        self.data.focus()
    }

    fn running_agents(&self) -> Vec<RunningAgent> {
        self.data.running_agents()
    }

    fn can_start_terminal_thread(&self) -> bool {
        self.data.can_start_terminal_thread()
    }
}

impl RowActions for Recorder {
    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget> {
        self.data.move_targets(row)
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
}

impl FilesSource for Recorder {
    fn files(&self) -> Vec<FileRow> {
        self.data.files()
    }

    fn workspace_root(&self) -> Option<String> {
        self.data.workspace_root()
    }
}

impl ChangesSource for Recorder {
    fn changes(&self) -> Changes {
        self.data.changes()
    }

    fn merge_box(&self) -> Option<MergeBox> {
        self.data.merge_box()
    }
}

impl FileAccess for Recorder {}

impl FolderSource for Recorder {
    fn folder_phase(&self) -> FolderPhase {
        self.data.folder_phase()
    }
}
