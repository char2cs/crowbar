use std::rc::Rc;

use gpui::{App, AppContext as _, Context, Entity, Window};
use settings_ui::{FakeBackend, ModalConfig, SettingsModal, Snapshot, Values, default_schema};

use crate::error::{Error, Result};
use crate::shell_request::ShellRequest;

/// Opens the settings modal. Values live in memory only: the daemon has no settings store the
/// shell can read yet.
pub(crate) fn open_settings<V: 'static>(
    requests: async_channel::Sender<ShellRequest>,
    window: &mut Window,
    cx: &mut Context<'_, V>,
) -> Result<Entity<SettingsModal>> {
    let schema = default_schema().map_err(|e| Error::Settings(e.to_string()))?;
    let backend = Rc::new(FakeBackend::new(Snapshot::new(Values::default())));
    let config = ModalConfig::new(schema, Rc::clone(&backend) as _, backend).on_close(
        move |_: &mut Window, _: &mut App| {
            if requests.try_send(ShellRequest::CloseSettings).is_err() {
                tracing::warn!("the shell is closing; the settings modal stays open");
            }
        },
    );
    Ok(cx.new(|cx| SettingsModal::new(config, window, cx)))
}
