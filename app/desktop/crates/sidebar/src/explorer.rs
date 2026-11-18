//! The Files panel's behaviour: actions, clipboard, dialogs, keys, filtering and folder chains.

mod file_action;
mod file_clipboard;
mod file_dialog;
mod file_filter;
mod file_key;
mod file_properties;
mod file_steps;
mod folder_chain;
mod hidden_pattern;
mod menu_scope;
mod shell_answers;
mod shifts;

pub use file_action::FileAction;
pub use file_clipboard::FileClipboard;
pub use file_dialog::FileDialog;
pub use file_key::FileKey;
pub use file_properties::FileProperties;
pub use menu_scope::MenuScope;
pub use shell_answers::ShellAnswers;
