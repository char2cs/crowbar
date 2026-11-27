use crate::shell::dialogs::prefill;
#[derive(Debug, Clone, Copy)]
pub(crate) struct PromptWords {
    pub(crate) dialog: &'static str,
    pub(crate) title: &'static str,
    pub(crate) label: &'static str,
    pub(crate) placeholder: &'static str,
    pub(crate) cancel_id: &'static str,
    pub(crate) confirm_id: &'static str,
    pub(crate) confirm: &'static str,
    pub(crate) prefill: prefill::Prefill,
}
