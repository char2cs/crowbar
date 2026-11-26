//! The Files panel's behaviour: actions, clipboard, dialogs, drags, keys, filtering and folder
//! chains.

mod copy_note;
mod file_action;
mod file_clipboard;
mod file_dialog;
mod file_filter;
mod file_move;
mod file_steps;
mod folder_chain;
mod menu_scope;

pub use copy_note::CopyNote;
pub use file_action::FileAction;
pub use file_clipboard::FileClipboard;
pub use file_dialog::FileDialog;
pub use file_dialog::file_properties::FileProperties;
pub use file_move::FileMove;
pub use file_move::drop_spot::DropSpot;
pub use file_move::file_drop::FileDrop;
pub use file_steps::file_key::FileKey;
pub use menu_scope::MenuScope;
pub use menu_scope::shell_answers::ShellAnswers;
