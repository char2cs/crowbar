//! The process environment changes that make this process an X11 client.

const WAYLAND_DISPLAY: &str = "WAYLAND_DISPLAY";
const GDK_BACKEND: &str = "GDK_BACKEND";
const GDK_BACKEND_X11: &str = "x11";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum EnvChange {
    Remove(&'static str),
    Set(&'static str, &'static str),
}

impl EnvChange {
    #[expect(
        unsafe_code,
        reason = "editing the environment is unsafe in edition 2024; callers run before any thread starts"
    )]
    pub(crate) fn apply(self) {
        match self {
            // SAFETY: the shell applies this in `main` before it starts any other thread.
            Self::Remove(key) => unsafe { std::env::remove_var(key) },
            // SAFETY: the shell applies this in `main` before it starts any other thread.
            Self::Set(key, value) => unsafe { std::env::set_var(key, value) },
        }
    }
}

/// Makes this process an X11 client: drops `WAYLAND_DISPLAY` and pins GTK to its X11 backend.
///
/// Web views are embedded as X11 children of the GPUI window, so both GPUI and GTK must talk to
/// the X server (`XWayland` on a Wayland session). Call it first in `main`, before any other
/// thread starts: it edits the process environment.
pub fn force_x11() {
    let changes = [
        EnvChange::Remove(WAYLAND_DISPLAY),
        EnvChange::Set(GDK_BACKEND, GDK_BACKEND_X11),
    ];
    changes.into_iter().for_each(EnvChange::apply);
}

#[cfg(test)]
mod tests {
    use super::*;

    const PROBE: &str = "CROWBAR_ENV_CHANGE_PROBE";

    #[test]
    fn a_change_sets_and_removes_a_variable() {
        EnvChange::Set(PROBE, "on").apply();
        assert_eq!(std::env::var(PROBE).as_deref(), Ok("on"), "set");
        EnvChange::Remove(PROBE).apply();
        assert!(std::env::var_os(PROBE).is_none(), "removed");
    }
}
