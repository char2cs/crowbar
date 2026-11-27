use std::path::PathBuf;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("refusing to open {target:?}: {reason}")]
    Refused {
        target: String,
        reason: &'static str,
    },
    #[error("a page clip is made on the main thread only")]
    Clip,
    #[error("cannot blur behind the window: {0}")]
    Frost(&'static str),
    #[cfg(target_os = "macos")]
    #[error("AppKit refused the blur effect: {0}")]
    Vibrancy(#[source] window_vibrancy::Error),
    #[error("cannot resolve {path}: {source}")]
    Unresolved {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("cannot start {program}: {source}")]
    Spawn {
        program: &'static str,
        #[source]
        source: std::io::Error,
    },
    #[cfg(target_os = "linux")]
    #[error("cannot connect to the X server: {0}")]
    X11Connect(#[from] x11rb::errors::ConnectError),
    #[cfg(target_os = "linux")]
    #[error("cannot give the keyboard to X window {window:#x}: {source}")]
    Focus {
        window: u32,
        #[source]
        source: x11rb::errors::ReplyError,
    },
    #[cfg(target_os = "linux")]
    #[error("cannot follow the keyboard focus of X window {window:#x}: {source}")]
    Follow {
        window: u32,
        #[source]
        source: x11rb::errors::ReplyError,
    },
    #[cfg(target_os = "linux")]
    #[error("cannot wait for the X keyboard focus to move: {0}")]
    FocusWait(#[source] x11rb::errors::ConnectionError),
    #[cfg(target_os = "linux")]
    #[error("cannot proxy the drags of an X window: {0}")]
    Proxy(#[source] x11rb::errors::ReplyOrIdError),
    #[error("cannot start a platform thread: {0}")]
    Thread(#[source] std::io::Error),
    #[cfg(target_os = "linux")]
    #[error("cannot read the X keyboard mapping: {0}")]
    KeyMap(#[source] x11rb::errors::ReplyError),
    #[cfg(target_os = "linux")]
    #[error("cannot place a float window: {0}")]
    Float(#[from] x11rb::errors::ReplyError),
    #[cfg(target_os = "linux")]
    #[error("cannot shape the web view window {window:#x}: {source}")]
    Cutout {
        window: u32,
        #[source]
        source: x11rb::errors::ReplyError,
    },
    #[cfg(target_os = "linux")]
    #[error("the window is not an X window")]
    ForeignWindow,
    #[cfg(target_os = "linux")]
    #[error("lost the X server that places float windows: {0}")]
    FloatConnection(#[from] x11rb::errors::ConnectionError),
}
