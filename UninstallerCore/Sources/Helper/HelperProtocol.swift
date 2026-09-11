import Foundation

/// XPC protocol shared between the app and the privileged helper daemon.
@objc public protocol UninstallerHelperProtocol {
    func trashItems(at paths: [String], withReply reply: @escaping ([String: String]) -> Void)
    func bootoutLaunchd(label: String, domain: String, withReply reply: @escaping (String?) -> Void)
    func ping(withReply reply: @escaping (String) -> Void)
}

public enum HelperConstants {
    public static let machServiceName = "com.safeuninstaller.UninstallerHelper"
    public static let helperPlistName = "com.safeuninstaller.UninstallerHelper.plist"
    public static let helperBundleID = "com.safeuninstaller.UninstallerHelper"
    public static let appBundleID = "com.safeuninstaller.app"
}
