//! Overlay windows (`WindowOptions::overlay_of`): where one goes over its owner and how the
//! owner's keyboard is lent to it. The window manager never focuses an overlay; the owner keeps
//! X focus and its key events are delivered to the overlay while the overlay takes keys.

use x11rb::protocol::composite::{self, ConnectionExt as _};
use x11rb::protocol::damage::{self, ConnectionExt as _};
use x11rb::connection::Connection as _;
use x11rb::protocol::randr::ConnectionExt as _;
use x11rb::protocol::xproto::{self, ConnectionExt as _};
use x11rb::xcb_ffi::XCBConnection;

use gpui::{Bounds, Pixels, Point, Size, point, px, size};

/// The overlay's side of an owner/overlay pair, kept in the overlay's window state.
#[derive(Debug, Clone)]
pub(crate) struct OverlayLink {
    pub(crate) owner: xproto::Window,
    /// The overlay's rectangle in root coordinates, in device pixels, as its last configure
    /// event reported it: the window manager may move it elsewhere than asked.
    pub(crate) window: Bounds<i32>,
    /// The owner's content area in root coordinates, in device pixels, from its configure events.
    pub(crate) owner_area: Bounds<i32>,
    /// Whether a window manager framed the overlay, and the owner: a framed window's real
    /// configure events give its position in the frame, so only synthetic ones move it.
    pub(crate) framed: Framing,
    pub(crate) takes_keys: bool,
    pub(crate) region: Vec<xproto::Rectangle>,
    /// What the last frame painted opaque, in device pixels.
    pub(crate) content: Vec<xproto::Rectangle>,
    /// Whether a compositor blends the overlay over its owner.
    pub(crate) composited: bool,
    /// Whether the overlay is mapped.
    pub(crate) shown: bool,
    /// The input-only child of the owner that takes the input region where the overlay was cut
    /// away (see [`needs_catcher`]), once created.
    pub(crate) catcher: Option<xproto::Window>,
    /// How many times the owner's content changed, as damage reports it.
    pub(crate) owner_version: u64,
    /// Whether the overlay captured its owner since it was shown, and so redraws when the owner
    /// changes.
    pub(crate) watching: bool,
}

/// Which windows of an overlay pair a window manager reparented into a frame.
#[derive(Debug, Default, Clone, Copy)]
pub(crate) struct Framing {
    pub(crate) overlay: bool,
    pub(crate) owner: bool,
}

impl OverlayLink {
    /// The owner's content area in the overlay's coordinates, in logical pixels.
    pub(crate) fn owner_bounds(&self, scale_factor: f32) -> Bounds<Pixels> {
        let offset = point(
            self.owner_area.origin.x - self.window.origin.x,
            self.owner_area.origin.y - self.window.origin.y,
        );
        let placed = Placement { window: self.window, offset };
        placed.owner_bounds(self.owner_area.size, scale_factor)
    }
}

/// Where a window is after a configure event `event` about it, last known at `known`, given
/// whether it sits in a window manager's frame. Root coordinates, device pixels.
pub(crate) fn configured(
    known: Bounds<i32>,
    event: &xproto::ConfigureNotifyEvent,
    framed: bool,
) -> Bounds<i32> {
    let synthetic = event.response_type & SYNTHETIC != 0;
    let origin = match synthetic || !framed {
        true => point(event.x.into(), event.y.into()),
        false => known.origin,
    };
    Bounds::new(origin, size(event.width.into(), event.height.into()))
}

/// The bit X sets in the response type of an event a client sent, such as the root-coordinate
/// configure events ICCCM 4.1.5 has window managers send.
const SYNTHETIC: u8 = 0x80;

/// Without a compositor the overlay is cut down to its content, and X clips a window's input to
/// its shape, so an invisible part of the input region, such as a menu's click-away layer, would
/// let presses through to the owner and its web views. An input-only child of the owner,
/// stacked above the web views, takes that part instead.
pub(crate) fn needs_catcher(link: &OverlayLink) -> bool {
    link.shown && !link.composited
}

/// `region` (overlay coordinates) in the owner's coordinates, the owner sitting at `offset`
/// inside the overlay; all in device pixels.
pub(crate) fn in_owner(region: &[xproto::Rectangle], offset: Point<i32>) -> Vec<xproto::Rectangle> {
    region
        .iter()
        .map(|rect| xproto::Rectangle {
            x: (i32::from(rect.x) - offset.x) as i16,
            y: (i32::from(rect.y) - offset.y) as i16,
            ..*rect
        })
        .collect()
}

/// A rectangle just outside the window, set in place of an empty shape. Reparenting window
/// managers such as openbox copy a client's shape onto their frame only while it has rectangles
/// and take an empty one for no shape, which leaves the whole frame painted and clickable.
pub(crate) const NOTHING: xproto::Rectangle = xproto::Rectangle {
    x: -1,
    y: -1,
    width: 1,
    height: 1,
};

/// `_MOTIF_WM_HINTS` of an overlay: the decorations field is set and asks for none, whether or
/// not a compositor runs, so no window manager draws a title bar or border around it.
pub(crate) const UNDECORATED: [u32; 5] = [MWM_HINTS_DECORATIONS, 0, 0, 0, 0];
const MWM_HINTS_DECORATIONS: u32 = 1 << 1;

/// The rectangles of a shape covering `rects`, or [`NOTHING`] when there are none.
pub(crate) fn shape_rects(rects: &[xproto::Rectangle]) -> Vec<xproto::Rectangle> {
    if rects.is_empty() {
        return vec![NOTHING];
    }
    rects.to_vec()
}

/// The bounding shape of an overlay: none while a compositor blends its transparent pixels;
/// without one they would show black, so the window is cut down to what it painted opaque.
pub(crate) fn bounding(
    content: &[xproto::Rectangle],
    composited: bool,
) -> Option<Vec<xproto::Rectangle>> {
    (!composited).then(|| shape_rects(content))
}

/// The selection a compositing manager owns on screen `screen` (EWMH `_NET_WM_CM_Sn`).
pub(crate) fn compositor_selection(screen: usize) -> String {
    format!("_NET_WM_CM_S{screen}")
}

/// An overlay window's rectangle in root coordinates and the owner's origin inside it, in
/// device pixels.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Placement {
    pub(crate) window: Bounds<i32>,
    pub(crate) offset: Point<i32>,
}

impl Placement {
    pub(crate) fn owner_bounds(&self, owner: Size<i32>, scale_factor: f32) -> Bounds<Pixels> {
        Bounds::new(
            point(
                px(self.offset.x as f32 / scale_factor),
                px(self.offset.y as f32 / scale_factor),
            ),
            size(
                px(owner.width as f32 / scale_factor),
                px(owner.height as f32 / scale_factor),
            ),
        )
    }
}

fn overlap(a: Bounds<i32>, b: Bounds<i32>) -> Option<Bounds<i32>> {
    let left = a.origin.x.max(b.origin.x);
    let top = a.origin.y.max(b.origin.y);
    let right = (a.origin.x + a.size.width).min(b.origin.x + b.size.width);
    let bottom = (a.origin.y + a.size.height).min(b.origin.y + b.size.height);
    (right > left && bottom > top)
        .then(|| Bounds::new(point(left, top), size(right - left, bottom - top)))
}

/// Covers the owner's content area `owner` (root coordinates) clipped to the monitor it overlaps
/// most: window managers push a window that leaves its monitor back onto it, which would shift
/// the overlay off its owner, so the overlay stays on the monitor and draws with an offset.
pub(crate) fn placement(owner: Bounds<i32>, monitors: &[Bounds<i32>]) -> Placement {
    let window = monitors
        .iter()
        .filter_map(|monitor| overlap(owner, *monitor))
        .max_by_key(|visible| visible.size.width * visible.size.height)
        .unwrap_or(owner);
    Placement {
        window,
        offset: point(
            owner.origin.x - window.origin.x,
            owner.origin.y - window.origin.y,
        ),
    }
}

/// The content area of `owner` in root coordinates.
pub(crate) fn owner_area(
    xcb: &XCBConnection,
    owner: xproto::Window,
    root: xproto::Window,
) -> Option<Bounds<i32>> {
    let origin = xcb
        .translate_coordinates(owner, root, 0, 0)
        .ok()?
        .reply()
        .ok()?;
    let geometry = xcb.get_geometry(owner).ok()?.reply().ok()?;
    Some(Bounds::new(
        point(origin.dst_x.into(), origin.dst_y.into()),
        size(geometry.width.into(), geometry.height.into()),
    ))
}

/// Has the server keep `owner`'s content, its child web views included, off screen (Composite
/// automatic redirection), so [`capture`] reads it whatever the overlay paints above it, and
/// report when that content changes (a `DamageNotify` for `owner`).
pub(crate) fn redirect(xcb: &XCBConnection, owner: xproto::Window) -> anyhow::Result<()> {
    xcb.composite_query_version(0, 4)?.reply()?;
    xcb.composite_redirect_window(owner, composite::Redirect::AUTOMATIC)?
        .check()?;
    xcb.damage_query_version(1, 1)?.reply()?;
    let watch = xcb.generate_id()?;
    xcb.damage_create(watch, owner, damage::ReportLevel::NON_EMPTY)?
        .check()?;
    Ok(())
}

/// Clears what `watch` reported, so the next change of the owner reports again.
pub(crate) fn acknowledge_damage(xcb: &XCBConnection, watch: damage::Damage) {
    let _ = xcb.damage_subtract(watch, x11rb::NONE, x11rb::NONE);
    let _ = xcb.flush();
}

/// The content of `owner`, `size` device pixels, as opaque BGRA; `None` unless it is a 24- or
/// 32-bit window.
pub(crate) fn capture(
    xcb: &XCBConnection,
    owner: xproto::Window,
    size: Size<i32>,
) -> Option<image::RgbaImage> {
    let (width, height) = (u16::try_from(size.width).ok()?, u16::try_from(size.height).ok()?);
    let reply = xcb
        .get_image(xproto::ImageFormat::Z_PIXMAP, owner, 0, 0, width, height, !0)
        .ok()?
        .reply()
        .ok()?;
    if reply.depth != 24 && reply.depth != 32 {
        return None;
    }
    let mut bgra = reply.data;
    for pixel in bgra.chunks_exact_mut(4) {
        pixel[3] = u8::MAX;
    }
    image::RgbaImage::from_raw(width.into(), height.into(), bgra)
}

/// The active monitors in root coordinates; the whole root window when RandR cannot tell.
pub(crate) fn monitors(xcb: &XCBConnection, root: xproto::Window) -> Vec<Bounds<i32>> {
    let listed: Vec<Bounds<i32>> = xcb
        .randr_get_monitors(root, true)
        .ok()
        .and_then(|cookie| cookie.reply().ok())
        .map(|reply| {
            reply
                .monitors
                .iter()
                .map(|monitor| {
                    Bounds::new(
                        point(monitor.x.into(), monitor.y.into()),
                        size(monitor.width.into(), monitor.height.into()),
                    )
                })
                .collect()
        })
        .unwrap_or_default();
    if !listed.is_empty() {
        return listed;
    }
    xcb.get_geometry(root)
        .ok()
        .and_then(|cookie| cookie.reply().ok())
        .map(|root| {
            vec![Bounds::new(
                point(0, 0),
                size(root.width.into(), root.height.into()),
            )]
        })
        .unwrap_or_default()
}

/// `region` (logical pixels) as device rectangles covering at least the same area.
pub(crate) fn device_rects(region: &[Bounds<Pixels>], scale_factor: f32) -> Vec<xproto::Rectangle> {
    region
        .iter()
        .map(|bounds| {
            let left = (f32::from(bounds.origin.x) * scale_factor).floor();
            let top = (f32::from(bounds.origin.y) * scale_factor).floor();
            let right = (f32::from(bounds.right()) * scale_factor).ceil();
            let bottom = (f32::from(bounds.bottom()) * scale_factor).ceil();
            xproto::Rectangle {
                x: left as i16,
                y: top as i16,
                width: (right - left).max(0.0) as u16,
                height: (bottom - top).max(0.0) as u16,
            }
        })
        .collect()
}

/// The window that gets a key event X delivered to `window`: the overlay of `window` while it
/// takes keys, `window` itself otherwise.
pub(crate) fn key_target<'a>(
    window: xproto::Window,
    overlays: impl IntoIterator<Item = (xproto::Window, &'a OverlayLink)>,
) -> xproto::Window {
    overlays
        .into_iter()
        .find(|(_, link)| link.owner == window && link.takes_keys)
        .map_or(window, |(overlay, _)| overlay)
}

/// Where a key release goes: to the window its press went to, so a press and its release
/// never land in different windows; `now` when the press was not seen.
pub(crate) fn release_target(
    pressed_in: Option<xproto::Window>,
    now: xproto::Window,
) -> xproto::Window {
    pressed_in.unwrap_or(now)
}

/// Whether focus leaving `left` for `gained` stays inside one owner/overlay pair, in which case
/// the owner stays active.
pub(crate) fn stays_in_pair(
    left: xproto::Window,
    gained: xproto::Window,
    owner_of: impl Fn(xproto::Window) -> Option<xproto::Window>,
) -> bool {
    let owner = owner_of(left).unwrap_or(left);
    gained == owner || owner_of(gained) == Some(owner)
}

/// Whether the pointer at `root` (root coordinates, device pixels) is over a part of the overlay
/// that takes input, so that leaving the owner for it is not leaving the pair.
pub(crate) fn holds_pointer(link: &OverlayLink, root: Point<i32>) -> bool {
    let x = root.x - link.window.origin.x;
    let y = root.y - link.window.origin.y;
    link.region.iter().any(|rect| {
        let (left, top) = (i32::from(rect.x), i32::from(rect.y));
        x >= left
            && y >= top
            && x < left + i32::from(rect.width)
            && y < top + i32::from(rect.height)
    })
}

/// Whether a window manager reparented `window` into a frame, rather than leaving it a child of
/// `root`.
pub(crate) fn is_framed(xcb: &XCBConnection, window: xproto::Window, root: xproto::Window) -> bool {
    xcb.query_tree(window)
        .ok()
        .and_then(|cookie| cookie.reply().ok())
        .is_some_and(|tree| tree.parent != root)
}

/// Whether `window` is `ancestor` or one of its descendants, such as a web view parented into it.
pub(crate) fn is_within(
    xcb: &XCBConnection,
    mut window: xproto::Window,
    ancestor: xproto::Window,
) -> bool {
    while window != x11rb::NONE {
        if window == ancestor {
            return true;
        }
        let Some(tree) = xcb
            .query_tree(window)
            .ok()
            .and_then(|cookie| cookie.reply().ok())
        else {
            return false;
        };
        if tree.parent == tree.root {
            return false;
        }
        window = tree.parent;
    }
    false
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rect(x: i32, y: i32, width: i32, height: i32) -> Bounds<i32> {
        Bounds::new(point(x, y), size(width, height))
    }

    fn link(owner: xproto::Window, takes_keys: bool) -> OverlayLink {
        OverlayLink {
            owner,
            window: Bounds::default(),
            owner_area: Bounds::default(),
            framed: Framing::default(),
            takes_keys,
            region: Vec::new(),
            content: Vec::new(),
            composited: true,
            shown: false,
            catcher: None,
            owner_version: 0,
            watching: false,
        }
    }

    #[test]
    fn an_owner_on_its_monitor_is_covered_exactly() {
        let placed = placement(rect(100, 80, 900, 600), &[rect(0, 0, 1280, 800)]);
        assert_eq!(placed.window, rect(100, 80, 900, 600));
        assert_eq!(placed.offset, point(0, 0));
    }

    #[test]
    fn an_owner_past_the_monitor_edge_is_clipped_and_drawn_with_an_offset() {
        let placed = placement(rect(-200, -30, 900, 600), &[rect(0, 0, 1280, 800)]);
        assert_eq!(placed.window, rect(0, 0, 700, 570));
        assert_eq!(placed.offset, point(-200, -30));
    }

    #[test]
    fn the_monitor_holding_most_of_the_owner_wins() {
        let monitors = [rect(0, 0, 1000, 800), rect(1000, 0, 1000, 800)];
        let placed = placement(rect(900, 0, 600, 400), &monitors);
        assert_eq!(placed.window, rect(1000, 0, 500, 400));
        assert_eq!(placed.offset, point(-100, 0));
    }

    #[test]
    fn an_owner_on_no_monitor_is_covered_unclipped() {
        let placed = placement(rect(5000, 0, 600, 400), &[rect(0, 0, 1000, 800)]);
        assert_eq!(placed.window, rect(5000, 0, 600, 400));
    }

    #[test]
    fn the_owner_bounds_are_logical() {
        let placed = placement(rect(-20, 0, 400, 300), &[rect(0, 0, 1000, 800)]);
        let bounds = placed.owner_bounds(size(400, 300), 2.0);
        assert_eq!(bounds.origin, point(px(-10.0), px(0.0)));
        assert_eq!(bounds.size, size(px(200.0), px(150.0)));
    }

    #[test]
    fn device_rects_cover_fractional_bounds() {
        let region = [Bounds::new(point(px(1.25), px(2.5)), size(px(10.0), px(3.3)))];
        let rects = device_rects(&region, 2.0);
        assert_eq!(
            (rects[0].x, rects[0].y, rects[0].width, rects[0].height),
            (2, 5, 21, 7)
        );
    }

    #[test]
    fn keys_go_to_an_overlay_only_while_it_takes_them() {
        let resting = link(1, false);
        assert_eq!(key_target(1, [(2, &resting)]), 1);
        let taking = link(1, true);
        assert_eq!(key_target(1, [(2, &taking)]), 2);
        assert_eq!(key_target(3, [(2, &taking)]), 3, "another window keeps its keys");
    }

    #[test]
    fn focus_moving_within_a_pair_keeps_the_owner_active() {
        let owner_of = |window| (window == 2).then_some(1);
        assert!(stays_in_pair(1, 2, owner_of), "owner to its overlay");
        assert!(stays_in_pair(2, 1, owner_of), "overlay back to its owner");
        assert!(!stays_in_pair(1, 3, owner_of), "to another window");
        assert!(!stays_in_pair(2, 3, owner_of), "overlay to another window");
    }

    #[test]
    fn only_the_input_region_holds_the_pointer() {
        let mut held = link(1, false);
        held.window = rect(100, 50, 400, 300);
        held.region = vec![xproto::Rectangle {
            x: 10,
            y: 20,
            width: 30,
            height: 40,
        }];
        assert!(holds_pointer(&held, point(110, 70)), "the region's corner");
        assert!(!holds_pointer(&held, point(140, 70)), "just past its right edge");
        assert!(!holds_pointer(&held, point(10, 20)), "root coordinates are offset");
        held.region.clear();
        assert!(!holds_pointer(&held, point(110, 70)), "an empty region takes nothing");
    }

    fn rectangle(x: i16, y: i16, width: u16, height: u16) -> xproto::Rectangle {
        xproto::Rectangle { x, y, width, height }
    }

    fn plain(rects: &[xproto::Rectangle]) -> Vec<(i16, i16, u16, u16)> {
        rects.iter().map(|r| (r.x, r.y, r.width, r.height)).collect()
    }

    #[test]
    fn an_empty_shape_is_a_rectangle_outside_the_window() {
        let menu = rectangle(10, 20, 100, 50);
        assert_eq!(plain(&shape_rects(&[])), plain(&[NOTHING]));
        assert_eq!(plain(&shape_rects(&[menu])), plain(&[menu]));
        assert!(NOTHING.x < 0 && NOTHING.y < 0, "outside the window");
    }

    #[test]
    fn only_an_uncomposited_overlay_is_cut_to_its_content() {
        let menu = rectangle(10, 20, 100, 50);
        assert!(bounding(&[menu], true).is_none());
        assert_eq!(bounding(&[menu], false).map(|r| plain(&r)), Some(plain(&[menu])));
        assert_eq!(bounding(&[], false).map(|r| plain(&r)), Some(plain(&[NOTHING])));
    }

    #[test]
    fn an_overlay_asks_for_no_decorations() {
        assert_eq!(UNDECORATED[0] & MWM_HINTS_DECORATIONS, MWM_HINTS_DECORATIONS, "flags");
        assert_eq!(UNDECORATED[2], 0, "decorations: none");
    }

    #[test]
    fn the_compositor_selection_is_named_after_the_screen_number() {
        assert_eq!(compositor_selection(0), "_NET_WM_CM_S0");
    }

    #[test]
    fn only_a_shown_uncomposited_overlay_needs_a_catcher() {
        let mut overlay = link(1, false);
        assert!(!needs_catcher(&overlay), "hidden");
        overlay.shown = true;
        assert!(!needs_catcher(&overlay), "composited");
        overlay.composited = false;
        assert!(needs_catcher(&overlay));
    }

    #[test]
    fn the_catcher_region_is_the_input_region_in_owner_coordinates() {
        let region = [rectangle(10, 20, 100, 50)];
        assert_eq!(plain(&in_owner(&region, point(4, 6))), plain(&[rectangle(6, 14, 100, 50)]));
    }

    fn configure(synthetic: bool, (x, y): (i16, i16)) -> xproto::ConfigureNotifyEvent {
        xproto::ConfigureNotifyEvent {
            response_type: if synthetic { SYNTHETIC | 22 } else { 22 },
            sequence: 0,
            event: 1,
            window: 1,
            above_sibling: 0,
            x,
            y,
            width: 640,
            height: 480,
            border_width: 0,
            override_redirect: false,
        }
    }

    #[test]
    fn a_framed_window_moves_only_on_the_window_managers_synthetic_configure() {
        let known = rect(100, 50, 400, 300);
        let real = configured(known, &configure(false, (0, 0)), true);
        assert_eq!(real, rect(100, 50, 640, 480), "frame-relative position ignored");
        let synthetic = configured(known, &configure(true, (300, 200)), true);
        assert_eq!(synthetic, rect(300, 200, 640, 480));
        let unframed = configured(known, &configure(false, (7, 9)), false);
        assert_eq!(unframed, rect(7, 9, 640, 480), "a child of the root reports root coordinates");
    }

    #[test]
    fn the_owner_bounds_follow_where_the_overlay_really_is() {
        let mut overlay = link(1, false);
        overlay.owner_area = rect(100, 80, 400, 300);
        overlay.window = rect(100, 80, 400, 300);
        assert_eq!(overlay.owner_bounds(1.0).origin, point(px(0.0), px(0.0)));
        overlay.window = rect(90, 80, 400, 300);
        assert_eq!(overlay.owner_bounds(1.0).origin, point(px(10.0), px(0.0)));
    }

    #[test]
    fn a_release_goes_where_its_press_went() {
        assert_eq!(release_target(Some(7), 9), 7, "the overlay let go in between");
        assert_eq!(release_target(None, 9), 9, "a press before the window existed");
    }
}
