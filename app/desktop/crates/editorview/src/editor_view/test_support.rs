//! Views opened for the unit tests, and reads of what they show.

use gpui::{
    App, AppContext as _, Context, Entity, Focusable as _, TestAppContext, VisualTestContext,
    Window,
};

use super::EditorView;
use crate::document::Document;
use crate::find_widget::FindWidget;
use crate::{Access, DocumentOpening, EditorEvent, Opening};

pub(crate) fn opened<'cx>(
    (text, line): (&str, Option<u32>),
    access: Access,
    cx: &'cx mut TestAppContext,
) -> (Entity<EditorView>, &'cx mut VisualTestContext) {
    shown(
        (document_opening("src/a.ts", text), Opening { line, access }),
        None,
        cx,
    )
}

pub(crate) fn opened_at<'cx>(
    path: &str,
    cx: &'cx mut TestAppContext,
) -> (Entity<EditorView>, &'cx mut VisualTestContext) {
    let opening = Opening {
        line: None,
        access: Access::Editable,
    };
    shown((document_opening(path, "# Title\n"), opening), None, cx)
}

pub(crate) fn document_opening(
    path: &str,
    text: &str,
) -> DocumentOpening {
    DocumentOpening {
        text: text.to_owned(),
        path: path.to_owned(),
        saved: None,
    }
}

/// A view over a new document `openings` describes, told to `server` when one is given.
pub(crate) fn shown(
    (document, opening): (DocumentOpening, Opening),
    server: Option<std::rc::Rc<dyn crate::LanguageServer>>,
    cx: &mut TestAppContext,
) -> (Entity<EditorView>, &mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).unwrap());
    cx.update(crate::init);
    cx.add_window_view(|window, cx| {
        let document = cx.new(|cx| Document::new(document, server, cx));
        EditorView::new(document, opening, window, cx)
    })
}

pub(crate) fn document_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Entity<Document> {
    window.read(|cx| view.read(cx).document().clone())
}

pub(crate) fn find_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Entity<FindWidget> {
    window.read(|cx| view.read(cx).find.clone())
}

pub(crate) fn selected_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> std::ops::Range<usize> {
    window.read(|cx| view.read(cx).state.read(cx).selected_range())
}

pub(crate) fn rows_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Option<std::ops::Range<usize>> {
    window.read(|cx| view.read(cx).state.read(cx).visible_row_range())
}

pub(crate) fn line_height_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Option<gpui::Pixels> {
    window.read(|cx| view.read(cx).state.read(cx).line_height())
}

pub(crate) fn scroll_top_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> gpui::Pixels {
    window.read(|cx| view.read(cx).state.read(cx).scroll_offset().y)
}

pub(crate) fn body_of(window: &mut VisualTestContext) -> Option<gpui::Bounds<gpui::Pixels>> {
    window.debug_bounds(super::BODY)
}

pub(crate) fn listened(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> std::rc::Rc<std::cell::RefCell<Vec<EditorEvent>>> {
    let heard = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
    let log = std::rc::Rc::clone(&heard);
    let record = move |_: Entity<EditorView>, event: &EditorEvent, _: &mut App| {
        log.borrow_mut().push(event.clone());
    };
    window.update(|window, cx| {
        window.focus(&view.focus_handle(cx), cx);
        cx.subscribe(view, record).detach();
    });
    heard
}

pub(crate) fn on_find<R>(
    find: &Entity<FindWidget>,
    window: &mut VisualTestContext,
    run: impl FnOnce(&mut FindWidget, &mut Window, &mut Context<'_, FindWidget>) -> R,
) -> R {
    window.update(|window, cx| find.update(cx, |find, cx| run(find, window, cx)))
}

pub(crate) fn open_find(
    find: &Entity<FindWidget>,
    reveal: crate::reveal::Reveal,
    window: &mut VisualTestContext,
) {
    on_find(find, window, |find, window, cx| {
        find.open(reveal, window, cx);
    });
}

type Posts = std::rc::Rc<std::cell::RefCell<Vec<(&'static str, serde_json::Value)>>>;

/// A language server that records what it was told and answers from a table, `null` else.
#[derive(Clone, Default)]
pub(crate) struct FakeServer {
    told: Posts,
    answers: Posts,
}

impl FakeServer {
    pub(crate) fn port(&self) -> std::rc::Rc<dyn crate::LanguageServer> {
        std::rc::Rc::new(self.clone())
    }

    pub(crate) fn answer(
        &self,
        verb: &'static str,
        data: serde_json::Value,
    ) {
        self.answers.borrow_mut().push((verb, data));
    }

    pub(crate) fn verbs(&self) -> Vec<&'static str> {
        self.told.borrow().iter().map(|(verb, _)| *verb).collect()
    }

    pub(crate) fn bodies(&self) -> Vec<serde_json::Value> {
        self.told
            .borrow()
            .iter()
            .map(|(_, body)| body.clone())
            .collect()
    }
}

impl crate::LanguageServer for FakeServer {
    fn post(
        &self,
        verb: &'static str,
        body: serde_json::Value,
        _: &mut App,
    ) -> gpui::Task<Result<serde_json::Value, crate::LspError>> {
        self.told.borrow_mut().push((verb, body));
        let answers = self.answers.borrow();
        let data = answers.iter().find(|(asked, _)| *asked == verb);
        gpui::Task::ready(Ok(
            data.map_or(serde_json::Value::Null, |(_, data)| data.clone())
        ))
    }
}

/// A focused view of `text` at `path` that tells a [`FakeServer`] about its document.
pub(crate) fn served<'cx>(
    (path, text): (&str, &str),
    cx: &'cx mut TestAppContext,
) -> (Entity<EditorView>, &'cx mut VisualTestContext, FakeServer) {
    let server = FakeServer::default();
    let opening = Opening {
        line: None,
        access: Access::Editable,
    };
    let (view, window) = shown(
        (document_opening(path, text), opening),
        Some(server.port()),
        cx,
    );
    window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
    (view, window, server)
}

/// What the view's hover provider answers at `offset` of its text.
pub(crate) fn hover_at(
    view: &Entity<EditorView>,
    offset: usize,
    window: &mut VisualTestContext,
) -> Option<lsp_types::Hover> {
    let hover = window.update(|window, cx| {
        let state = view.read(cx).state.clone();
        let text = state.read(cx).text().clone();
        let provider = state.read(cx).lsp().hover_provider.clone()?;
        Some(provider.hover(&text, offset, window, cx))
    })?;
    let answer = std::rc::Rc::new(std::cell::RefCell::new(None));
    let slot = std::rc::Rc::clone(&answer);
    window
        .spawn(async move |_| *slot.borrow_mut() = hover.await.ok())
        .detach();
    window.run_until_parked();
    answer.borrow_mut().take().flatten()
}

/// The items the view's completion provider answers at `offset` of its text.
pub(crate) fn completions_at(
    view: &Entity<EditorView>,
    offset: usize,
    window: &mut VisualTestContext,
) -> Vec<lsp_types::CompletionItem> {
    let asked = window.update(|window, cx| ask_completions(view, offset, window, cx));
    let Some(asked) = asked else {
        return Vec::new();
    };
    let answer = std::rc::Rc::new(std::cell::RefCell::new(None));
    let slot = std::rc::Rc::clone(&answer);
    window
        .spawn(async move |_| *slot.borrow_mut() = asked.await.ok())
        .detach();
    window.run_until_parked();
    match answer.take() {
        Some(lsp_types::CompletionResponse::Array(items)) => items,
        Some(lsp_types::CompletionResponse::List(list)) => list.items,
        None => Vec::new(),
    }
}

fn ask_completions(
    view: &Entity<EditorView>,
    offset: usize,
    window: &mut Window,
    cx: &mut App,
) -> Option<gpui::Task<gpui::Result<lsp_types::CompletionResponse>>> {
    let state = view.read(cx).state.clone();
    let text = state.read(cx).text().clone();
    let provider = state.read(cx).lsp().completion_provider.clone()?;
    let context = lsp_types::CompletionContext {
        trigger_kind: lsp_types::CompletionTriggerKind::INVOKED,
        trigger_character: None,
    };
    Some(provider.completions(&text, offset, context, window, cx))
}

/// Presses `keys`, each `@namespace::Action` token as the chord the keymap binds last to that
/// action, so a test presses each platform's own chord.
pub(crate) fn press(
    keys: &str,
    window: &mut VisualTestContext,
) {
    let pressed = window.read(|cx| {
        let tokens: Vec<String> = keys.split(' ').map(|token| key_of(token, cx)).collect();
        tokens.join(" ")
    });
    window.simulate_keystrokes(&pressed);
}

fn key_of(
    token: &str,
    cx: &App,
) -> String {
    match token.strip_prefix('@') {
        Some(action) => chord_of(action, cx),
        None => token.to_owned(),
    }
}

fn chord_of(
    action: &str,
    cx: &App,
) -> String {
    let built = cx.build_action(action, None).unwrap();
    let os = cx.platform_info().os;
    let keymap = cx.key_bindings();
    let keymap = keymap.borrow();
    let binding = keymap.bindings_for_action(built.as_ref()).last().unwrap();
    let strokes: Vec<String> = binding
        .keystrokes()
        .iter()
        .map(|key| key.unparse(os))
        .collect();
    strokes.join(" ")
}
