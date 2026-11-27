use std::rc::Rc;

use crate::values::change::Change;
use crate::values::subscription::Subscription;

pub type Listener = Rc<dyn Fn(&Change)>;

pub trait Follow {
    fn subscribe(
        &self,
        listener: Listener,
    ) -> Subscription;
}
