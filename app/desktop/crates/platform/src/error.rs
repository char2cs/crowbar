use std::path::PathBuf;

/// Why a desktop service could not run.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The target is not something the shell may hand to the system.
    #[error("refusing to open {target:?}: {reason}")]
    Refused {
        /// What was asked for.
        target: String,
        /// Why it was refused.
        reason: &'static str,
    },
    /// The path to reveal does not exist or cannot be resolved.
    #[error("cannot resolve {path}: {source}")]
    Unresolved {
        /// The path.
        path: PathBuf,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
    /// The desktop's launcher could not be started.
    #[error("cannot start {program}: {source}")]
    Spawn {
        /// The launcher program.
        program: &'static str,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
    /// No X server answered on `DISPLAY`.
    #[cfg(target_os = "linux")]
    #[error("cannot connect to the X server: {0}")]
    X11Connect(#[from] x11rb::errors::ConnectError),
    /// The X server did not move the keyboard to a window.
    #[cfg(target_os = "linux")]
    #[error("cannot give the keyboard to X window {window:#x}: {source}")]
    Focus {
        /// The window that was to take the keyboard.
        window: u32,
        /// The failure.
        #[source]
        source: x11rb::errors::ReplyError,
    },
    /// The X server would not report a window's keyboard focus.
    #[cfg(target_os = "linux")]
    #[error("cannot follow the keyboard focus of X window {window:#x}: {source}")]
    Follow {
        /// The window to follow.
        window: u32,
        /// The failure.
        #[source]
        source: x11rb::errors::ReplyError,
    },
    /// The X server would not tell its keyboard mapping.
    #[cfg(target_os = "linux")]
    #[error("cannot read the X keyboard mapping: {0}")]
    KeyMap(#[source] x11rb::errors::ReplyError),
}
