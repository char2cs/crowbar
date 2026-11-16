use std::path::PathBuf;

use shell_data::LegacyImport;

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

    #[test]
    fn a_home_without_the_old_app_changes_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let options = AppOptions::from_env(|key| {
            (key == "CROWBAR_HOME").then(|| dir.path().join("h").display().to_string())
        });
        import_old_settings(Some(dir.path().to_path_buf()), &options);
        assert!(!options.state_dir().exists());
    }
}
