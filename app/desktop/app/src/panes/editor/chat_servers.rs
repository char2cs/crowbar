pub(crate) type ChatServers =
    std::rc::Rc<dyn Fn(&str) -> std::rc::Rc<dyn editorview::LanguageServer>>;
