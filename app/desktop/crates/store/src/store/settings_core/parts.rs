//! What a settings core is built from.

use std::sync::Arc;

use tokio::runtime::Handle;

use crate::live::ChangeSignal;
use crate::live::Fleet;
use crate::live::HostDesk;
use crate::live::Hub;
use crate::live::OutcomeSink;
use crate::store::settings_core::json_file::JsonFile;
use crate::store::settings_core::system_fonts::SystemFonts;

pub(crate) struct Parts {
    pub(crate) file: JsonFile,
    pub(crate) hub: Arc<Hub>,
    pub(crate) handle: Handle,
    pub(crate) local: daemonapi::Client,
    pub(crate) signal: ChangeSignal,
    pub(crate) fonts: SystemFonts,
    pub(crate) outcome: OutcomeSink,
    pub(crate) fleet: Arc<Fleet>,
    pub(crate) desk: HostDesk,
}
