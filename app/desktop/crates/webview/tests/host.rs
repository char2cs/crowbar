//! Native web views against a real X server and `WebKitGTK`: where their keys go and what a press
//! on them reports.
//!
//! GTK runs on one thread per process, so one test drives every case on one host.
#![cfg(target_os = "linux")]

use std::sync::{Arc, mpsc};
use std::time::Duration;

use webview::{
    Bounds, Config, Handle, Host, HostConfig, IpcHandler, Kind, PageHandler, ParentWindow,
    ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse, Visibility,
};
use x11rb::connection::Connection as _;
use x11rb::protocol::Event;
use x11rb::protocol::xproto::{
    BUTTON_PRESS_EVENT, BUTTON_RELEASE_EVENT, ConnectionExt as _, CreateWindowAux, EventMask,
    KEY_PRESS_EVENT, KEY_RELEASE_EVENT, Screen, WindowClass,
};
use x11rb::protocol::xtest::ConnectionExt as _;
use x11rb::rust_connection::RustConnection;
use x11rb::{COPY_DEPTH_FROM_PARENT, CURRENT_TIME, NONE};

type TestResult<T = ()> = Result<T, Box<dyn std::error::Error>>;

const PAGE: &str = "<script>
addEventListener('keydown', (event) => window.ipc.postMessage('key ' + event.key));
addEventListener('load', () => window.ipc.postMessage('ready'));
</script>";
const PAGE_URL: &str = "crowbar://test/page";
const PAGE_WAIT: Duration = Duration::from_secs(30);
const KEY_WAIT: Duration = Duration::from_secs(5);
const KEYCODE_A: u8 = 38;
const SIDE: u16 = 300;
const INSIDE: i16 = 50;
const LEFT_BUTTON: u8 = 1;

struct Page(&'static str);

impl ProtocolHandler for Page {
    fn handle(
        &self,
        _request: ProtocolRequest,
        respond: ProtocolResponder,
    ) {
        respond(ProtocolResponse {
            status: 200,
            headers: vec![("content-type".into(), "text/html".into())],
            body: self.0.as_bytes().to_vec(),
        });
    }
}

fn forward_to(sender: mpsc::Sender<String>) -> IpcHandler {
    Arc::new(move |text| sender.send(text).unwrap_or_else(drop))
}

fn report_to(sender: mpsc::Sender<String>) -> PageHandler {
    Arc::new(move |event| {
        sender.send(format!("{event:?}")).unwrap_or_else(drop);
        true
    })
}

fn is_mapped(
    event: &Result<Event, x11rb::errors::ConnectionError>,
    window: u32,
) -> bool {
    matches!(event, Ok(Event::MapNotify(mapped)) if mapped.window == window)
}

fn mapped_parent(
    connection: &RustConnection,
    screen: &Screen,
) -> TestResult<u32> {
    let window = connection.generate_id()?;
    let events = CreateWindowAux::new().event_mask(EventMask::STRUCTURE_NOTIFY);
    connection.create_window(
        COPY_DEPTH_FROM_PARENT,
        window,
        screen.root,
        0,
        0,
        SIDE,
        SIDE,
        0,
        WindowClass::INPUT_OUTPUT,
        screen.root_visual,
        &events,
    )?;
    connection.map_window(window)?;
    connection.flush()?;
    std::iter::repeat_with(|| connection.wait_for_event())
        .find(|event| is_mapped(event, window) || event.is_err())
        .ok_or("the event stream ended")??;
    Ok(window)
}

fn wait_for(
    messages: &mpsc::Receiver<String>,
    wanted: &str,
    patience: Duration,
) -> Result<(), mpsc::RecvTimeoutError> {
    std::iter::repeat_with(|| messages.recv_timeout(patience))
        .find(|message| message.as_ref().map_or(true, |text| text == wanted))
        .map_or(Err(mpsc::RecvTimeoutError::Disconnected), |found| {
            found.map(drop)
        })
}

fn page_in(
    host: &Host,
    parent: u32,
) -> TestResult<(Handle, mpsc::Receiver<String>)> {
    let (sender, messages) = mpsc::channel();
    let side = f64::from(SIDE);
    let bounds = Bounds {
        x: 0.0,
        y: 0.0,
        width: side,
        height: side,
    };
    let config = Config {
        url: PAGE_URL.into(),
        bounds,
        kind: Kind::Core,
        visibility: Visibility::Shown,
        background: None,
        initialization_script: None,
        on_ipc: forward_to(sender.clone()),
        on_page: Some(report_to(sender)),
    };
    let view = host.create_in(ParentWindow::X11(parent), config)?;
    wait_for(&messages, "ready", PAGE_WAIT)?;
    Ok((view, messages))
}

fn park_pointer_outside(
    connection: &RustConnection,
    screen: &Screen,
) -> TestResult {
    let x = i16::try_from(screen.width_in_pixels - 1)?;
    let y = i16::try_from(screen.height_in_pixels - 1)?;
    connection.warp_pointer(NONE, screen.root, 0, 0, 0, 0, x, y)?;
    connection.get_input_focus()?.reply()?;
    Ok(())
}

fn press(
    connection: &RustConnection,
    screen: &Screen,
) -> TestResult {
    for event in [BUTTON_PRESS_EVENT, BUTTON_RELEASE_EVENT] {
        connection.xtest_fake_input(event, LEFT_BUTTON, CURRENT_TIME, screen.root, 0, 0, 0)?;
    }
    connection.get_input_focus()?.reply()?;
    Ok(())
}

fn type_a(
    connection: &RustConnection,
    screen: &Screen,
) -> TestResult {
    for event in [KEY_PRESS_EVENT, KEY_RELEASE_EVENT] {
        connection.xtest_fake_input(event, KEYCODE_A, CURRENT_TIME, screen.root, 0, 0, 0)?;
    }
    connection.get_input_focus()?.reply()?;
    Ok(())
}

fn a_focused_view_takes_keys_while_the_pointer_is_elsewhere(
    connection: &RustConnection,
    screen: &Screen,
    host: &Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent)?;
    park_pointer_outside(connection, screen)?;
    view.focus()?;
    view.eval("window.ipc.postMessage('focused')")?;
    wait_for(&messages, "focused", PAGE_WAIT)?;
    type_a(connection, screen)?;
    assert_eq!(
        wait_for(&messages, "key a", KEY_WAIT),
        Ok(()),
        "the page never saw the key"
    );
    Ok(())
}

fn a_press_on_the_page_is_reported(
    connection: &RustConnection,
    screen: &Screen,
    host: &Host,
    parent: u32,
) -> TestResult {
    let (_view, messages) = page_in(host, parent)?;
    connection.warp_pointer(NONE, parent, 0, 0, 0, 0, INSIDE, INSIDE)?;
    press(connection, screen)?;
    assert_eq!(
        wait_for(&messages, "Pressed", KEY_WAIT),
        Ok(()),
        "the press was not reported"
    );
    Ok(())
}

#[test]
#[ignore = "needs display"]
fn native_views_on_x11() -> TestResult {
    let (connection, screen_index) = RustConnection::connect(None)?;
    let screen = connection.setup().roots[screen_index].clone();
    let parent = mapped_parent(&connection, &screen)?;
    let host = Host::native(HostConfig {
        scheme: "crowbar".into(),
        protocol: Arc::new(Page(PAGE)),
        data_dir: None,
    })?;
    a_focused_view_takes_keys_while_the_pointer_is_elsewhere(&connection, &screen, &host, parent)?;
    a_press_on_the_page_is_reported(&connection, &screen, &host, parent)
}
