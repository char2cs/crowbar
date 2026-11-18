//! What a settings core is built from.

use std::sync::Arc;

use daemonapi::DaemonClient;
use tokio::runtime::Handle;

use crate::change_signal::ChangeSignal;
use crate::fleet::Fleet;
use crate::host_desk::HostDesk;
use crate::hub::Hub;
use crate::outcome::OutcomeSink;
use crate::setting_file::SettingFile;
use crate::system_fonts::SystemFonts;

pub(crate) struct SettingsWiring {
    pub(crate) file: SettingFile,
    pub(crate) hub: Arc<Hub>,
    pub(crate) handle: Handle,
    pub(crate) local: DaemonClient,
    pub(crate) signal: ChangeSignal,
    pub(crate) fonts: SystemFonts,
    pub(crate) outcome: OutcomeSink,
    pub(crate) fleet: Arc<Fleet>,
    pub(crate) desk: HostDesk,
}
