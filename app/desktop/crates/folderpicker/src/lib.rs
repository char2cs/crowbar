//! The remote folder picker of the Crowbar shell.
//!
//! [`RemoteFolderPicker`] shows the folders of a paired host's own disk, listed as they are opened
//! through a [`store::FolderBrowse`], where no native dialog can reach. It emits
//! [`PickerEvent::Chosen`] with the absolute path of the folder picked, or [`PickerEvent::Cancelled`].

mod folder_source;
mod look;
mod picker_event;
mod remote_folder_picker;
mod test_support;
mod tree;

pub use picker_event::PickerEvent;
pub use remote_folder_picker::RemoteFolderPicker;
