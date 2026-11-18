//! The remote folder picker of the Crowbar shell.
//!
//! [`RemoteFolderPicker`] shows the folders of a paired host's own disk, listed as they are opened
//! through a [`store::FolderBrowse`], where no native dialog can reach. It emits
//! [`PickerEvent::Chosen`] with the absolute path of the folder picked, or [`PickerEvent::Cancelled`].

mod folder_note;
mod folder_reader;
mod folder_requests;
mod folder_row;
mod folder_rows;
mod folder_source;
mod muted;
mod picker_actions;
mod picker_event;
mod picker_panels;
mod picker_widgets;
mod remote_folder_picker;
mod row_look;
mod shown;
mod test_support;
mod tree_row;

pub use picker_event::PickerEvent;
pub use remote_folder_picker::RemoteFolderPicker;
