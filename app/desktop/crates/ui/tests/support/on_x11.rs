//! What the shell needs of GPUI that only a real X11 window shows (CI's Xvfb), pinned so that a
//! gpui upgrade which breaks one fails here by name.

use std::sync::mpsc;
use std::time::{Duration, Instant};

use gpui::{
    App, AppContext as _, Bounds, Context, FontId, Hsla, IntoElement, ParentElement as _, Render,
    Styled as _, TextRun, Window, WindowBounds, WindowOptions, black, div, font, point, px, rgb,
    size,
};
use raw_window_handle::{HasWindowHandle, RawWindowHandle};
use ui::{Icon, IconName, IconWeight};
use x11rb::protocol::xproto::{ConnectionExt as _, ImageFormat as XImageFormat};

const WINDOW_PIXELS: u16 = 200;
const ICON_SIDE: f32 = 96.0;
const SAMPLE: &str = "Crowbar";
const SAMPLE_SIZE: f32 = 32.0;
const FRAME_WAIT: Duration = Duration::from_secs(20);
const RED: u32 = 0x00ff_0000;
/// A pixel counts as red when its red channel is at least this and the others at most `DARK`.
const BRIGHT: u8 = 200;
const DARK: u8 = 60;
const MIN_RED_PIXELS: usize = 100;

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
}

/// One real window under X11 (CI's Xvfb):
/// - `0001-x11-window-handle`: the window hands out its XCB handle, which web views parent to;
/// - `0004-svg-inherit-color`: an icon with no colour paints in the text colour around it;
/// - font weights: the UI family shapes wider at a heavier weight, and an italic asked of it
///   gets its oblique face.
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
    let window = cx
        .open_window(options, |_, cx| cx.new(|_| IconProbe { colour: RED }))
        .map_err(|error| format!("opening the probe window: {error}"))?;
    window
        .update(cx, |_, window, _| {
            window.on_next_frame(move |window, cx| capture(sender, deadline, window, cx));
        })
        .map_err(|error| format!("the probe window closed: {error}"))
}

fn capture(
    report: Report,
    deadline: Instant,
    window: &mut Window,
    cx: &mut App,
) {
    let xcb_window = xcb_window(window);
    let red_pixels = xcb_window.map_or(0, red_pixels);
    if red_pixels < MIN_RED_PIXELS && Instant::now() < deadline {
        window.refresh();
        window.on_next_frame(move |window, cx| capture(report, deadline, window, cx));
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
    };
    report.send(Ok(seen)).unwrap_or_default();
    cx.quit();
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
    };
    let layout = window
        .text_system()
        .layout_line(text, px(text_size), &[run], None);
    f32::from(layout.width)
}
