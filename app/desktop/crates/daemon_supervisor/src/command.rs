use tokio::sync::oneshot;

use crate::error::Error;

pub(crate) enum Command {
    Restart(oneshot::Sender<Result<(), Error>>),
    Stop(oneshot::Sender<()>),
}
