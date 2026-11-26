//! The Files panel's data: file rows, kinds, the access and source ports, view flags, density, icon theme and git status.

mod file_access;
mod file_kind;
mod file_row;
mod file_tree_view;
mod files_source;
mod folder_phase;
mod folder_source;
mod git_status;
mod path_text;

pub use file_access::FileAccess;
pub use file_kind::FileKind;
pub use file_row::FileRow;
pub use file_tree_view::FileTreeView;
pub use file_tree_view::file_density::FileDensity;
pub use file_tree_view::file_tree_flag::FileTreeFlag;
pub use file_tree_view::icon_theme::IconTheme;
pub use file_tree_view::indent_guides::IndentGuides;
pub use files_source::FilesSource;
pub use folder_phase::FolderPhase;
pub use folder_source::FolderSource;
pub use git_status::GitStatus;
pub use path_text::PathText;
