//! One followed host's stream: the task reading it and the way to steer it.

use tokio::sync::mpsc;
use tokio::task::JoinHandle;

use crate::live::fleet::stream_control::StreamControl;

#[derive(Debug)]
pub(crate) struct Member {
    pub(crate) control: mpsc::UnboundedSender<StreamControl>,
    pub(crate) task: JoinHandle<()>,
}
