//! Parking: a browser tab that is not on screen stays shown and keeps its size,
//! and is moved out of the window instead. WebKit decides a page's visibility
//! from its view being hidden or its window being offscreen, not from where the
//! view sits inside the window, so a parked page keeps running at full speed
//! and its layout viewport does not change. Nothing outside the window is
//! painted or receives clicks.
use std::collections::HashSet;
use std::sync::Mutex;

/// Gap between a parked tab's right edge and the window's left edge.
const MARGIN: f64 = 64.0;

/// Where a parked tab of `width` logical points sits: wholly left of the window.
/// Negative offsets stay within what AppKit, GTK and WebView2 accept, unlike an
/// arbitrarily distant one (X11 windows are limited to 16-bit coordinates).
pub fn origin(width: f64) -> (f64, f64) {
    (-(width.max(0.0) + MARGIN), 0.0)
}

/// Labels of the tabs that are parked. `browser_set_bounds` consults it so a
/// resize report that arrives while a tab is parked cannot bring it back.
#[derive(Default)]
pub struct Parked(Mutex<HashSet<String>>);

impl Parked {
    pub fn park(&self, label: &str) {
        self.set().insert(label.to_owned());
    }

    pub fn unpark(&self, label: &str) {
        self.set().remove(label);
    }

    pub fn is_parked(&self, label: &str) -> bool {
        self.set().contains(label)
    }

    fn set(&self) -> std::sync::MutexGuard<'_, HashSet<String>> {
        self.0
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_parked_tab_is_wholly_left_of_the_window() {
        for width in [0.0, 1.0, 400.0, 1280.0, 4000.0] {
            let (x, y) = origin(width);
            assert!(x + width < 0.0, "width {width} would reach the window");
            assert_eq!(y, 0.0);
        }
    }

    #[test]
    fn a_negative_width_never_moves_the_tab_into_the_window() {
        let (x, _) = origin(-500.0);
        assert!(x < 0.0);
    }

    #[test]
    fn parking_is_tracked_per_tab_and_undone_by_unparking() {
        let parked = Parked::default();
        parked.park("browser-a");
        assert!(parked.is_parked("browser-a"));
        assert!(!parked.is_parked("browser-b"));
        parked.unpark("browser-a");
        assert!(!parked.is_parked("browser-a"));
    }
}
