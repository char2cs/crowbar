//! The patches in `app/desktop/vendor/patches/gpui` that only a real X11 window shows (CI's
//! Xvfb), pinned so that a gpui upgrade which drops one fails here by name.

#[path = "xdnd_relay.rs"]
mod xdnd_relay;

use std::cell::RefCell;
use std::rc::Rc;
use std::sync::mpsc;
use std::time::{Duration, Instant};

use gpui::{
    App, AppContext as _, Bounds, Context, FontId, Hsla, IntoElement, ParentElement as _, Render,
    Styled as _, TextRun, Window, WindowBounds, WindowOptions, black, div, font, point, px, rgb,
    size,
};
use raw_window_handle::{HasWindowHandle, RawWindowHandle};
use ui::{Icon, IconName, IconWeight};
use x11rb::CURRENT_TIME;
use x11rb::connection::Connection as _;
use x11rb::protocol::xproto::{
    ConnectionExt as _, CreateWindowAux, ImageFormat as XImageFormat, InputFocus, WindowClass,
};
use x11rb::rust_connection::RustConnection;

const WINDOW_PIXELS: u16 = 200;
const ICON_SIDE: f32 = 96.0;
const SAMPLE: &str = "Crowbar";
const SAMPLE_SIZE: f32 = 32.0;
const BRANCH: &str = "feature/pricing-rounding";
/// The mono sizes of the tree's branch line and the branch pill, and `WebKitGTK`'s widths for
/// `BRANCH` at them: each 0.6em advance rounded to a whole pixel (6.3 to 6, 7.8 to 8).
const MONO_WIDTHS: [(f32, f32); 2] = [(10.5, 144.0), (13.0, 192.0)];
const FRAME_WAIT: Duration = Duration::from_secs(20);
const RED: u32 = 0x00ff_0000;
/// A pixel counts as red when its red channel is at least this and the others at most `DARK`.
const BRIGHT: u8 = 200;
const DARK: u8 = 60;
const MIN_RED_PIXELS: usize = 100;
const CHILD_SIDE: u16 = 10;

struct IconProbe {
    colour: u32,
}

impl Render for IconProbe {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div()
            .size_full()
            .bg(black())
            .text_color(rgb(self.colour))
            .child(
                Icon::new(IconName::Folder)
                    .weight(IconWeight::Fill)
                    .size(px(ICON_SIDE)),
            )
    }
}

#[derive(Debug)]
struct Seen {
    xcb_window: Option<u32>,
    red_pixels: usize,
    regular_width: f32,
    bold_width: f32,
    upright: FontId,
    italic: FontId,
    activity_while_a_child_held_the_keyboard: Vec<bool>,
}

struct ChildFocus {
    report: Report,
    seen: Option<Seen>,
    child_focused: bool,
}

type Check = Rc<RefCell<ChildFocus>>;

/// One real window under X11 (CI's Xvfb):
/// - `0001-x11-window-handle`: the window hands out its XCB handle, which web views parent to;
/// - `0004-svg-inherit-color`: an icon with no colour paints in the text colour around it;
/// - `0002-variable-weight`: the variable UI face shapes wider at a heavier weight, and an
///   italic asked of it gets a face of its own (synthesized, the family has no italic);
/// - `0003-inferior-focus`: the keyboard moving into one of the window's own child windows, as
///   into a web view, leaves the window active.
#[test]
#[ignore = "needs display"]
fn a_real_window_parents_paints_inherited_icon_colour_and_each_weight() {
    let (sender, seen) = mpsc::channel();
    gpui_platform::application()
        .with_assets(ui::assets())
        .run(move |cx| open_probe(&sender, cx));
    let seen = seen
        .try_recv()
        .expect("the probe window ran")
        .expect("the probe window drew a frame");
    assert!(seen.xcb_window.is_some(), "{seen:?}");
    assert!(seen.red_pixels >= MIN_RED_PIXELS, "{seen:?}");
    assert!(seen.bold_width > seen.regular_width, "{seen:?}");
    assert_ne!(seen.italic, seen.upright, "{seen:?}");
    assert_eq!(
        seen.activity_while_a_child_held_the_keyboard,
        [true],
        "the window went inactive while its own child window held the keyboard"
    );
}

/// `whole-pixel-mono` (gpui-pre-wgpu): a fixed-pitch face advances a whole pixel per glyph, as
/// `WebKitGTK` lays out the Tauri build's mono text, while proportional faces keep fractions.
#[test]
#[ignore = "needs display"]
fn a_fixed_pitch_face_advances_a_whole_pixel_per_glyph() {
    let (sender, measured) = mpsc::channel();
    gpui_platform::application().run(move |cx| measure_mono(&sender, cx));
    let widths = measured
        .try_recv()
        .expect("the probe window ran")
        .expect("the probe window opened");
    let webkit: Vec<f32> = MONO_WIDTHS.iter().map(|(_, width)| *width).collect();
    assert_eq!(widths, webkit);
}

type Widths = mpsc::Sender<Result<Vec<f32>, String>>;

fn measure_mono(
    sender: &Widths,
    cx: &mut App,
) {
    let opened = ui::font::register(cx)
        .map_err(|error| format!("the embedded fonts: {error}"))
        .and_then(|()| open_mono_window(sender, cx));
    if let Err(error) = opened {
        sender.send(Err(error)).unwrap_or_default();
        cx.quit();
    }
}

fn open_mono_window(
    sender: &Widths,
    cx: &mut App,
) -> Result<(), String> {
    cx.open_window(WindowOptions::default(), |window, cx| {
        sender.send(Ok(mono_widths(window))).unwrap_or_default();
        window.on_next_frame(|_, cx| cx.quit());
        cx.new(|_| IconProbe { colour: RED })
    })
    .map(|_| ())
    .map_err(|error| format!("opening the probe window: {error}"))
}

fn mono_widths(window: &Window) -> Vec<f32> {
    MONO_WIDTHS
        .iter()
        .map(|(size, _)| text_width(window, (font(ui::font::MONO_FAMILY), BRANCH, *size)))
        .collect()
}

type Report = mpsc::Sender<Result<Seen, String>>;

fn open_probe(
    sender: &Report,
    cx: &mut App,
) {
    if let Err(error) = open_window(sender.clone(), cx) {
        sender.send(Err(error)).unwrap_or_default();
        cx.quit();
    }
}

fn open_window(
    sender: Report,
    cx: &mut App,
) -> Result<(), String> {
    ui::font::register(cx).map_err(|error| format!("the embedded fonts: {error}"))?;
    let deadline = Instant::now() + FRAME_WAIT;
    cx.spawn(async move |cx| {
        cx.background_executor().timer(FRAME_WAIT).await;
        cx.update(|cx| cx.quit());
    })
    .detach();
    let bounds = Bounds::new(
        point(px(0.0), px(0.0)),
        size(px(f32::from(WINDOW_PIXELS)), px(f32::from(WINDOW_PIXELS))),
    );
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let check: Check = Rc::new(RefCell::new(ChildFocus {
        report: sender,
        seen: None,
        child_focused: false,
    }));
    let heard = Rc::clone(&check);
    let window = cx
        .open_window(options, move |window, cx| new_probe(heard, window, cx))
        .map_err(|error| format!("opening the probe window: {error}"))?;
    window
        .update(cx, |_, window, _| {
            window.on_next_frame(move |window, cx| capture(check, deadline, window, cx));
        })
        .map_err(|error| format!("the probe window closed: {error}"))
}

fn new_probe(
    heard: Check,
    window: &mut Window,
    cx: &mut App,
) -> gpui::Entity<IconProbe> {
    let probe = cx.new(|_| IconProbe { colour: RED });
    probe.update(cx, |_, cx| watch_activation(heard, window, cx));
    probe
}

fn watch_activation(
    heard: Check,
    window: &mut Window,
    cx: &mut Context<'_, IconProbe>,
) {
    cx.observe_window_activation(window, move |_, window, cx| {
        on_activation(&heard, window, cx);
    })
    .detach();
}

fn capture(
    check: Check,
    deadline: Instant,
    window: &mut Window,
    cx: &mut App,
) {
    let xcb_window = xcb_window(window);
    let red_pixels = xcb_window.map_or(0, red_pixels);
    if red_pixels < MIN_RED_PIXELS && Instant::now() < deadline {
        window.refresh();
        window.on_next_frame(move |window, cx| capture(check, deadline, window, cx));
        return;
    }
    let ui_font = font(ui::font::UI_FAMILY);
    let seen = Seen {
        xcb_window,
        red_pixels,
        regular_width: sample_width(window, ui_font.clone()),
        bold_width: sample_width(window, ui_font.clone().bold()),
        upright: cx.text_system().resolve_font(&ui_font),
        italic: cx.text_system().resolve_font(&ui_font.italic()),
        activity_while_a_child_held_the_keyboard: Vec::new(),
    };
    check.borrow_mut().seen = Some(seen);
    if let Err(error) = focus_the_window(&check, window) {
        fail(&check, error, cx);
    }
}

fn focus_the_window(
    check: &Check,
    window: &Window,
) -> Result<(), String> {
    let parent = xcb_window(window).ok_or("the window has no XCB handle")?;
    let (connection, _) = x11rb::connect(None).map_err(|error| error.to_string())?;
    let focus = connection
        .get_input_focus()
        .map_err(|error| error.to_string())?
        .reply()
        .map_err(|error| error.to_string())?;
    if focus.focus == parent && window.is_window_active() {
        return focus_a_child(check, parent);
    }
    connection
        .set_input_focus(InputFocus::PARENT, parent, CURRENT_TIME)
        .map_err(|error| error.to_string())?
        .check()
        .map_err(|error| format!("focusing the window: {error}"))?;
    Ok(())
}

fn on_activation(
    check: &Check,
    window: &Window,
    cx: &mut App,
) {
    let active = window.is_window_active();
    let Some(parent) = xcb_window(window) else {
        return;
    };
    let mut state = check.borrow_mut();
    let child_focused = state.child_focused;
    let Some(seen) = state.seen.as_mut() else {
        return;
    };
    if child_focused {
        seen.activity_while_a_child_held_the_keyboard.push(active);
    }
    drop(state);
    if !active {
        return;
    }
    if child_focused {
        return finish(check, cx);
    }
    if let Err(error) = focus_a_child(check, parent) {
        fail(check, error, cx);
    }
}

fn finish(
    check: &Check,
    cx: &mut App,
) {
    let mut state = check.borrow_mut();
    if let Some(seen) = state.seen.take() {
        state.report.send(Ok(seen)).unwrap_or_default();
    }
    cx.quit();
}

fn fail(
    check: &Check,
    error: String,
    cx: &mut App,
) {
    check.borrow().report.send(Err(error)).unwrap_or_default();
    cx.quit();
}

/// Gives the keyboard to a child window of `parent` and back, as the shell does to a web view.
fn focus_a_child(
    check: &Check,
    parent: u32,
) -> Result<(), String> {
    let (connection, _) = x11rb::connect(None).map_err(|error| error.to_string())?;
    check.borrow_mut().child_focused = true;
    hand_the_keyboard_through_a_child(&connection, parent).map_err(|error| error.to_string())
}

fn hand_the_keyboard_through_a_child(
    connection: &RustConnection,
    parent: u32,
) -> Result<(), Box<dyn std::error::Error>> {
    let child = connection.generate_id()?;
    connection.create_window(
        x11rb::COPY_DEPTH_FROM_PARENT,
        child,
        parent,
        0,
        0,
        CHILD_SIDE,
        CHILD_SIDE,
        0,
        WindowClass::INPUT_OUTPUT,
        x11rb::COPY_FROM_PARENT,
        &CreateWindowAux::new(),
    )?;
    connection.map_window(child)?;
    connection.set_input_focus(InputFocus::PARENT, child, CURRENT_TIME)?;
    connection.set_input_focus(InputFocus::PARENT, parent, CURRENT_TIME)?;
    connection.get_input_focus()?.reply()?;
    Ok(())
}

fn xcb_window(window: &Window) -> Option<u32> {
    let handle = HasWindowHandle::window_handle(window).ok()?;
    match handle.as_raw() {
        RawWindowHandle::Xcb(xcb) => Some(xcb.window.get()),
        _ => None,
    }
}

fn red_pixels(xcb_window: u32) -> usize {
    let Ok((connection, _)) = x11rb::connect(None) else {
        return 0;
    };
    let image = connection
        .get_image(
            XImageFormat::Z_PIXMAP,
            xcb_window,
            0,
            0,
            WINDOW_PIXELS,
            WINDOW_PIXELS,
            u32::MAX,
        )
        .ok()
        .and_then(|cookie| cookie.reply().ok());
    image.map_or(0, |image| {
        image
            .data
            .as_chunks::<4>()
            .0
            .iter()
            .filter(|[blue, green, red, _]| *red >= BRIGHT && *green <= DARK && *blue <= DARK)
            .count()
    })
}

fn sample_width(
    window: &Window,
    font: gpui::Font,
) -> f32 {
    text_width(window, (font, SAMPLE, SAMPLE_SIZE))
}

fn text_width(
    window: &Window,
    (font, text, text_size): (gpui::Font, &str, f32),
) -> f32 {
    let run = TextRun {
        len: text.len(),
        font,
        color: Hsla::default(),
        background_color: None,
        underline: None,
        strikethrough: None,
        letter_spacing: gpui::Pixels::ZERO,
    };
    let layout = window
        .text_system()
        .layout_line(text, px(text_size), &[run], None);
    f32::from(layout.width)
}
