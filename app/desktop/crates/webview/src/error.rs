use raw_window_handle::HandleError;
use thiserror::Error;

use crate::view::Id;

#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("unsupported parent window handle: {0}")]
    UnsupportedParent(&'static str),
    #[error("parent window handle unavailable: {0}")]
    Handle(#[from] HandleError),
    #[error("platform thread failed to start: {0}")]
    PlatformThread(String),
    #[error("platform thread has stopped")]
    HostStopped,
    #[error("the platform thread did not answer in time")]
    TimedOut,
    #[error("must be called on the main thread")]
    NotMainThread,
    #[error("no web view {0}")]
    UnknownWebView(Id),
    #[error("web view engine error: {0}")]
    Engine(#[from] wry::Error),
    #[error("operating system error: {0}")]
    Os(#[from] platform::Error),
}

pub type Result<T> = std::result::Result<T, Error>;
