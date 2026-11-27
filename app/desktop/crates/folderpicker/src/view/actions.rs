use gpui::Context;
use store::BrowseEntry;

use crate::picker_event::PickerEvent;
use crate::view::View;

impl View {
    pub(crate) fn choose(
        &mut self,
        entry: &BrowseEntry,
        cx: &mut Context<'_, Self>,
    ) {
        self.selected = Some(entry.clone());
        self.expanded.insert(entry.path.clone());
        cx.notify();
    }

    pub(crate) fn toggle(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.expanded.remove(path) {
            self.expanded.insert(path.to_owned());
        }
        cx.notify();
    }

    pub(crate) fn confirm(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chosen) = self.selected.as_ref().filter(|_| self.source.can_ask()) else {
            return;
        };
        cx.emit(PickerEvent::Chosen(chosen.path.clone()));
    }

    pub(crate) fn cancel(cx: &mut Context<'_, Self>) {
        cx.emit(PickerEvent::Cancelled);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{Events, folder, open, reachable_fake};

    fn choosing_a_folder_opens_it_and_select_hands_over_its_path(cx: &mut TestAppContext) {
        let (fake, events) = (reachable_fake(), Events::default());
        let (picker, cx) = open(cx, &fake, &events);
        cx.run_until_parked();
        picker.update(cx, View::confirm);
        assert_eq!(events.borrow().len(), 0, "nothing chosen yet");
        picker.update(cx, |this, cx| this.choose(&folder("/srv"), cx));
        cx.run_until_parked();
        assert!(
            fake.asked.borrow().contains(&"/srv".to_owned()),
            "the opened folder is listed"
        );
        picker.update(cx, View::confirm);
        assert_eq!(*events.borrow(), [PickerEvent::Chosen("/srv".to_owned())]);
    }

    fn select_needs_a_host_that_answers_and_cancel_always_dismisses(cx: &mut TestAppContext) {
        let (fake, events) = (reachable_fake(), Events::default());
        let (picker, cx) = open(cx, &fake, &events);
        picker.update(cx, |this, cx| this.choose(&folder("/usr"), cx));
        fake.reachable.set(false);
        picker.update(cx, View::confirm);
        assert_eq!(
            events.borrow().len(),
            0,
            "a host that does not answer is not chosen on"
        );
        picker.update(cx, |_, cx| View::cancel(cx));
        assert_eq!(*events.borrow(), [PickerEvent::Cancelled]);
        picker.update(cx, |this, cx| this.toggle("/usr", cx));
        assert!(
            picker.read_with(cx, |this, _| this.expanded.is_empty()),
            "toggling closes it"
        );
    }

    const CASES: [fn(&mut TestAppContext); 2] = [
        choosing_a_folder_opens_it_and_select_hands_over_its_path,
        select_needs_a_host_that_answers_and_cancel_always_dismisses,
    ];

    #[gpui::test]
    fn every_pick_ends_as_the_user_and_the_host_allow(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        for case in CASES {
            case(cx);
        }
    }
}
