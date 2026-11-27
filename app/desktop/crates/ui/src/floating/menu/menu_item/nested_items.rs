use std::any::Any;
use std::rc::Rc;

#[derive(Clone)]
pub(crate) struct NestedItems(Rc<dyn Any>);

impl NestedItems {
    pub(crate) fn new<T: 'static>(items: Rc<[T]>) -> Self {
        Self(Rc::new(items))
    }

    pub(crate) fn get<T: 'static>(&self) -> Option<Rc<[T]>> {
        self.0.downcast_ref::<Rc<[T]>>().cloned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn items_come_back_only_as_the_stored_type() {
        let nested = NestedItems::new::<u8>(Rc::from([1, 2, 3]));
        assert_eq!(nested.get::<u8>().map(|items| items.len()), Some(3));
        assert!(nested.get::<u16>().is_none());
    }
}
