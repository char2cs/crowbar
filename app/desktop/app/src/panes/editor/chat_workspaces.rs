pub(crate) type ChatWorkspaces = std::rc::Rc<dyn Fn(&str) -> Option<String>>;
