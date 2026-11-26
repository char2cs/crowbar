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
    /// The connection that waits for focus changes broke.
    #[cfg(target_os = "linux")]
    #[error("cannot wait for the X keyboard focus to move: {0}")]
    FocusWait(#[source] x11rb::errors::ConnectionError),
    /// The X server would not make or name the window that proxies a window's drags.
    #[cfg(target_os = "linux")]
    #[error("cannot proxy the drags of an X window: {0}")]
    Proxy(#[source] x11rb::errors::ReplyOrIdError),
    /// A thread of the platform layer could not start.
    #[error("cannot start a platform thread: {0}")]
    Thread(#[source] std::io::Error),
    /// The X server would not tell its keyboard mapping.
    #[cfg(target_os = "linux")]
    #[error("cannot read the X keyboard mapping: {0}")]
    KeyMap(#[source] x11rb::errors::ReplyError),
    /// The X server would not place, shape or describe a float window.
    #[cfg(target_os = "linux")]
    #[error("cannot place a float window: {0}")]
    Float(#[from] x11rb::errors::ReplyError),
    /// The X server would not cut a hole out of a web view's window.
    #[cfg(target_os = "linux")]
    #[error("cannot shape the web view window {window:#x}: {source}")]
    Cutout {
        /// The window to shape.
        window: u32,
        /// The failure.
        #[source]
        source: x11rb::errors::ReplyError,
    },
    /// A float window or its owner is not an X window.
    #[cfg(target_os = "linux")]
    #[error("the window is not an X window")]
    ForeignWindow,
    /// The connection that places float windows broke.
    #[cfg(target_os = "linux")]
    #[error("lost the X server that places float windows: {0}")]
    FloatConnection(#[from] x11rb::errors::ConnectionError),
}
