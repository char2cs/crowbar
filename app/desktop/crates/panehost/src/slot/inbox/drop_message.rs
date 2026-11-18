use std::path::PathBuf;
use std::sync::Arc;

use protocol::FsPath;
use serde_json::{Value, json};
use webview::{PageEvent, PageHandler};

use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;
use crate::slot::owner::Owner;

const DROP_KIND: &str = "shell.files-dropped";

pub(crate) fn drop_handler(
    owner: Owner,
    sink: async_channel::Sender<IpcMessage>,
) -> PageHandler {
    Arc::new(move |event| {
        on_page(&owner, &sink, event);
        true
    })
}

fn on_page(
    owner: &Owner,
    sink: &async_channel::Sender<IpcMessage>,
    event: PageEvent,
) {
    let PageEvent::FilesDropped(paths) = event else {
        return;
    };
    let Some(pane) = owner.current() else {
        return;
    };
    let text = drop_text(&paths);
    if sink.try_send(IpcMessage { pane, text }).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a file drop");
    }
}

fn drop_text(paths: &[PathBuf]) -> String {
    let names: Vec<String> = paths
        .iter()
        .map(|path| path.to_string_lossy().into_owned())
        .collect();
    json!({ "sdk": 1, "kind": DROP_KIND, "paths": names }).to_string()
}

pub(crate) fn drop_report(message: &IpcMessage) -> Option<HostEvent> {
    if !message.text.contains(DROP_KIND) {
        return None;
    }
    let value: Value = serde_json::from_str(&message.text).ok()?;
    if value.get("kind")?.as_str()? != DROP_KIND {
        return None;
    }
    let paths = value
        .get("paths")?
        .as_array()?
        .iter()
        .filter_map(|path| FsPath::new(path.as_str()?).ok())
        .collect();
    Some(HostEvent::FilesDropped {
        pane: message.pane.clone(),
        paths,
    })
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;

    fn message(text: String) -> IpcMessage {
        IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text,
        }
    }

    #[test]
    fn a_drop_travels_to_the_host_as_absolute_paths() {
        let text = drop_text(&[PathBuf::from("/tmp/a.rs"), PathBuf::from("rel/b.rs")]);
        let Some(HostEvent::FilesDropped { pane, paths }) = drop_report(&message(text)) else {
            panic!("no drop reported");
        };
        let names: Vec<&str> = paths.iter().map(FsPath::as_str).collect();
        assert_eq!((pane.as_str(), names), ("p1", vec!["/tmp/a.rs"]));
    }

    #[test]
    fn other_messages_are_not_drops() {
        let hover = r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#;
        assert_eq!(drop_report(&message(hover.into())), None);
    }
}
