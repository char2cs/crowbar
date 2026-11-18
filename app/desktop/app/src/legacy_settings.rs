use std::path::PathBuf;

use store::LegacyImport;

use crate::app_options::AppOptions;

pub(crate) fn import_old_settings(
    user_home: Option<PathBuf>,
    options: &AppOptions,
) {
    let Some(user_home) = user_home else {
        return;
    };
    let settings = options.state_dir().join("settings.json");
    match LegacyImport::discover(&user_home, settings).run() {
        Ok(0) => {},
        Ok(taken) => tracing::info!(taken, "settings of the previous app were imported"),
        Err(error) => tracing::warn!(%error, "settings of the previous app were not imported"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const ABSENT_HOME: &str = "/nonexistent-crowbar-home";

    #[test]
    fn a_home_without_the_old_app_changes_nothing() {
        let options =
            AppOptions::from_env(|key| (key == "CROWBAR_HOME").then(|| format!("{ABSENT_HOME}/h")));
        import_old_settings(Some(PathBuf::from(ABSENT_HOME)), &options);
        assert!(!options.state_dir().exists());
    }
}
