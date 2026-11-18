//! What a view must be able to do for the input widgets it owns: run an [`Intent`].

use gpui::{Context, Window};

use crate::intent::Intent;

pub(crate) trait Runner: Sized + 'static {
    fn run(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    );
}
