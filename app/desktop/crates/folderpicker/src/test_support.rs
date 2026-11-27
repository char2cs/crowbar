#![cfg(test)]

use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;

use gpui::{App, Entity, TestAppContext, VisualTestContext};
use store::{BrowseEntry, EntryKind, FolderListing};

use crate::folder_source::Reader;
use crate::folder_source::Requests;
use crate::picker_event::PickerEvent;
use crate::view::View;

#[derive(Default)]
pub(crate) struct Fake {
    pub(crate) listings: RefCell<HashMap<String, FolderListing>>,
    pub(crate) asked: RefCell<Vec<String>>,
    pub(crate) retried: RefCell<Vec<String>>,
    pub(crate) reachable: Cell<bool>,
}

impl Reader for Fake {
    fn host_name(&self) -> String {
        "Box".to_owned()
    }

    fn can_ask(&self) -> bool {
        self.reachable.get()
    }

    fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing> {
        self.listings.borrow().get(path).cloned()
    }
}

impl Requests for Fake {
    fn request(
        &self,
        path: &str,
    ) {
        self.asked.borrow_mut().push(path.to_owned());
    }

    fn retry(
        &self,
        path: &str,
    ) {
        self.retried.borrow_mut().push(path.to_owned());
    }
}

pub(crate) type Events = Rc<RefCell<Vec<PickerEvent>>>;

pub(crate) fn folder(path: &str) -> BrowseEntry {
    let name = path.rsplit('/').next().unwrap_or_default();
    BrowseEntry {
        name: name.to_owned(),
        path: path.to_owned(),
        kind: EntryKind::Folder,
    }
}

pub(crate) fn open<'a>(
    cx: &'a mut TestAppContext,
    fake: &Rc<Fake>,
    events: &Events,
) -> (Entity<View>, &'a mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).expect("ui installs"));
    let source = Rc::clone(fake);
    let (picker, cx) = cx.add_window_view(|_, _| View::over(source));
    let sink = Rc::clone(events);
    cx.update(|_, cx| record(&picker, sink, cx));
    (picker, cx)
}

fn record(
    picker: &Entity<View>,
    sink: Events,
    cx: &mut App,
) {
    cx.subscribe(picker, move |_, event: &PickerEvent, _| {
        sink.borrow_mut().push(event.clone());
    })
    .detach();
}

pub(crate) fn reachable_fake() -> Rc<Fake> {
    let fake = Rc::new(Fake::default());
    fake.reachable.set(true);
    let root = FolderListing::Loaded(vec![folder("/srv"), folder("/usr")]);
    fake.listings.borrow_mut().insert("/".to_owned(), root);
    fake
}
