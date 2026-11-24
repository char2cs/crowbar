use std::ops::Range;

use editor::{FindOption, FindOptions, FindQuery, Vertical};
use gpui::{AppContext as _, Context, Entity, Focusable as _, Subscription, Window};
use gpui_kit::base::input::InputEvent;
use gpui_kit::component::input::{EditorState, RangeDecorationCollection};
use ui::{TextInput, TextInputEvent, TextInputStyle};

use crate::reveal::Reveal;

mod count;
mod highlights;
mod render;
mod replacing;
mod stepping;

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findWidget.css
const LABEL_SIZE: f32 = 12.0;
const COUNT_MARGIN: f32 = 3.0;
const FIND_PLACEHOLDER: &str = "Find";
const REPLACE_PLACEHOLDER: &str = "Replace";

pub(crate) struct FindWidget {
    editor: Entity<EditorState>,
    find_input: Entity<TextInput>,
    replace_input: Entity<TextInput>,
    reveal: Reveal,
    options: FindOptions,
    scope: Option<Range<usize>>,
    matches: Vec<Range<usize>>,
    invalid: Option<String>,
    current: Option<usize>,
    highlights: RangeDecorationCollection,
    _subscriptions: Vec<Subscription>,
}

impl FindWidget {
    pub(crate) fn new(
        editor: Entity<EditorState>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let find_input =
            cx.new(|cx| TextInput::new(FIND_PLACEHOLDER, cx).style(TextInputStyle::Bare));
        let replace_input =
            cx.new(|cx| TextInput::new(REPLACE_PLACEHOLDER, cx).style(TextInputStyle::Bare));
        let highlights = editor.update(cx, |state, cx| {
            state.create_range_decorations_collection(Vec::new(), cx)
        });
        let subscriptions = vec![
            cx.subscribe_in(&find_input, window, |widget, _, event, window, cx| {
                widget.on_find_input(event, window, cx);
            }),
            cx.subscribe_in(&replace_input, window, |widget, _, event, window, cx| {
                widget.on_replace_input(event, window, cx);
            }),
            cx.subscribe(&editor, Self::on_editor),
        ];
        Self {
            editor,
            find_input,
            replace_input,
            reveal: Reveal::Hidden,
            options: FindOptions::default(),
            scope: None,
            matches: Vec::new(),
            invalid: None,
            current: None,
            highlights,
            _subscriptions: subscriptions,
        }
    }

    pub(crate) fn is_shown(&self) -> bool {
        self.reveal != Reveal::Hidden
    }

    pub(crate) fn open(
        &mut self,
        reveal: Reveal,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let seed = self.seed(cx);
        let seeded = seed.is_some();
        if let Some(seed) = seed {
            self.find_input
                .update(cx, |input, cx| input.set_text(&seed, cx));
        }
        self.find_input.update(cx, TextInput::select_all);
        self.reveal = reveal;
        self.research(cx);
        let focused = match (reveal, seeded) {
            (Reveal::Replace, true) => &self.replace_input,
            _ => &self.find_input,
        };
        let focused = focused.read(cx).focus_handle(cx);
        window.focus(&focused, cx);
        cx.notify();
    }

    pub(crate) fn close(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.reveal = Reveal::Hidden;
        self.scope = None;
        self.highlights.clear(cx);
        let editor = self.editor.read(cx).focus_handle(cx);
        window.focus(&editor, cx);
        cx.notify();
    }

    pub(crate) fn toggle(
        &mut self,
        option: FindOption,
        cx: &mut Context<'_, Self>,
    ) {
        self.options = self.options.toggled(option);
        self.research(cx);
    }

    pub(crate) fn toggle_scope(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let selected = self.editor.read(cx).selected_range();
        self.scope = match self.scope {
            Some(_) => None,
            None => (!selected.is_empty()).then_some(selected),
        };
        self.research(cx);
    }

    fn query(
        &self,
        cx: &Context<'_, Self>,
    ) -> FindQuery {
        FindQuery::new(self.find_input.read(cx).text(), self.options.clone())
    }

    fn seed(
        &self,
        cx: &Context<'_, Self>,
    ) -> Option<String> {
        let state = self.editor.read(cx);
        let selected = state.selected_range();
        let text = state.value();
        let around = Some(selected.clone())
            .filter(|selected| !selected.is_empty())
            .or_else(|| editor::word_at(&text, selected.start))?;
        let seed = text.get(around)?;
        (!seed.contains('\n')).then(|| seed.to_owned())
    }

    fn research(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.invalid = None;
        let text = self.editor.read(cx).value();
        let found = match self.query(cx).matches(&text) {
            Ok(found) => found,
            Err(error) => {
                self.invalid = Some(error.to_string());
                Vec::new()
            },
        };
        let scope = self.scope.clone();
        let within = |range: &Range<usize>| {
            scope
                .as_ref()
                .is_none_or(|scope| scope.start <= range.start && range.end <= scope.end)
        };
        self.matches = found.into_iter().filter(within).collect();
        let selected = self.editor.read(cx).selected_range();
        self.current = self
            .matches
            .iter()
            .position(|range| range.start <= selected.start && selected.end <= range.end);
        self.paint(cx);
    }

    fn on_find_input(
        &mut self,
        event: &TextInputEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(_) => self.typed(cx),
            TextInputEvent::Submitted => self.step(Vertical::Down, cx),
            TextInputEvent::Cancelled => self.close(window, cx),
            _ => {},
        }
    }

    fn on_replace_input(
        &mut self,
        event: &TextInputEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(_) => cx.notify(),
            TextInputEvent::Submitted => self.replace_one(window, cx),
            TextInputEvent::Cancelled => self.close(window, cx),
            _ => {},
        }
    }

    fn on_editor(
        &mut self,
        _: Entity<EditorState>,
        event: &InputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let changed = matches!(event, InputEvent::Change);
        if changed && self.is_shown() {
            self.research(cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use editor::FindOption;
    use gpui::TestAppContext;

    use super::FindWidget;
    use crate::Access;
    use crate::editor_view::test_support::{find_of, on_find, opened};
    use crate::reveal::Reveal;
    use gpui::{Context, Entity, VisualTestContext, Window};

    #[derive(Clone, Copy)]
    enum Step {
        Open,
        MatchCase,
        Scope,
        Close,
    }

    const TEXT: &str = "total x Total\ntotal";
    const SCOPE: std::ops::Range<usize> = 0..13;
    type Case = (&'static [Step], bool, &'static str);

    const CASES: &[Case] = &[
        (&[Step::Open], true, "1 of 3"),
        (&[Step::Open, Step::MatchCase], true, "1 of 2"),
        (
            &[Step::Open, Step::MatchCase, Step::MatchCase],
            true,
            "1 of 3",
        ),
        (&[Step::Open, Step::Scope], true, "? of 2"),
        (&[Step::Open, Step::Scope, Step::Scope], true, "? of 3"),
        (&[Step::Open, Step::Close], false, "1 of 3"),
    ];

    #[gpui::test]
    fn the_widget_seeds_from_the_caret_word_and_counts_under_its_toggles(cx: &mut TestAppContext) {
        for (steps, shown, label) in CASES {
            let (view, window) = opened((TEXT, Some(1)), Access::Editable, cx);
            let find = find_of(&view, window);
            run(&find, steps, window);
            let seen = window.read(|cx| (find.read(cx).is_shown(), find.read(cx).label(cx)));
            assert_eq!(seen, (*shown, (*label).to_owned()), "{}", steps.len());
        }
    }

    fn run(
        find: &Entity<FindWidget>,
        steps: &'static [Step],
        window: &mut VisualTestContext,
    ) {
        on_find(find, window, |find, window, cx| {
            apply_all(find, steps, (window, cx));
        });
    }

    fn apply_all(
        find: &mut FindWidget,
        steps: &[Step],
        (window, cx): (&mut Window, &mut Context<'_, FindWidget>),
    ) {
        for step in steps {
            apply(find, *step, (window, cx));
        }
    }

    fn apply(
        find: &mut FindWidget,
        step: Step,
        (window, cx): (&mut Window, &mut Context<'_, FindWidget>),
    ) {
        match step {
            Step::Open => find.open(Reveal::Find, window, cx),
            Step::MatchCase => find.toggle(FindOption::MatchCase, cx),
            Step::Scope => scope_over(find, SCOPE, cx),
            Step::Close => find.close(window, cx),
        }
    }

    fn scope_over(
        find: &mut FindWidget,
        selected: std::ops::Range<usize>,
        cx: &mut Context<'_, FindWidget>,
    ) {
        find.editor
            .update(cx, |state, cx| state.set_selected_range(selected, cx));
        find.toggle_scope(cx);
    }
}
