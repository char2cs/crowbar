use std::path::PathBuf;
use std::sync::Arc;

use supervisor::Supervisor;

#[derive(Debug, Default)]
pub(crate) struct Launched {
    pub(crate) supervisor: Option<Arc<Supervisor>>,
    pub(crate) log: Option<PathBuf>,
}
