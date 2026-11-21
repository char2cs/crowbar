use gpui::{Entity, Subscription, Task};

use crate::backend::connection_notice::ConnectionNotice;
use crate::backend::crash_panel::CrashPanel;
use crate::workspace::Workspace;

pub(crate) struct BackendWatch {
    pub(crate) connection: ConnectionNotice,
    pub(crate) warning: Option<Task<()>>,
    pub(crate) crash: Option<(Entity<CrashPanel>, Subscription)>,
    pub(crate) workspace: Entity<Workspace>,
}
