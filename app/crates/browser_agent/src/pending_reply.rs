use browser_nav::AgentAnswer;
use tokio::sync::oneshot;

use crate::reply_scope::ReplyScope;

#[derive(Debug)]
pub(crate) struct PendingReply {
    pub(crate) scope: ReplyScope,
    pub(crate) sender: oneshot::Sender<AgentAnswer>,
}
