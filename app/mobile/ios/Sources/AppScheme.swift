import Foundation
import WebKit

@_silgen_name("crowbar_chrome_dev_url")
private func crowbarChromeDevURL() -> UnsafePointer<CChar>?
@_silgen_name("crowbar_chrome_asset")
private func crowbarChromeAsset(
  _ path: UnsafePointer<CChar>,
  _ len: UnsafeMutablePointer<Int>,
  _ mime: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>
) -> UnsafeMutablePointer<UInt8>?
@_silgen_name("crowbar_chrome_asset_free")
private func crowbarChromeAssetFree(
  _ bytes: UnsafeMutablePointer<UInt8>,
  _ len: Int,
  _ mime: UnsafeMutablePointer<CChar>?
)

final class AppScheme: NSObject, WKURLSchemeHandler {
  private let devURL = crowbarChromeDevURL().map { String(cString: $0) }
  private var stopped = Set<ObjectIdentifier>()

  func webView(
    _ webView: WKWebView,
    start task: WKURLSchemeTask
  ) {
    guard let url = task.request.url else { return }
    guard let devURL else { return serve(task, url: url) }
    proxy(task, url: url, to: devURL)
  }

  func webView(
    _ webView: WKWebView,
    stop task: WKURLSchemeTask
  ) {
    stopped.insert(ObjectIdentifier(task))
  }

  private func proxy(
    _ task: WKURLSchemeTask,
    url: URL,
    to devURL: String
  ) {
    var parts = URLComponents(url: url, resolvingAgainstBaseURL: false)
    parts?.fragment = nil
    let path =
      (parts?.percentEncodedPath ?? "") + (parts?.percentEncodedQuery.map { "?\($0)" } ?? "")
    guard let source = URL(string: devURL + path) else { return fail(task) }
    URLSession.shared
      .dataTask(with: source) { data, response, _ in
        DispatchQueue.main.async(execute: self.delivery(task, url, data, response))
      }
      .resume()
  }

  private func delivery(
    _ task: WKURLSchemeTask,
    _ url: URL,
    _ data: Data?,
    _ response: URLResponse?
  ) -> () -> Void {
    {
      guard let data, let http = response as? HTTPURLResponse else { return self.fail(task) }
      let headers = http.allHeaderFields as? [String: String]
      let reply = SchemeReply(
        url: url,
        status: http.statusCode,
        headers: headers ?? [:],
        body: data
      )
      self.respond(task, reply)
    }
  }

  private func serve(
    _ task: WKURLSchemeTask,
    url: URL
  ) {
    var len = 0
    var mime: UnsafeMutablePointer<CChar>?
    let path = url.path.isEmpty ? "/" : url.path
    guard let bytes = path.withCString({ crowbarChromeAsset($0, &len, &mime) }) else {
      return respond(task, SchemeReply(url: url, status: HTTPStatus.notFound))
    }
    let body = Data(bytes: bytes, count: len)
    let type = mime.map { String(cString: $0) } ?? "application/octet-stream"
    crowbarChromeAssetFree(bytes, len, mime)
    respond(
      task,
      SchemeReply(url: url, status: HTTPStatus.ok, headers: ["Content-Type": type], body: body)
    )
  }

  private func respond(
    _ task: WKURLSchemeTask,
    _ reply: SchemeReply
  ) {
    // PLATFORM: WebKit raises if a task it already stopped is answered.
    guard stopped.remove(ObjectIdentifier(task)) == nil else { return }
    guard
      let response = HTTPURLResponse(
        url: reply.url,
        statusCode: reply.status,
        httpVersion: "HTTP/1.1",
        headerFields: reply.headers
      )
    else { return task.didFailWithError(URLError(.badServerResponse)) }
    task.didReceive(response)
    task.didReceive(reply.body)
    task.didFinish()
  }

  private func fail(_ task: WKURLSchemeTask) {
    guard stopped.remove(ObjectIdentifier(task)) == nil else { return }
    task.didFailWithError(URLError(.cannotLoadFromNetwork))
  }
}
