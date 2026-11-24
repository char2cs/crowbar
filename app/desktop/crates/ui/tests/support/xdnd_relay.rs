//! `xdnd-relay` (gpui-pre-linux): a drag source talks only to the top-level window under the
//! pointer, so a drag over an XDND-aware child window, as over a web view the shell embeds,
//! reaches the child's window under the pointer through the window, at the pointer's position in
//! the child (where GTK looks for it), and that window's answers reach the source, even with the
//! source's drag icon under the pointer.

use std::sync::mpsc;
use std::time::Duration;

use gpui::{App, AppContext as _, Bounds, Window, WindowBounds, WindowOptions, point, px, size};
use x11rb::connection::Connection as _;
use x11rb::protocol::Event;
use x11rb::protocol::xproto::{
    AtomEnum, ClientMessageData, ClientMessageEvent, ConnectionExt as _, CreateWindowAux,
    EventMask, PropMode, WindowClass,
};
use x11rb::rust_connection::RustConnection;
use x11rb::wrapper::ConnectionExt as _;
use x11rb::{COPY_DEPTH_FROM_PARENT, COPY_FROM_PARENT, CURRENT_TIME, NONE};

use super::{IconProbe, xcb_window};

type Failure = Box<dyn std::error::Error + Send + Sync>;
type Outcome = Result<Heard, String>;
type Received = (ClientMessageEvent, String);

const SIDE: u16 = 200;
const INSIDE: i16 = 50;
const CHILD_AT: i16 = 20;
const CHILD_SIDE: u16 = 100;
const ICON_SIDE: u16 = 16;
const WAIT: Duration = Duration::from_secs(20);
const XDND_VERSION: u32 = 5;
const MORE_TYPES: u32 = 1;
const ACCEPTED: u32 = 1;
const WAKE: &str = "CROWBAR_TEST_WAKE";
/// Four types, one more than an `XdndEnter` carries, so the child reads the list from the window.
const TYPES: [&str; 4] = ["text/uri-list", "text/plain", "UTF8_STRING", "STRING"];
const SEQUENCE: [&str; 5] = [
    "XdndEnter",
    "XdndPosition",
    "XdndStatus",
    "XdndDrop",
    "XdndFinished",
];

#[derive(Debug, PartialEq, Eq)]
struct Message {
    at: &'static str,
    kind: String,
    from: &'static str,
}

#[derive(Debug, PartialEq, Eq)]
struct Heard {
    messages: Vec<Message>,
    point: Option<(u16, u16)>,
    window_types: Vec<String>,
}

#[derive(Clone, Copy)]
struct Windows {
    window: u32,
    child: u32,
    inner: u32,
    source: u32,
    root: u32,
    types: [u32; 4],
}

impl Windows {
    fn heard(
        self,
        at: u32,
        kind: &str,
        from: u32,
    ) -> Message {
        Message {
            at: self.role(at),
            kind: kind.to_owned(),
            from: self.role(from),
        }
    }

    fn role(
        self,
        id: u32,
    ) -> &'static str {
        match id {
            id if id == self.window => "window",
            id if id == self.child => "child",
            id if id == self.inner => "inner window",
            id if id == self.source => "source",
            _ => "another window",
        }
    }
}

#[test]
#[ignore = "needs display"]
fn a_drag_over_an_xdnd_aware_child_reaches_it_and_its_answers_reach_the_source() {
    let (sender, heard) = mpsc::channel();
    gpui_platform::application().run(move |cx| open(sender, cx));
    let heard = heard
        .try_recv()
        .expect("the probe window ran")
        .expect("the drag went through");
    let at = |at, kind: &str, from| Message {
        at,
        kind: kind.to_owned(),
        from,
    };
    let [enter, position, status, drop, finished] = SEQUENCE;
    assert_eq!(
        heard,
        Heard {
            messages: vec![
                at("inner window", enter, "window"),
                at("inner window", position, "window"),
                at("source", status, "window"),
                at("inner window", drop, "window"),
                at("source", finished, "window"),
            ],
            point: Some((within_child(), within_child())),
            window_types: TYPES.map(str::to_owned).to_vec(),
        }
    );
}

fn within_child() -> u16 {
    (INSIDE - CHILD_AT).cast_unsigned()
}

fn open(
    sender: mpsc::Sender<Outcome>,
    cx: &mut App,
) {
    let bounds = Bounds::new(
        point(px(0.0), px(0.0)),
        size(px(f32::from(SIDE)), px(f32::from(SIDE))),
    );
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let failed = sender.clone();
    let opened = cx.open_window(options, move |window, cx| {
        window.on_next_frame(move |window, cx| drag(sender, window, cx));
        cx.new(|_| IconProbe { colour: 0 })
    });
    if let Err(error) = opened {
        failed.send(Err(error.to_string())).unwrap_or_default();
        cx.quit();
    }
}

fn drag(
    sender: mpsc::Sender<Outcome>,
    window: &Window,
    cx: &mut App,
) {
    let started = xcb_window(window)
        .ok_or_else(|| "the window has no XCB handle".into())
        .and_then(windows_over);
    let (connection, windows) = match started {
        Ok(started) => started,
        Err(error) => {
            sender.send(Err(error.to_string())).unwrap_or_default();
            return cx.quit();
        },
    };
    let conversation = cx
        .background_executor()
        .spawn(async move { converse(&connection, windows).map_err(|error| error.to_string()) });
    cx.spawn(async move |cx| {
        sender.send(conversation.await).unwrap_or_default();
        cx.update(|cx| cx.quit());
    })
    .detach();
    cx.spawn(async move |cx| {
        cx.background_executor().timer(WAIT).await;
        wake(windows.child).unwrap_or_default();
    })
    .detach();
}

fn windows_over(window: u32) -> Result<(RustConnection, Windows), Failure> {
    let (connection, screen) = x11rb::connect(None)?;
    let root = connection
        .setup()
        .roots
        .get(screen)
        .ok_or("the display has no such screen")?
        .root;
    let child = aware_child_of(&connection, window)?;
    let inner = inner_window_of(&connection, child)?;
    let types = TYPES
        .iter()
        .map(|name| atom(&connection, name))
        .collect::<Result<Vec<_>, _>>()?;
    let source = source_offering(&connection, root, &types)?;
    drag_icon_over(&connection, (window, root))?;
    connection.warp_pointer(NONE, window, 0, 0, 0, 0, INSIDE, INSIDE)?;
    connection.get_input_focus()?.reply()?;
    let windows = Windows {
        window,
        child,
        inner,
        source,
        root,
        types: <[u32; 4]>::try_from(types).map_err(|_| "four types")?,
    };
    Ok((connection, windows))
}

fn aware_child_of(
    connection: &RustConnection,
    window: u32,
) -> Result<u32, Failure> {
    let child = connection.generate_id()?;
    connection.create_window(
        COPY_DEPTH_FROM_PARENT,
        child,
        window,
        CHILD_AT,
        CHILD_AT,
        CHILD_SIDE,
        CHILD_SIDE,
        0,
        WindowClass::INPUT_OUTPUT,
        COPY_FROM_PARENT,
        &CreateWindowAux::new(),
    )?;
    let aware = atom(connection, "XdndAware")?;
    connection.change_property32(
        PropMode::REPLACE,
        child,
        aware,
        AtomEnum::ATOM,
        &[XDND_VERSION],
    )?;
    connection.map_window(child)?;
    Ok(child)
}

fn inner_window_of(
    connection: &RustConnection,
    child: u32,
) -> Result<u32, Failure> {
    let inner = connection.generate_id()?;
    connection.create_window(
        COPY_DEPTH_FROM_PARENT,
        inner,
        child,
        0,
        0,
        CHILD_SIDE,
        CHILD_SIDE,
        0,
        WindowClass::INPUT_OUTPUT,
        COPY_FROM_PARENT,
        &CreateWindowAux::new(),
    )?;
    connection.map_window(inner)?;
    Ok(inner)
}

fn drag_icon_over(
    connection: &RustConnection,
    (window, root): (u32, u32),
) -> Result<(), Failure> {
    let at = connection
        .translate_coordinates(window, root, INSIDE, INSIDE)?
        .reply()?;
    let half = (ICON_SIDE / 2).cast_signed();
    let icon = connection.generate_id()?;
    connection.create_window(
        COPY_DEPTH_FROM_PARENT,
        icon,
        root,
        at.dst_x - half,
        at.dst_y - half,
        ICON_SIDE,
        ICON_SIDE,
        0,
        WindowClass::INPUT_OUTPUT,
        COPY_FROM_PARENT,
        &CreateWindowAux::new().override_redirect(1),
    )?;
    connection.map_window(icon)?;
    Ok(())
}

fn source_offering(
    connection: &RustConnection,
    root: u32,
    types: &[u32],
) -> Result<u32, Failure> {
    let source = connection.generate_id()?;
    connection.create_window(
        0,
        source,
        root,
        0,
        0,
        1,
        1,
        0,
        WindowClass::INPUT_ONLY,
        COPY_FROM_PARENT,
        &CreateWindowAux::new(),
    )?;
    let type_list = atom(connection, "XdndTypeList")?;
    connection.change_property32(PropMode::REPLACE, source, type_list, AtomEnum::ATOM, types)?;
    Ok(source)
}

fn converse(
    connection: &RustConnection,
    windows: Windows,
) -> Result<Heard, Failure> {
    let copy = atom(connection, "XdndActionCopy")?;
    enter_and_move(connection, windows, copy)?;
    let mut messages = Vec::new();
    let mut point = None;
    while let Some((event, kind)) = next_heard(connection, messages.len())? {
        let [from, _, at, ..] = event.data.as_data32();
        point = position_point(&kind, at).or(point);
        messages.push(windows.heard(event.window, &kind, from));
        answer(connection, windows, (&kind, copy))?;
    }
    Ok(Heard {
        messages,
        point,
        window_types: window_types(connection, windows.window)?,
    })
}

fn next_heard(
    connection: &RustConnection,
    heard: usize,
) -> Result<Option<Received>, Failure> {
    if heard >= SEQUENCE.len() {
        return Ok(None);
    }
    let event = client_message(connection)?;
    let kind = atom_name(connection, event.type_)?;
    Ok((kind != WAKE).then_some((event, kind)))
}

fn client_message(connection: &RustConnection) -> Result<ClientMessageEvent, Failure> {
    match connection.wait_for_event()? {
        Event::ClientMessage(event) => Ok(event),
        _ => client_message(connection),
    }
}

fn position_point(
    kind: &str,
    at: u32,
) -> Option<(u16, u16)> {
    (kind == "XdndPosition").then_some(((at >> 16) as u16, (at & 0xffff) as u16))
}

fn enter_and_move(
    connection: &RustConnection,
    windows: Windows,
    copy: u32,
) -> Result<(), Failure> {
    let point = connection
        .translate_coordinates(windows.window, windows.root, INSIDE, INSIDE)?
        .reply()?;
    let root_point =
        (u32::from(point.dst_x.cast_unsigned()) << 16) | u32::from(point.dst_y.cast_unsigned());
    let [first, second, third, _] = windows.types;
    let enter = [
        windows.source,
        (XDND_VERSION << 24) | MORE_TYPES,
        first,
        second,
        third,
    ];
    send(connection, windows.window, "XdndEnter", enter)?;
    let position = [windows.source, 0, root_point, CURRENT_TIME, copy];
    send(connection, windows.window, "XdndPosition", position)
}

fn answer(
    connection: &RustConnection,
    windows: Windows,
    (heard, copy): (&str, u32),
) -> Result<(), Failure> {
    let reply = match heard {
        "XdndPosition" => ("XdndStatus", [windows.inner, ACCEPTED, 0, 0, copy]),
        "XdndStatus" => ("XdndDrop", [windows.source, 0, CURRENT_TIME, 0, 0]),
        "XdndDrop" => ("XdndFinished", [windows.inner, ACCEPTED, copy, 0, 0]),
        _ => return Ok(()),
    };
    send(connection, windows.window, reply.0, reply.1)
}

fn window_types(
    connection: &RustConnection,
    window: u32,
) -> Result<Vec<String>, Failure> {
    let type_list = atom(connection, "XdndTypeList")?;
    let reply = connection
        .get_property(false, window, type_list, AtomEnum::ATOM, 0, 16)?
        .reply()?;
    reply
        .value32()
        .into_iter()
        .flatten()
        .map(|atom| atom_name(connection, atom))
        .collect()
}

fn wake(child: u32) -> Result<(), Failure> {
    let (connection, _) = x11rb::connect(None)?;
    send(&connection, child, WAKE, [0; 5])?;
    Ok(())
}

fn send(
    connection: &RustConnection,
    to: u32,
    kind: &str,
    data: [u32; 5],
) -> Result<(), Failure> {
    let message = ClientMessageEvent {
        response_type: x11rb::protocol::xproto::CLIENT_MESSAGE_EVENT,
        format: 32,
        sequence: 0,
        window: to,
        type_: atom(connection, kind)?,
        data: ClientMessageData::from(data),
    };
    connection.send_event(false, to, EventMask::NO_EVENT, message)?;
    connection.flush()?;
    Ok(())
}

fn atom(
    connection: &RustConnection,
    name: &str,
) -> Result<u32, Failure> {
    Ok(connection
        .intern_atom(false, name.as_bytes())?
        .reply()?
        .atom)
}

fn atom_name(
    connection: &RustConnection,
    atom: u32,
) -> Result<String, Failure> {
    let reply = connection.get_atom_name(atom)?.reply()?;
    Ok(String::from_utf8_lossy(&reply.name).into_owned())
}
