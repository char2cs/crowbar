use std::sync::mpsc;

use crate::backend::native::linux::gtk_state::GtkState;
use crate::error::Result;

pub(crate) type Job = Box<dyn FnOnce(&mut GtkState) + Send>;

pub(crate) struct Channels {
    pub(crate) inbox: async_channel::Receiver<Job>,
    pub(crate) jobs: async_channel::Sender<Job>,
    pub(crate) ready: mpsc::Sender<Result<()>>,
}
