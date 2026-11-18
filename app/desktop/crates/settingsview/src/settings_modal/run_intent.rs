//! Applies an [`Intent`] to the state and performs the resulting effect.

use gpui::{Context, Window};
use settings::{Effect, Key, RecordCommand, Value};
use ui::TextInputEvent;

use crate::intent::Intent;
use crate::intent::Runner;
use crate::settings_modal::SettingsModal;

impl Runner for SettingsModal {
    fn run(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.notice = None;
        let effect = self.apply(intent, window, cx);
        self.perform(effect);
        self.fields.sync(&self.state, cx);
        cx.notify();
    }
}

impl SettingsModal {
    fn perform(
        &mut self,
        effect: Option<Effect>,
    ) {
        let Some(effect) = effect else {
            return;
        };
        let failure = match effect {
            Effect::Write(key, value) => self.write(&key, value),
            Effect::Invoke(id) => self
                .settings
                .invoke(&id)
                .err()
                .map(|error| error.to_string()),
            Effect::Record {
                source,
                record,
                command,
            } => self
                .records
                .apply(&source, &record, command)
                .err()
                .map(|error| error.to_string()),
        };
        if failure.is_some() {
            self.notice = failure;
        }
    }

    fn write(
        &mut self,
        key: &Key,
        value: Value,
    ) -> Option<String> {
        let Err(error) = self.settings.set(key, value) else {
            self.state.acknowledged(key);
            return None;
        };
        self.state.write_failed(key);
        Some(format!(
            "Could not save {}. {error}",
            self.state.label_of(key)
        ))
    }

    fn apply(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<Effect> {
        match intent {
            Intent::Toggle(row) => self.state.toggle(&row),
            Intent::Choose(row, index) => self.state.choose(&row, index),
            Intent::Edit(row, event) => self.edit(&row, &event),
            Intent::Commit(row) => self.state.commit(&row),
            Intent::Nudge(row, nudge) => self.state.nudge(&row, nudge),
            Intent::Reset(row) => self.state.reset(&row),
            Intent::Button(spec) => self.state.press_button(&spec),
            Intent::Select(page) => {
                self.state.select_page(&page);
                None
            },
            Intent::Move(movement) => {
                self.state.move_selection(movement);
                None
            },
            Intent::Search(event) => self.search_event(&event, window, cx),
            Intent::Capture(command) => {
                self.state.toggle_capture(&command);
                self.focus.focus(window);
                None
            },
            Intent::ClearChord(command) => self.state.clear_chord(&command),
            Intent::Key(pressed) => self.state.press_key(&pressed),
            Intent::Accept => self.state.accept_prompt(),
            Intent::Dismiss => {
                self.state.dismiss_prompt();
                None
            },
            Intent::Close => self.close(window, cx),
            record => Self::record_effect(record),
        }
    }

    fn edit(
        &mut self,
        row: &settings::Row,
        event: &TextInputEvent,
    ) -> Option<Effect> {
        match event {
            TextInputEvent::Changed(text) => {
                self.state.edit(row, text);
                None
            },
            TextInputEvent::Submitted => self.state.commit(row),
            TextInputEvent::Cancelled => {
                self.state.revert(row);
                None
            },
            _ => None,
        }
    }

    fn search_event(
        &mut self,
        event: &TextInputEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<Effect> {
        match event {
            TextInputEvent::Changed(text) => self.state.set_query(text),
            TextInputEvent::Cancelled => {
                self.search.update(cx, |input, cx| input.set_text("", cx));
                self.state.set_query("");
                self.focus.focus(window);
            },
            TextInputEvent::Submitted => self.focus.focus(window),
            _ => {},
        }
        None
    }

    fn close(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<Effect> {
        if let Some(handler) = self.on_close.clone() {
            handler(window, cx);
        }
        None
    }

    fn record_effect(intent: Intent) -> Option<Effect> {
        let (source, record, command) = match intent {
            Intent::Flag {
                source,
                record,
                flag,
                on,
            } => (source, record, RecordCommand::SetFlag { flag, on }),
            Intent::Reorder { source, record, to } => (source, record, RecordCommand::Move { to }),
            Intent::RunRecord {
                source,
                record,
                action,
            } => (
                source,
                record,
                RecordCommand::Run {
                    action: action.id().to_owned(),
                },
            ),
            Intent::Retry(source) => (source, String::new(), RecordCommand::Refresh),
            _ => return None,
        };
        Some(Effect::Record {
            source,
            record,
            command,
        })
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Read as _};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};
    use ui::TextInputEvent;

    #[gpui::test]
    fn a_cancelled_search_clears_the_query(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("git".to_owned())),
            cx,
        );
        run(&harness, Intent::Search(TextInputEvent::Cancelled), cx);
        assert_eq!(
            inspect(&harness, cx, |modal| modal.state.query().to_owned()),
            ""
        );
    }

    #[gpui::test]
    fn submitted_text_commits_and_cancelled_text_reverts(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        let target = row("browserHomepage");
        run(
            &harness,
            Intent::Edit(
                target.clone(),
                TextInputEvent::Changed("https://a.test".to_owned()),
            ),
            cx,
        );
        run(
            &harness,
            Intent::Edit(target.clone(), TextInputEvent::Cancelled),
            cx,
        );
        assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
        run(
            &harness,
            Intent::Edit(
                target.clone(),
                TextInputEvent::Changed("https://b.test".to_owned()),
            ),
            cx,
        );
        run(
            &harness,
            Intent::Edit(target, TextInputEvent::Submitted),
            cx,
        );
        assert!(harness.backend.get(&Key::new("browserHomepage")).is_some());
    }
}
