use crate::shell::chats::Mark;
use crate::shell::daemons::HostId;
use crate::shell::switcher::project_id::ProjectId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Project {
    pub id: ProjectId,
    pub name: String,
    pub mark: Mark,
    pub host: HostId,
}
