//! What the shell tells the user about its backend: develop's connection toast, the
//! supervisor's restart and wedge toasts, and the crash-loop panel.

mod backend_status;
mod backend_watch;
mod connection_notice;
mod crash_panel;
pub(crate) mod daemon_notice;
mod notice_act;
mod projects_failed;
mod warned;

pub(crate) use backend_status::BackendStatus;
pub(crate) use backend_status::backend_status;
pub(crate) use backend_watch::BackendWatch;
pub(crate) use connection_notice::ConnectionNotice;
pub(crate) use crash_panel::CrashPanel;
pub(crate) use notice_act::NoticeAct;
pub(crate) use projects_failed::ProjectsFailed;
