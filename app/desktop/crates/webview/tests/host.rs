//! Native web views against a real X server and `WebKitGTK`: where their keys go, what a press
//! on them reports and the caret blink the desktop's settings manager announces.
//!
//! GTK runs on one thread per process, so one test drives every case on one host.
#![cfg(target_os = "linux")]

#[path = "support/pixels.rs"]
mod pixels;
#[path = "support/placement.rs"]
mod placement;
#[path = "support/script_dialog.rs"]
mod script_dialog;

use std::sync::{Arc, mpsc};
use std::time::{Duration, Instant};

use webview::{
    Bounds, CaretBlink, Config, Handle, Host, HostConfig, IpcHandler, Kind, PageHandler,
    ParentWindow, ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse,
    Visibility,
};
use x11rb::connection::Connection as _;
use x11rb::protocol::Event;
use x11rb::protocol::xproto::{
    BUTTON_PRESS_EVENT, BUTTON_RELEASE_EVENT, ChangeWindowAttributesAux, ClientMessageEvent,
    ConnectionExt as _, CreateWindowAux, EventMask, InputFocus, KEY_PRESS_EVENT, KEY_RELEASE_EVENT,
    PropMode, Screen, WindowClass,
};
use x11rb::protocol::xtest::ConnectionExt as _;
use x11rb::rust_connection::RustConnection;
use x11rb::{COPY_DEPTH_FROM_PARENT, CURRENT_TIME, NONE};

type TestResult<T = ()> = Result<T, Box<dyn std::error::Error>>;

const PAGE: &str = "<script>
addEventListener('keydown', (event) => window.ipc.postMessage('key ' + event.key));
addEventListener('load', () => window.ipc.postMessage('ready'));
addEventListener('focus', () => window.ipc.postMessage('has focus ' + document.hasFocus()));
addEventListener('blur', () => window.ipc.postMessage('has focus ' + document.hasFocus()));
</script>";
const PAGE_URL: &str = "crowbar://test/page";
const PAGE_WAIT: Duration = Duration::from_secs(30);
const KEY_WAIT: Duration = Duration::from_secs(5);
const KEYCODE_A: u8 = 38;
const SIDE: u16 = 300;
const INSIDE: i16 = 50;
const LEFT_BUTTON: u8 = 1;
const REPORT_WAIT: Duration = Duration::from_secs(10);
const LITTLE_ENDIAN: u8 = 0;
const INTEGER: u8 = 0;
const BLINK: &str = "Net/CursorBlink";
const BLINK_TIME: &str = "Net/CursorBlinkTime";
const MANAGER: &str = "MANAGER";
const SETTINGS: &str = "_XSETTINGS_SETTINGS";
const FORMAT_BYTES: u8 = 8;
const FORMAT_WORDS: u8 = 32;

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
    let events = CreateWindowAux::new()
        .event_mask(EventMask::STRUCTURE_NOTIFY)
        .background_pixel(screen.black_pixel);
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
    visibility: Visibility,
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
        visibility,
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

fn type_key(
    connection: &RustConnection,
    screen: &Screen,
    keycode: u8,
) -> TestResult {
    for event in [KEY_PRESS_EVENT, KEY_RELEASE_EVENT] {
        connection.xtest_fake_input(event, keycode, CURRENT_TIME, screen.root, 0, 0, 0)?;
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
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    park_pointer_outside(connection, screen)?;
    view.focus()?;
    view.eval("window.ipc.postMessage('focused')")?;
    wait_for(&messages, "focused", PAGE_WAIT)?;
    type_key(connection, screen, KEYCODE_A)?;
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
    let (_view, messages) = page_in(host, parent, Visibility::Shown)?;
    connection.warp_pointer(NONE, parent, 0, 0, 0, 0, INSIDE, INSIDE)?;
    press(connection, screen)?;
    assert_eq!(
        wait_for(&messages, "Pressed", KEY_WAIT),
        Ok(()),
        "the press was not reported"
    );
    Ok(())
}

fn the_page_holding_the_keyboard_has_document_focus(
    connection: &RustConnection,
    screen: &Screen,
    host: &Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    view.focus()?;
    assert_eq!(
        wait_for(&messages, "has focus true", KEY_WAIT),
        Ok(()),
        "the focused page is not focused for the document"
    );
    view.focus_parent()?;
    assert_eq!(
        wait_for(&messages, "has focus false", KEY_WAIT),
        Ok(()),
        "the page kept document focus after the shell took the keyboard"
    );
    view.focus()?;
    wait_for(&messages, "has focus true", KEY_WAIT)?;
    let other_app = mapped_parent(connection, screen)?;
    connection.set_input_focus(InputFocus::PARENT, other_app, CURRENT_TIME)?;
    connection.flush()?;
    assert_eq!(
        wait_for(&messages, "has focus false", KEY_WAIT),
        Ok(()),
        "the page kept document focus after another window took the keyboard"
    );
    Ok(())
}

fn keys_reaching(
    connection: &RustConnection,
    window: u32,
) -> TestResult<usize> {
    let mut heard = 0;
    while let Some(event) = connection.poll_for_event()? {
        heard += usize::from(matches!(event, Event::KeyPress(key) if key.event == window));
    }
    Ok(heard)
}

fn the_parent_takes_keys_over_a_page_once_it_holds_the_keyboard(
    connection: &RustConnection,
    screen: &Screen,
    host: &Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    let listening = EventMask::STRUCTURE_NOTIFY | EventMask::KEY_PRESS;
    connection.change_window_attributes(
        parent,
        &ChangeWindowAttributesAux::new().event_mask(listening),
    )?;
    connection.warp_pointer(NONE, parent, 0, 0, 0, 0, INSIDE, INSIDE)?;
    view.focus()?;
    view.focus_parent()?;
    view.eval("window.ipc.postMessage('handed back')")?;
    wait_for(&messages, "handed back", PAGE_WAIT)?;
    keys_reaching(connection, parent)?;
    type_key(connection, screen, KEYCODE_A)?;
    assert_eq!(
        keys_reaching(connection, parent)?,
        1,
        "the page under the pointer took the key"
    );
    view.focus()?;
    view.eval("window.ipc.postMessage('taken')")?;
    wait_for(&messages, "taken", PAGE_WAIT)?;
    type_key(connection, screen, KEYCODE_A)?;
    assert_eq!(
        wait_for(&messages, "key a", KEY_WAIT),
        Ok(()),
        "the page did not get its keys back"
    );
    Ok(())
}

struct Manager {
    connection: RustConnection,
    window: u32,
    settings: u32,
    serial: u32,
}

impl Manager {
    fn start() -> TestResult<Self> {
        let (connection, screen_index) = RustConnection::connect(None)?;
        let root = connection
            .setup()
            .roots
            .get(screen_index)
            .ok_or("no screen")?
            .root;
        let window = owner_window(&connection, root)?;
        let selection = atom(&connection, &format!("_XSETTINGS_S{screen_index}"))?;
        let settings = atom(&connection, SETTINGS)?;
        let mut manager = Self {
            connection,
            window,
            settings,
            serial: 0,
        };
        manager.announce((1, 1000))?;
        manager.claim(root, selection)?;
        Ok(manager)
    }

    fn claim(
        &self,
        root: u32,
        selection: u32,
    ) -> TestResult {
        self.connection
            .set_selection_owner(self.window, selection, CURRENT_TIME)?;
        let message = ClientMessageEvent::new(
            FORMAT_WORDS,
            root,
            atom(&self.connection, MANAGER)?,
            [CURRENT_TIME, selection, self.window, 0, 0],
        );
        self.connection
            .send_event(false, root, EventMask::STRUCTURE_NOTIFY, message)?;
        self.connection.flush()?;
        Ok(())
    }

    fn announce(
        &mut self,
        (blink, time): (i32, i32),
    ) -> TestResult {
        self.serial += 1;
        let mut bytes = vec![LITTLE_ENDIAN, 0, 0, 0];
        bytes.extend(self.serial.to_le_bytes());
        bytes.extend(2_u32.to_le_bytes());
        integer(&mut bytes, (BLINK, blink), self.serial);
        integer(&mut bytes, (BLINK_TIME, time), self.serial);
        self.connection.change_property(
            PropMode::REPLACE,
            self.window,
            self.settings,
            self.settings,
            FORMAT_BYTES,
            u32::try_from(bytes.len())?,
            &bytes,
        )?;
        self.connection.flush()?;
        Ok(())
    }
}

fn owner_window(
    connection: &RustConnection,
    root: u32,
) -> TestResult<u32> {
    let window = connection.generate_id()?;
    let aux = CreateWindowAux::new().event_mask(EventMask::PROPERTY_CHANGE);
    connection.create_window(
        COPY_DEPTH_FROM_PARENT,
        window,
        root,
        0,
        0,
        1,
        1,
        0,
        WindowClass::INPUT_ONLY,
        0,
        &aux,
    )?;
    Ok(window)
}

fn atom(
    connection: &RustConnection,
    name: &str,
) -> TestResult<u32> {
    Ok(connection
        .intern_atom(false, name.as_bytes())?
        .reply()?
        .atom)
}

fn integer(
    bytes: &mut Vec<u8>,
    (name, value): (&str, i32),
    serial: u32,
) {
    bytes.extend([INTEGER, 0]);
    bytes.extend(u16::try_from(name.len()).unwrap_or_default().to_le_bytes());
    bytes.extend(name.as_bytes());
    bytes.resize(bytes.len().next_multiple_of(4), 0);
    bytes.extend(serial.to_le_bytes());
    bytes.extend(value.to_le_bytes());
}

fn until_reported(
    reports: &mpsc::Receiver<CaretBlink>,
    wanted: CaretBlink,
) -> Result<(), Vec<CaretBlink>> {
    let deadline = Instant::now() + REPORT_WAIT;
    let mut heard = Vec::new();
    let found = std::iter::from_fn(|| {
        reports
            .recv_timeout(deadline.saturating_duration_since(Instant::now()))
            .ok()
    })
    .inspect(|blink| heard.push(*blink))
    .any(|blink| blink == wanted);
    if found {
        return Ok(());
    }
    Err(heard)
}

fn the_caret_follows_the_desktop_settings_manager(host: &Host) -> TestResult {
    let (sender, reports) = mpsc::channel();
    host.follow_caret(Arc::new(move |blink| {
        sender.send(blink).unwrap_or_default();
    }))?;
    let mut manager = Manager::start()?;
    let phase = |millis| CaretBlink::Every(Duration::from_millis(millis));
    assert_eq!(
        until_reported(&reports, phase(500)),
        Ok(()),
        "1000 ms cycle"
    );
    manager.announce((0, 1000))?;
    assert_eq!(
        until_reported(&reports, CaretBlink::Steady),
        Ok(()),
        "blink off"
    );
    manager.announce((1, 800))?;
    assert_eq!(until_reported(&reports, phase(400)), Ok(()), "800 ms cycle");
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
    placement::a_spare_page_paints_over_the_page_it_replaces(&connection, &host, parent)?;
    script_dialog::a_dialog_shows_over_its_page_and_answers_it(
        &connection,
        &screen,
        &host,
        parent,
    )?;
    script_dialog::a_prompt_takes_typing_after_the_keyboard_comes_back(
        &connection,
        &screen,
        &host,
        parent,
    )?;
    a_focused_view_takes_keys_while_the_pointer_is_elsewhere(&connection, &screen, &host, parent)?;
    a_press_on_the_page_is_reported(&connection, &screen, &host, parent)?;
    the_page_holding_the_keyboard_has_document_focus(&connection, &screen, &host, parent)?;
    the_parent_takes_keys_over_a_page_once_it_holds_the_keyboard(
        &connection,
        &screen,
        &host,
        parent,
    )?;
    the_caret_follows_the_desktop_settings_manager(&host)
}
