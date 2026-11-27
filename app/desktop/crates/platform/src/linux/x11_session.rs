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
