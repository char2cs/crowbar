use std::path::Path;

use gpui::{
    App, Bounds, Pixels, SharedString, TitlebarOptions, Window, WindowBounds, WindowOptions, point,
    px, size,
};
use window_state::{DEFAULT_SIZE, MIN_SIZE, Placement, Rect, WindowState, WindowStore};

pub(crate) const WINDOW_TITLE: &str = "Crowbar";

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
            window_min_size: Some(size(px(MIN_SIZE.0), px(MIN_SIZE.1))),
            titlebar: Some(titlebar()),
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

const MAC_OVERLAY: bool = cfg!(target_os = "macos");
const TRAFFIC_LIGHTS: (f32, f32) = (12.0, 23.0);

// PLATFORM: only macOS overlays the title bar; Linux keeps the system frame.
fn titlebar() -> TitlebarOptions {
    TitlebarOptions {
        title: Some(SharedString::from(WINDOW_TITLE)),
        appears_transparent: MAC_OVERLAY,
        traffic_light_position: MAC_OVERLAY
            .then(|| point(px(TRAFFIC_LIGHTS.0), px(TRAFFIC_LIGHTS.1))),
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
}
