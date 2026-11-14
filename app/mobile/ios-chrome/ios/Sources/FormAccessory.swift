import ObjectiveC
import UIKit
import WebKit

/// Takes away WebKit's form bar (previous / next / done) above the keyboard: the composer and
/// fields are the app's own, and the bar only covers them. WebKit offers no switch, so the
/// webview's content view gets a subclass whose `inputAccessoryView` is nil.
enum FormAccessory {
  private static let suffix = "_CrowbarNoFormBar"

  static func hide(in webview: WKWebView) {
    guard
      let content = webview.scrollView.subviews.first(where: {
        NSStringFromClass(type(of: $0)).hasPrefix("WKContent")
      })
    else { return }
    let base: AnyClass = type(of: content)
    let name = NSStringFromClass(base)
    guard !name.hasSuffix(suffix), let bare = subclass(of: base, named: name + suffix) else {
      return
    }
    object_setClass(content, bare)
  }

  private static func subclass(of base: AnyClass, named name: String) -> AnyClass? {
    if let existing = NSClassFromString(name) { return existing }
    guard let made = objc_allocateClassPair(base, name, 0) else { return nil }
    let selector = #selector(getter: UIResponder.inputAccessoryView)
    let none: @convention(block) (AnyObject) -> UIView? = { _ in nil }
    let types = class_getInstanceMethod(UIResponder.self, selector).flatMap {
      method_getTypeEncoding($0)
    }
    class_addMethod(made, selector, imp_implementationWithBlock(none), types)
    objc_registerClassPair(made)
    return made
  }
}
