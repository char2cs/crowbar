use std::path::PathBuf;

use store::LegacyDrafts;

use crate::app_options::AppOptions;
use crate::editor_panes::DRAFTS_FILE;
use crate::host_builder::WEBVIEW_DIR;

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
        Err(error) => return warn_not_imported(&error),
    };
    let file = editor::DiskDraftFile::at(state.join(DRAFTS_FILE));
    let (mut drafts, _) = editor::Drafts::open(Box::new(file));
    match drafts.adopt(taken) {
        Ok(kept) => report_kept(kept),
        Err(error) => warn_not_written(&error),
    }
}

fn warn_not_imported(error: &store::Error) {
    tracing::warn!(%error, "drafts of the previous editor were not imported");
}

fn warn_not_written(error: &std::io::Error) {
    tracing::warn!(%error, "imported drafts were not written");
}

fn report_kept(kept: usize) {
    if kept > 0 {
        tracing::info!(kept, "unsaved drafts of the previous editor were imported");
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
