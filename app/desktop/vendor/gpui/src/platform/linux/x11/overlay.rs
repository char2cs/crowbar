//! Overlay windows (`WindowOptions::overlay_of`): where one goes over its owner and how the
//! owner's keyboard is lent to it. The window manager never focuses an overlay; the owner keeps
//! X focus and its key events are delivered to the overlay while the overlay takes keys.

use x11rb::protocol::randr::ConnectionExt as _;
use x11rb::protocol::xproto::{self, ConnectionExt as _};
use x11rb::xcb_ffi::XCBConnection;

use crate::{Bounds, Pixels, Point, Size, point, px, size};

/// The overlay's side of an owner/overlay pair, kept in the overlay's window state.
#[derive(Debug, Clone)]
pub(crate) struct OverlayLink {
    pub(crate) owner: xproto::Window,
    /// The overlay's rectangle in root coordinates, in device pixels.
    pub(crate) window: Bounds<i32>,
    /// The owner's content area in the overlay's coordinates, in logical pixels.
    pub(crate) owner_bounds: Bounds<Pixels>,
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
}

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
            owner_bounds: Bounds::default(),
            takes_keys,
            region: Vec::new(),
            content: Vec::new(),
            composited: true,
            shown: false,
            catcher: None,
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
}
