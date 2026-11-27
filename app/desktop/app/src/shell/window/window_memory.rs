use std::path::Path;

use crate::shell::window::traffic_lights;
use gpui::{
    App, Bounds, Pixels, SharedString, TitlebarOptions, Window, WindowBackgroundAppearance,
    WindowBounds, WindowOptions, point, px, size,
};
use windowstate::{DEFAULT_SIZE, Placement, Rect, WindowState, WindowStore};

pub(crate) const WINDOW_TITLE: &str = "Crowbar";
pub(crate) const APP_ID: &str = "Crowbar";

pub(crate) struct WindowMemory {
    store: WindowStore,
}

impl WindowMemory {
    pub(crate) fn new(file: &Path) -> Self {
        Self {
            store: WindowStore::new(file),
        }
    }

    pub(crate) fn options(
        &self,
        cx: &mut App,
    ) -> WindowOptions {
        let displays: Vec<Rect> = cx
            .displays()
            .iter()
            .map(|display| to_rect(display.bounds()))
            .collect();
        let placement = self.placement(&displays).unwrap_or_else(|| first_run(cx));
        WindowOptions {
            window_bounds: Some(window_bounds(&placement)),
            titlebar: Some(titlebar()),
            app_id: Some(APP_ID.to_owned()),
            window_background: background(),
            app_owns_titlebar_drag: true,
            ..WindowOptions::default()
        }
    }

    pub(crate) fn remember_on_close(
        &self,
        window: &Window,
        cx: &App,
        allowed: impl Fn(&mut Window, &mut App) -> bool + 'static,
    ) {
        let store = self.store.clone();
        window.on_window_should_close(cx, move |window, cx| {
            save(&store, window.window_bounds());
            allowed(window, cx)
        });
    }

    fn placement(
        &self,
        displays: &[Rect],
    ) -> Option<Placement> {
        match self.store.load() {
            Ok(saved) => saved.map(|state| state.placement(displays)),
            Err(error) => {
                tracing::warn!(%error, "the saved window placement is unusable");
                None
            },
        }
    }
}

fn first_run(cx: &mut App) -> Placement {
    let bounds = Bounds::centered(None, size(px(DEFAULT_SIZE.0), px(DEFAULT_SIZE.1)), cx);
    Placement {
        bounds: to_rect(bounds),
        maximized: false,
    }
}

fn to_rect(bounds: Bounds<Pixels>) -> Rect {
    Rect {
        x: f32::from(bounds.origin.x),
        y: f32::from(bounds.origin.y),
        width: f32::from(bounds.size.width),
        height: f32::from(bounds.size.height),
    }
}

fn from_rect(rect: &Rect) -> Bounds<Pixels> {
    Bounds::new(
        point(px(rect.x), px(rect.y)),
        size(px(rect.width), px(rect.height)),
    )
}

fn window_bounds(placement: &Placement) -> WindowBounds {
    let bounds = from_rect(&placement.bounds);
    if placement.maximized {
        return WindowBounds::Maximized(bounds);
    }
    WindowBounds::Windowed(bounds)
}

// PLATFORM: macos the blur behind the window shows only through a see-through one.
const fn background() -> WindowBackgroundAppearance {
    if cfg!(target_os = "macos") {
        return WindowBackgroundAppearance::Transparent;
    }
    WindowBackgroundAppearance::Opaque
}

const MAC_OVERLAY: bool = cfg!(target_os = "macos");
// PLATFORM: macos overlays the title bar; Linux keeps the system frame.
fn titlebar() -> TitlebarOptions {
    TitlebarOptions {
        title: Some(SharedString::from(WINDOW_TITLE)),
        appears_transparent: MAC_OVERLAY,
        traffic_light_position: MAC_OVERLAY.then(traffic_lights::position),
    }
}

fn save(
    store: &WindowStore,
    bounds: WindowBounds,
) {
    if let Err(error) = store.save(&state_of(bounds)) {
        tracing::warn!(%error, "the window placement was not saved");
    }
}

fn state_of(bounds: WindowBounds) -> WindowState {
    WindowState {
        bounds: to_rect(bounds.get_bounds()),
        maximized: matches!(bounds, WindowBounds::Maximized(_)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const BOUNDS: Bounds<Pixels> = Bounds {
        origin: gpui::Point {
            x: px(10.0),
            y: px(20.0),
        },
        size: gpui::Size {
            width: px(900.0),
            height: px(600.0),
        },
    };

    #[gpui::test]
    fn the_shell_moves_its_window_itself_so_a_dragged_tab_stays_a_tab(
        cx: &mut gpui::TestAppContext
    ) {
        let memory = WindowMemory::new(Path::new("/nowhere/window.json"));
        assert!(cx.update(|cx| memory.options(cx).app_owns_titlebar_drag));
    }

    #[gpui::test]
    fn only_a_macos_window_is_see_through(cx: &mut gpui::TestAppContext) {
        let memory = WindowMemory::new(Path::new("/nowhere/window.json"));
        let background = cx.update(|cx| memory.options(cx).window_background);
        assert_eq!(background, seen_here());
    }

    // PLATFORM: macos blurs what is behind the sidebar; X11 and Wayland windows are opaque.
    fn seen_here() -> WindowBackgroundAppearance {
        if cfg!(target_os = "macos") {
            return WindowBackgroundAppearance::Transparent;
        }
        WindowBackgroundAppearance::Opaque
    }

    #[test]
    fn a_closed_window_is_remembered_with_its_restore_size() {
        let cases = [
            (WindowBounds::Windowed(BOUNDS), false),
            (WindowBounds::Maximized(BOUNDS), true),
        ];
        for (bounds, maximized) in cases {
            let state = state_of(bounds);
            assert_eq!(state.maximized, maximized);
            assert_eq!(from_rect(&state.bounds), BOUNDS);
        }
    }

    #[test]
    fn a_placement_opens_as_windowed_or_maximized() {
        let rect = to_rect(BOUNDS);
        let windowed = window_bounds(&Placement {
            bounds: rect,
            maximized: false,
        });
        let maximized = window_bounds(&Placement {
            bounds: rect,
            maximized: true,
        });
        assert_eq!(windowed, WindowBounds::Windowed(BOUNDS));
        assert_eq!(maximized, WindowBounds::Maximized(BOUNDS));
    }

    #[gpui::test]
    fn the_window_may_shrink_below_the_sidebar_drawer_breakpoint(cx: &mut gpui::TestAppContext) {
        let memory = WindowMemory::new(Path::new("/nonexistent/crowbar/window.json"));
        let least = cx.update(|cx| memory.options(cx).window_min_size);
        assert_eq!(least, None, "develop's window declares no minimum size");
    }

    #[gpui::test]
    fn the_window_is_classed_crowbar_for_the_desktop_file(cx: &mut gpui::TestAppContext) {
        let memory = WindowMemory::new(Path::new("/nonexistent/crowbar/window.json"));
        let app_id = cx.update(|cx| memory.options(cx).app_id);
        assert_eq!(app_id.as_deref(), Some("Crowbar"));
    }
}
