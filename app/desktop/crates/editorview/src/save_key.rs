use gpui::actions;

actions!(editorview, [Save]);

pub(crate) const CONTEXT: &str = "CrowbarEditor";
// PLATFORM: macos saves with command, and linux uses control for the same chord.
#[cfg(target_os = "macos")]
pub(crate) const CHORD: &str = "cmd-s";
#[cfg(not(target_os = "macos"))]
pub(crate) const CHORD: &str = "ctrl-s";
