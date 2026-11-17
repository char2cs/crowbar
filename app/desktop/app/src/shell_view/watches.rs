#[derive(Default)]
pub(super) struct Watches {
    pub(super) keymap: Option<settings_ui::Subscription>,
    pub(super) appearance: Option<settings_ui::Subscription>,
    pub(super) pane_settings: Option<settings_ui::Subscription>,
}
