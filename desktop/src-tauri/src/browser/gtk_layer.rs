//! Browser tab placement on GTK. Tauri packs every child webview into its window's vbox,
//! where a position is ignored and a tab splits the window with the app. Tabs move instead
//! to a pass-through `gtk::Fixed` laid over the app's webview, and are placed there by hand.
use gtk::prelude::*;

/// Names the overlay that holds the app's webview and the tab layer above it.
const LAYER: &str = "crowbar-browser-layer";

/// x, y, width, height in logical pixels, as the web side measures them.
pub type Frame = (f64, f64, f64, f64);

/// Moves `view`, just packed into its window's vbox beside the app's webview, onto the tab
/// layer at `frame`.
pub fn adopt(view: &gtk::Widget, frame: Frame) {
    let Some(vbox) = view.parent().and_then(|p| p.downcast::<gtk::Box>().ok()) else {
        return;
    };
    let Some(layer) = layer_in(&vbox, view) else {
        return;
    };
    vbox.remove(view);
    layer.put(view, 0, 0);
    place(view, frame);
}

/// Puts an adopted `view` at `frame`: a size request survives every later layout pass.
pub fn place(view: &gtk::Widget, (x, y, width, height): Frame) {
    let Some(layer) = view.parent().and_then(|p| p.downcast::<gtk::Fixed>().ok()) else {
        return;
    };
    layer.move_(view, x.round() as i32, y.round() as i32);
    view.set_size_request(
        width.round().max(1.0) as i32,
        height.round().max(1.0) as i32,
    );
}

/// The tab layer of `vbox`, made the first time by wrapping the app's webview (the other
/// one in the vbox) in an overlay in place.
fn layer_in(vbox: &gtk::Box, view: &gtk::Widget) -> Option<gtk::Fixed> {
    let children = vbox.children();
    if let Some(layer) = children.iter().find_map(existing_layer) {
        return Some(layer);
    }
    let app = children
        .iter()
        .find(|c| *c != view && c.is::<webkit2gtk::WebView>())?;
    let overlay = gtk::Overlay::new();
    overlay.set_widget_name(LAYER);
    let position = vbox.child_position(app);
    let (expand, fill, padding, _) = vbox.query_child_packing(app);
    vbox.remove(app);
    overlay.add(app);
    let layer = gtk::Fixed::new();
    overlay.add_overlay(&layer);
    // The layer itself takes no input; the tabs on it are windows of their own and do.
    overlay.set_overlay_pass_through(&layer, true);
    vbox.pack_start(&overlay, expand, fill, padding);
    vbox.reorder_child(&overlay, position);
    overlay.show_all();
    Some(layer)
}

fn existing_layer(child: &gtk::Widget) -> Option<gtk::Fixed> {
    if child.widget_name() != LAYER {
        return None;
    }
    let overlay = child.clone().downcast::<gtk::Overlay>().ok()?;
    overlay
        .children()
        .into_iter()
        .find_map(|c| c.downcast::<gtk::Fixed>().ok())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A window laid out as Tauri does on GTK: a menu, then the app's webview, in a vbox.
    /// GTK runs on one thread and needs a display (`xvfb-run cargo test` without one).
    fn app_window() -> (gtk::Box, gtk::Widget) {
        gtk::init().expect("GTK needs a display: run the tests under xvfb-run");
        let window = gtk::Window::new(gtk::WindowType::Toplevel);
        let vbox = gtk::Box::new(gtk::Orientation::Vertical, 0);
        let app: gtk::Widget = webkit2gtk::WebView::new().upcast();
        vbox.pack_start(&gtk::MenuBar::new(), false, false, 0);
        vbox.pack_start(&app, true, true, 0);
        window.add(&vbox);
        window.set_default_size(1200, 800);
        window.show_all();
        (vbox, app)
    }

    /// Runs the size negotiation GTK would otherwise leave to the next frame.
    fn allocation_after_layout(widget: &gtk::Widget) -> gtk::Allocation {
        while gtk::events_pending() {
            gtk::main_iteration();
        }
        if let Ok(window) = widget.toplevel().unwrap().downcast::<gtk::Container>() {
            window.check_resize();
        }
        widget.allocation()
    }

    #[test]
    fn tabs_sit_where_the_pane_asks_and_the_app_keeps_the_whole_window() {
        let (vbox, app) = app_window();
        let full = allocation_after_layout(&app);

        let mut tabs = Vec::new();
        for frame in [(600.0, 120.0, 500.0, 400.0), (10.4, 20.6, 300.0, 200.0)] {
            // What Tauri does with a new child webview on GTK: pack it into the vbox.
            let tab: gtk::Widget = webkit2gtk::WebView::new().upcast();
            vbox.pack_start(&tab, true, true, 0);
            tab.show();
            adopt(&tab, frame);
            tabs.push(tab);
        }

        assert_eq!(allocation_after_layout(&app), full, "the app kept its size");
        let first = tabs[0].allocation();
        assert_eq!(
            (first.x(), first.y(), first.width(), first.height()),
            (600, 120, 500, 400)
        );
        let second = tabs[1].allocation();
        assert_eq!(
            (second.x(), second.y(), second.width(), second.height()),
            (10, 21, 300, 200)
        );

        place(&tabs[0], (-600.0, 0.0, 500.0, 400.0));
        let moved = allocation_after_layout(&tabs[0]);
        assert_eq!(
            (moved.x(), moved.width()),
            (-600, 500),
            "parked off the window"
        );
        assert_eq!(
            vbox.children().len(),
            2,
            "the menu and one overlay, the app inside it"
        );
    }
}
