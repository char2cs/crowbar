//! The native settings dialog of the Crowbar shell: [`SettingsModal`], the rail, pages, rows,
//! controls, record lists and shortcut capture, drawn over the gpui-free `settings` crate.

mod frame;
mod intent;
mod modal;
mod settings_modal;
mod test_support;

pub use modal::pages::shortcuts::capture::bindings;
pub use settings_modal::{ModalConfig, SettingsModal};
