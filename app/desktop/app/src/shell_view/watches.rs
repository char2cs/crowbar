#[derive(Default)]
pub(super) struct Watches {
    pub(super) keymap: Option<settings::Subscription>,
    pub(super) appearance: Option<settings::Subscription>,
    pub(super) pane_settings: Option<settings::Subscription>,
}
