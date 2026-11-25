#[derive(Default)]
pub(crate) struct Watches {
    pub(crate) keymap: Option<settings::Subscription>,
    pub(crate) appearance: Option<settings::Subscription>,
    pub(crate) pane_settings: Option<settings::Subscription>,
}
