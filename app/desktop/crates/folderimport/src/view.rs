//! The dialog: a folder path with a browse button, a name, Cancel and the add button.

use std::rc::Rc;

use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, Render, Subscription, Window,
};
use ui::{Dialog, DialogSize, Surface, TextInput, TextInputEvent};

use crate::folder_text::{absolute, base_name};
use crate::import_event::ImportEvent;
use crate::kind::Kind;

mod parts;

/// Whether the owner is still adding a folder; the dialog reads it on every render.
pub type Adding = Rc<dyn Fn(&str) -> bool>;

/// One of develop's add dialogs; it emits [`ImportEvent`]s for its owner to carry out.
pub struct View {
    kind: Kind,
    adding: Adding,
    folder: Entity<TextInput>,
    name: Entity<TextInput>,
    _inputs: [Subscription; 2],
}

impl std::fmt::Debug for View {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("View")
            .field("kind", &self.kind)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<ImportEvent> for View {}

impl View {
    /// An empty dialog of `kind` with the folder field focused; `adding` answers whether a
    /// folder is still being added.
    pub fn new(
        (kind, adding): (Kind, Adding),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let folder = cx.new(|cx| TextInput::new(kind.folder_placeholder(), cx).on(Surface::Window));
        let name = cx.new(|cx| TextInput::new(kind.name_placeholder(), cx).on(Surface::Window));
        folder.update(cx, |field, cx| field.focus(window, cx));
        let inputs = [
            cx.subscribe(&folder, Self::on_input),
            cx.subscribe(&name, Self::on_input),
        ];
        Self {
            kind,
            adding,
            folder,
            name,
            _inputs: inputs,
        }
    }

    /// Fills the folder with the browsed `path`; the Add repository dialog also names an unnamed
    /// repository after it, as develop's `handlePicked`.
    pub fn set_folder(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.folder.update(cx, |field, cx| field.set_text(path, cx));
        let unnamed = self.name.read(cx).text().is_empty();
        if unnamed && self.kind.names_from_browse() {
            let base = base_name(path).to_owned();
            self.name.update(cx, |field, cx| field.set_text(&base, cx));
        }
        cx.notify();
    }

    /// The owner added `folder`: the dialog that asked for it closes.
    pub fn added(
        &mut self,
        folder: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if self.path(cx) == folder {
            cx.emit(ImportEvent::Close);
        }
    }

    fn on_input(
        &mut self,
        _: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Submitted => self.submit(cx),
            TextInputEvent::Cancelled => cx.emit(ImportEvent::Close),
            _ => cx.notify(),
        }
    }

    fn path(
        &self,
        cx: &Context<'_, Self>,
    ) -> String {
        self.folder.read(cx).text().trim().to_owned()
    }

    fn busy(
        &self,
        cx: &Context<'_, Self>,
    ) -> bool {
        (self.adding)(&self.path(cx))
    }

    fn submit(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let folder = self.path(cx);
        if !absolute(&folder) || self.busy(cx) {
            return;
        }
        let name = self.name.read(cx).text().trim().to_owned();
        cx.emit(ImportEvent::Add { folder, name });
    }
}

impl Render for View {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let me = cx.entity();
        Dialog::new("folder-import")
            .title(self.kind.title())
            .size(DialogSize::Md)
            .body(self.body(cx))
            .footer(self.footer(cx))
            .on_close(move |_, cx| me.update(cx, |_, cx| cx.emit(ImportEvent::Close)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{TestAppContext, VisualTestContext};

    use super::*;

    pub(super) type Heard = Rc<RefCell<Vec<ImportEvent>>>;

    pub(super) fn open<'a>(
        cx: &'a mut TestAppContext,
        (kind, busy): (Kind, &'static str),
    ) -> (Entity<View>, Heard, &'a mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let adding: Adding = Rc::new(move |folder| folder == busy);
        let (view, cx) = cx.add_window_view(|window, cx| View::new((kind, adding), window, cx));
        let heard = Heard::default();
        let sink = record(Rc::clone(&heard));
        cx.update(|_, cx| cx.subscribe(&view, sink).detach());
        (view, heard, cx)
    }

    fn record(heard: Heard) -> impl FnMut(Entity<View>, &ImportEvent, &mut gpui::App) + 'static {
        move |_, event, _| heard.borrow_mut().push(event.clone())
    }

    fn type_into(
        view: &Entity<View>,
        (folder, name): (&str, &str),
        cx: &mut VisualTestContext,
    ) {
        view.update(cx, |view, cx| {
            view.folder
                .update(cx, |field, cx| field.set_text(folder, cx));
            view.name.update(cx, |field, cx| field.set_text(name, cx));
        });
    }

    #[gpui::test]
    fn only_an_absolute_folder_is_added_under_the_typed_name(cx: &mut TestAppContext) {
        let (view, heard, cx) = open(cx, (Kind::Space, ""));
        type_into(&view, ("code/x", "X"), cx);
        view.update(cx, View::submit);
        assert!(heard.borrow().is_empty(), "a relative path is refused");
        type_into(&view, (" /srv/app ", " Web "), cx);
        view.update(cx, View::submit);
        let add = ImportEvent::Add {
            folder: "/srv/app".into(),
            name: "Web".into(),
        };
        assert_eq!(*heard.borrow(), [add]);
    }

    #[gpui::test]
    fn a_folder_being_added_is_not_asked_again_and_closes_once_added(cx: &mut TestAppContext) {
        let (view, heard, cx) = open(cx, (Kind::Repo, "/srv/app"));
        type_into(&view, ("/srv/app", ""), cx);
        view.update(cx, View::submit);
        assert!(heard.borrow().is_empty(), "Adding… is disabled");
        view.update(cx, |view, cx| view.added("/srv/other", cx));
        assert!(heard.borrow().is_empty(), "another folder's add");
        view.update(cx, |view, cx| view.added("/srv/app", cx));
        assert_eq!(*heard.borrow(), [ImportEvent::Close]);
    }

    fn browsed_name(
        kind: Kind,
        typed: &str,
        cx: &mut TestAppContext,
    ) -> String {
        let (view, _, cx) = open(cx, (kind, ""));
        type_into(&view, ("", typed), cx);
        view.update(cx, |view, cx| view.set_folder("/srv/app/", cx));
        view.read_with(cx, |view, cx| view.name.read(cx).text().to_owned())
    }

    #[gpui::test]
    fn a_browsed_folder_names_only_an_unnamed_repository(cx: &mut TestAppContext) {
        let names = [
            browsed_name(Kind::Repo, "", cx),
            browsed_name(Kind::Repo, "Mine", cx),
            browsed_name(Kind::Space, "", cx),
        ];
        assert_eq!(names, ["app", "Mine", ""]);
    }
}
