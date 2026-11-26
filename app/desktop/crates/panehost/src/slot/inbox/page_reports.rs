//! What the shell hears of a page besides its frames: presses, hover, pointer moves and files the
//! operating system dropped on it. The web view takes the drop, so the shell hands the page the
//! paths and the point, and the page picks the element that takes them.

use std::sync::Arc;

use protocol::ShellFrame;
use serde_json::{Map, Value, json};
use webview::{PageEvent, PageHandler};

use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;
use crate::slot::owner::Owner;

const KIND: &str = "kind";
const FOCUS_KIND: &str = "shell.focus";
const HOVER_KIND: &str = "shell.hover";
const POINTER_KIND: &str = "shell.pointer";
const DROP_KIND: &str = "shell.files-dropped";

type Sink = async_channel::Sender<IpcMessage>;

pub(crate) fn press_handler(
    inner: Option<PageHandler>,
    owner: Owner,
    sink: Sink,
) -> PageHandler {
    Arc::new(move |event| match event {
        PageEvent::Pressed => report(&owner, &sink, (FOCUS_KIND, json!({}))),
        PageEvent::PointerMoved { x, y } => {
            report(&owner, &sink, (POINTER_KIND, json!({ "x": x, "y": y })))
        },
        event => inner.as_ref().is_none_or(|handler| handler(event)),
    })
}

pub(crate) fn drop_handler(
    inner: Option<PageHandler>,
    owner: Owner,
    sink: Sink,
) -> PageHandler {
    Arc::new(move |event| on_drop(inner.as_ref(), (&owner, &sink), event))
}

fn on_drop(
    inner: Option<&PageHandler>,
    (owner, sink): (&Owner, &Sink),
    event: PageEvent,
) -> bool {
    let PageEvent::FilesDropped { paths, x, y } = event else {
        return inner.is_none_or(|handler| handler(event));
    };
    let names: Vec<String> = paths
        .iter()
        .map(|path| path.to_string_lossy().into_owned())
        .collect();
    report(
        owner,
        sink,
        (DROP_KIND, json!({ "paths": names, "x": x, "y": y })),
    )
}

fn report(
    owner: &Owner,
    sink: &Sink,
    (kind, mut body): (&str, Value),
) -> bool {
    let Some(pane) = owner.current() else {
        return true;
    };
    if let Some(fields) = body.as_object_mut() {
        fields.insert("sdk".into(), json!(1));
        fields.insert(KIND.into(), json!(kind));
    }
    let text = body.to_string();
    if sink.try_send(IpcMessage { pane, text }).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a page report");
    }
    true
}

fn body_of(
    text: &str,
    kind: &str,
) -> Option<Map<String, Value>> {
    if !text.contains(kind) {
        return None;
    }
    let Value::Object(fields) = serde_json::from_str(text).ok()? else {
        return None;
    };
    (fields.get(KIND)?.as_str()? == kind).then_some(fields)
}

pub(crate) fn focus_report(message: &IpcMessage) -> Option<HostEvent> {
    let pane = message.pane.clone();
    body_of(&message.text, FOCUS_KIND).map(|_| HostEvent::PageFocused { pane })
}

pub(crate) fn hover_report(message: &IpcMessage) -> Option<HostEvent> {
    let hovered = body_of(&message.text, HOVER_KIND)?
        .get("hovered")?
        .as_bool()?;
    let pane = message.pane.clone();
    Some(HostEvent::HoverChanged { pane, hovered })
}

pub(crate) fn pointer_report(message: &IpcMessage) -> Option<HostEvent> {
    let (x, y) = point_of(&body_of(&message.text, POINTER_KIND)?)?;
    let pane = message.pane.clone();
    Some(HostEvent::PointerMoved { pane, x, y })
}

pub(crate) fn drop_frame(message: &IpcMessage) -> Option<ShellFrame> {
    let fields = body_of(&message.text, DROP_KIND)?;
    let paths = fields
        .get("paths")?
        .as_array()?
        .iter()
        .filter_map(|path| path.as_str().map(str::to_owned))
        .collect();
    let (x, y) = point_of(&fields)?;
    Some(ShellFrame::FilesDropped { paths, x, y })
}

fn point_of(fields: &Map<String, Value>) -> Option<(f64, f64)> {
    Some((fields.get("x")?.as_f64()?, fields.get("y")?.as_f64()?))
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use protocol::PaneId;

    use super::*;

    fn veto(event: &PageEvent) -> bool {
        *event != PageEvent::Title("veto".into())
    }

    fn heard_of(events: Vec<PageEvent>) -> (Vec<bool>, Vec<IpcMessage>) {
        let owner = Owner::default();
        owner.assign(PaneId::new("p1").unwrap());
        let (sink, inbox) = async_channel::unbounded();
        let inner = drop_handler(
            Some(Arc::new(|event| veto(&event))),
            owner.clone(),
            sink.clone(),
        );
        let handler = press_handler(Some(inner), owner, sink);
        let answers = events.into_iter().map(|event| handler(event)).collect();
        (
            answers,
            std::iter::from_fn(|| inbox.try_recv().ok()).collect(),
        )
    }

    #[test]
    fn page_events_are_reported_for_the_owner_and_every_other_event_passes_on() {
        let pane = PaneId::new("p1").unwrap();
        let dropped = PageEvent::FilesDropped {
            paths: vec![PathBuf::from("/tmp/a b.rs")],
            x: 1.0,
            y: 2.0,
        };
        let moved = PageEvent::PointerMoved { x: 2.0, y: 90.0 };
        let vetoed = PageEvent::Title("veto".into());
        let (answers, heard) = heard_of(vec![PageEvent::Pressed, moved, dropped, vetoed]);
        assert_eq!(
            answers,
            [true, true, true, false],
            "only the inner handler refuses"
        );
        let moved = HostEvent::PointerMoved {
            pane: pane.clone(),
            x: 2.0,
            y: 90.0,
        };
        let files = ShellFrame::FilesDropped {
            paths: vec!["/tmp/a b.rs".into()],
            x: 1.0,
            y: 2.0,
        };
        let reports = (
            focus_report(&heard[0]),
            pointer_report(&heard[1]),
            drop_frame(&heard[2]),
        );
        assert_eq!(
            reports,
            (
                Some(HostEvent::PageFocused { pane }),
                Some(moved),
                Some(files)
            )
        );
        assert_eq!(heard.len(), 3, "nothing else is reported");
    }

    #[test]
    fn a_report_reads_only_as_its_own_kind() {
        let on_p1 = |text: &str| IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text: text.into(),
        };
        let hover = on_p1(r#"{"sdk":1,"kind":"shell.hover","hovered":false}"#);
        let note = on_p1(r#"{"sdk":1,"kind":"page.note","text":"shell.focus shell.pointer"}"#);
        let partial = on_p1(r#"{"sdk":1,"kind":"shell.files-dropped","x":1,"y":2}"#);
        let pane = PaneId::new("p1").unwrap();
        let left = HostEvent::HoverChanged {
            pane,
            hovered: false,
        };
        assert_eq!(hover_report(&hover), Some(left), "a hover");
        assert_eq!(
            (focus_report(&hover), pointer_report(&note)),
            (None, None),
            "others"
        );
        assert_eq!(
            (drop_frame(&partial), hover_report(&on_p1("not json"))),
            (None, None)
        );
    }
}
