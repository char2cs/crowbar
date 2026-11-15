use std::rc::Rc;

use gpui::{App, AppContext as _, Context, Entity, Window};
use settings_ui::{ModalConfig, SettingsModal, default_schema};
use shell_data::SettingsAdapter;

use crate::error::{Error, Result};
use crate::shell_request::ShellRequest;

fn request_close(requests: &async_channel::Sender<ShellRequest>) {
    if requests.try_send(ShellRequest::CloseSettings).is_err() {
        tracing::warn!("the shell is closing; the settings modal stays open");
    }
}

pub(crate) fn open_settings<V: 'static>(
    requests: async_channel::Sender<ShellRequest>,
    backend: &SettingsAdapter,
    window: &mut Window,
    cx: &mut Context<'_, V>,
) -> Result<Entity<SettingsModal>> {
    let schema = default_schema().map_err(|e| Error::Settings(e.to_string()))?;
    let backend = Rc::new(backend.clone());
    let config = ModalConfig::new(schema, Rc::clone(&backend) as _, backend)
        .on_close(move |_: &mut Window, _: &mut App| request_close(&requests));
    Ok(cx.new(|cx| SettingsModal::new(config, window, cx)))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn a_live_shell_is_asked_exactly_once() -> bool {
        let (requests, queue) = async_channel::unbounded();
        request_close(&requests);
        queue.try_recv() == Ok(ShellRequest::CloseSettings) && queue.try_recv().is_err()
    }

    fn a_gone_shell_is_left_alone() -> bool {
        let (requests, queue) = async_channel::unbounded();
        drop(queue);
        request_close(&requests);
        requests.is_closed()
    }

    type Case = (&'static str, fn() -> bool);

    const CASES: &[Case] = &[
        ("the shell is listening", a_live_shell_is_asked_exactly_once),
        ("the shell is already gone", a_gone_shell_is_left_alone),
    ];

    #[test]
    fn closing_the_modal_asks_a_live_shell_once_and_a_gone_one_nothing() {
        let wrong: Vec<&str> = CASES
            .iter()
            .filter(|(_, scenario)| !scenario())
            .map(|(name, _)| *name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }

    #[test]
    fn the_default_schema_builds() {
        assert!(default_schema().is_ok(), "the modal has a schema to show");
    }
}
