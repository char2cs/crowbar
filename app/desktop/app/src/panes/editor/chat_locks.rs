pub(crate) type ChatLocks = std::rc::Rc<dyn Fn(&str) -> bool>;
