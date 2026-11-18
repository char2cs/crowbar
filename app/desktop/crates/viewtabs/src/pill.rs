//! One tab: its view, members, member kinds, look and shell.

mod member_kind;
mod member_view;
mod shell_look;
mod tab_shell;
mod tab_view;

pub use member_kind::MemberKind;
pub use member_view::MemberView;
pub(crate) use shell_look::ShellLook;
pub(crate) use tab_shell::{CloseAction, TabShell};
pub use tab_view::TabView;
