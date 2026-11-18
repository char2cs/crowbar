use super::env_change::EnvChange;

const WAYLAND_DISPLAY: &str = "WAYLAND_DISPLAY";
const GDK_BACKEND: &str = "GDK_BACKEND";
const GDK_BACKEND_X11: &str = "x11";

pub(crate) fn x11_changes() -> [EnvChange; 2] {
    [
        EnvChange::Remove(WAYLAND_DISPLAY),
        EnvChange::Set(GDK_BACKEND, GDK_BACKEND_X11),
    ]
}

/// Makes this process an X11 client: drops `WAYLAND_DISPLAY` and pins GTK to its X11 backend.
///
/// Web views are embedded as X11 children of the GPUI window, so both GPUI and GTK must talk to
/// the X server (`XWayland` on a Wayland session). Call it first in `main`, before any other
/// thread starts: it edits the process environment.
pub fn force_x11() {
    x11_changes().into_iter().for_each(EnvChange::apply);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_session_drops_wayland_and_pins_gtk_to_x11() {
        assert_eq!(
            x11_changes(),
            [
                EnvChange::Remove("WAYLAND_DISPLAY"),
                EnvChange::Set("GDK_BACKEND", "x11")
            ],
            "changes"
        );
    }
}
