use std::path::PathBuf;
use std::sync::Arc;

use protocol::{PaneId, ShellFrame};
use serde_json::{Value, json};
use webview::{PageEvent, PageHandler};

use crate::slot::inbox::ipc_message::IpcMessage;
use crate::slot::owner::Owner;

const DROP_KIND: &str = "shell.files-dropped";

pub(crate) fn drop_handler(
    inner: Option<PageHandler>,
    owner: Owner,
    sink: async_channel::Sender<IpcMessage>,
) -> PageHandler {
    Arc::new(move |event| on_page(inner.as_ref(), &owner, &sink, event))
}

fn on_page(
    inner: Option<&PageHandler>,
    owner: &Owner,
    sink: &async_channel::Sender<IpcMessage>,
    event: PageEvent,
) -> bool {
    let PageEvent::FilesDropped { paths, x, y } = event else {
        return inner.is_none_or(|handler| handler(event));
    };
    let Some(pane) = owner.current() else {
        return true;
    };
    if sink.try_send(drop_message(pane, &paths, (x, y))).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a file drop");
    }
    true
}

fn drop_message(
    pane: PaneId,
    paths: &[PathBuf],
    (x, y): (f64, f64),
) -> IpcMessage {
    let names: Vec<String> = paths
        .iter()
        .map(|path| path.to_string_lossy().into_owned())
        .collect();
    let text = json!({ "sdk": 1, "kind": DROP_KIND, "paths": names, "x": x, "y": y }).to_string();
    IpcMessage { pane, text }
}

pub(crate) fn drop_frame(message: &IpcMessage) -> Option<ShellFrame> {
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
        .filter_map(|path| path.as_str().map(str::to_owned))
        .collect();
    Some(ShellFrame::FilesDropped {
        paths,
        x: value.get("x")?.as_f64()?,
        y: value.get("y")?.as_f64()?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{fixture, frames_sent, instance};

    fn dropped_on(pane: &str) -> IpcMessage {
        let paths = [PathBuf::from("/tmp/a b.rs"), PathBuf::from("/tmp/c.rs")];
        drop_message(PaneId::new(pane).unwrap(), &paths, (12.5, 40.0))
    }

    #[test]
    fn a_drop_on_a_page_is_handed_to_that_page_with_its_point() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        let events = f.ipc(dropped_on("p1"));
        let drops: Vec<ShellFrame> = frames_sent(&f.mock)
            .into_iter()
            .filter(|frame| matches!(frame, ShellFrame::FilesDropped { .. }))
            .collect();
        assert_eq!(
            (events, drops),
            (
                Vec::new(),
                vec![ShellFrame::FilesDropped {
                    paths: vec!["/tmp/a b.rs".into(), "/tmp/c.rs".into()],
                    x: 12.5,
                    y: 40.0,
                }]
            )
        );
    }

    #[test]
    fn the_handler_reports_drops_and_passes_every_other_event_on() {
        let owner = Owner::default();
        owner.assign(PaneId::new("p1").unwrap());
        let (sink, inbox) = async_channel::unbounded();
        let inner: PageHandler = Arc::new(|event| event != PageEvent::Title("veto".into()));
        let handler = drop_handler(Some(inner), owner, sink);
        let dropped = PageEvent::FilesDropped {
            paths: vec![PathBuf::from("/tmp/a.rs")],
            x: 1.0,
            y: 2.0,
        };
        let answers = (handler(dropped), handler(PageEvent::Title("veto".into())));
        let reported = inbox.try_recv().ok().as_ref().and_then(drop_frame);
        assert_eq!(
            (answers, reported),
            (
                (true, false),
                Some(ShellFrame::FilesDropped {
                    paths: vec!["/tmp/a.rs".into()],
                    x: 1.0,
                    y: 2.0,
                })
            )
        );
    }

    fn on_p1(text: &str) -> IpcMessage {
        IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text: text.into(),
        }
    }

    #[test]
    fn only_a_drop_message_reads_as_a_drop() {
        let drop = r#"{"sdk":1,"kind":"shell.files-dropped","paths":["/a"],"x":1,"y":2}"#;
        let cases = [
            (
                drop,
                Some(ShellFrame::FilesDropped {
                    paths: vec!["/a".into()],
                    x: 1.0,
                    y: 2.0,
                }),
            ),
            (r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#, None),
            (
                r#"{"sdk":1,"kind":"shell.files-dropped","x":1,"y":2}"#,
                None,
            ),
            (
                r#"{"sdk":1,"kind":"page.note","text":"shell.files-dropped"}"#,
                None,
            ),
        ];
        for (text, expected) in cases {
            assert_eq!(drop_frame(&on_p1(text)), expected, "{text}");
        }
    }
}
