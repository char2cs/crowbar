use std::path::PathBuf;

use store::LegacyDrafts;

use crate::app_options::AppOptions;
use crate::logged::emit;
use crate::panes::editor::editor_panes::DRAFTS_FILE;
use crate::panes::kinds::host_builder::WEBVIEW_DIR;

mod import_event;
pub(crate) use import_event::ImportEvent;

pub(crate) fn import_old_drafts(
    user_home: Option<PathBuf>,
    options: &AppOptions,
) {
    let Some(user_home) = user_home else {
        return;
    };
    let state = options.state_dir();
    let legacy = LegacyDrafts::discover(&user_home, (&state.join(WEBVIEW_DIR), &state));
    let taken = match legacy.take() {
        Ok(taken) => taken,
        Err(error) => return emit(&ImportEvent::DraftsNotImported { error: &error }),
    };
    let file = editor::DiskDraftFile::at(state.join(DRAFTS_FILE));
    let (mut drafts, _) = editor::Drafts::open(Box::new(file));
    let kept = match drafts.adopt(taken) {
        Ok(kept) => kept,
        Err(error) => return emit(&ImportEvent::DraftsNotWritten { error: &error }),
    };
    if kept > 0 {
        emit(&ImportEvent::DraftsImported { kept });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const ABSENT_HOME: &str = "/nonexistent-crowbar-home";

    #[test]
    fn a_home_without_the_old_editor_changes_nothing() {
        let options =
            AppOptions::from_env(|key| (key == "CROWBAR_HOME").then(|| format!("{ABSENT_HOME}/h")));
        import_old_drafts(Some(PathBuf::from(ABSENT_HOME)), &options);
        assert!(!options.state_dir().exists());
    }
}
