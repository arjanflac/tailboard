import Foundation
import Security

enum SharedKeychain {
    private static let service = "com.arjanflac.tgclipboard.shared"

    static func read(_ account: String) -> Data? {
        var query = baseQuery(account)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess else {
            return nil
        }
        return result as? Data
    }

    static func write(_ data: Data?, account: String) {
        let query = baseQuery(account)
        if let data {
            let attributes = [kSecValueData as String: data]
            let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
            if status == errSecItemNotFound {
                var item = query
                item[kSecValueData as String] = data
                SecItemAdd(item as CFDictionary, nil)
            }
        } else {
            SecItemDelete(query as CFDictionary)
        }
    }

    private static func baseQuery(_ account: String) -> [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        if let accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }

    private static var accessGroup: String? {
        guard let prefix = Bundle.main.object(
            forInfoDictionaryKey: "TGClipboardAppIdentifierPrefix"
        ) as? String, !prefix.isEmpty else {
            return nil
        }
        return prefix + AppGroupStore.suiteName
    }
}
