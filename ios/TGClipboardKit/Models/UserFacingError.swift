import Foundation

/// Maps any error thrown by the client layers to plain, actionable copy.
/// Principle 4 of the interface spec: raw `error.localizedDescription`
/// never reaches a label. Kept in TGClipboardKit so every app surface shares
/// one voice.
public enum UserFacingError {
    public static func message(_ error: Error) -> String {
        if let clipError = error as? TGClipboardError {
            switch clipError {
            case .noHubURL:
                return "Open Tailboard once to finish setup, then try again."
            case .hubUnreachable(let underlying):
                return connectivityMessage(underlying)
            case .httpError(let code, _):
                switch code {
                case 401, 403:
                    return "The sync server rejected the request. Check Advanced settings."
                case 404:
                    return "The sync server doesn't recognize this device. Reconnect in Settings."
                case 500...:
                    return "The sync server hit a problem. Try again shortly."
                default:
                    return "The sync server returned an error (\(code)). Try again."
                }
            }
        }
        if error is URLError {
            return connectivityMessage(error)
        }
        return "Something went wrong. Try again."
    }

    /// Connectivity failures usually mean Tailscale is off — say so.
    private static func connectivityMessage(_ error: Error) -> String {
        guard let urlError = error as? URLError else {
            return "Can't reach your other devices. Check that Tailscale is on."
        }
        switch urlError.code {
        case .notConnectedToInternet, .networkConnectionLost:
            return "No connection. Check that Tailscale is on, then try again."
        case .timedOut:
            return "No response from your other devices. Check that Tailscale is on."
        case .cannotFindHost, .cannotConnectToHost, .dnsLookupFailed:
            return "Can't reach your other devices. Check that Tailscale is on."
        default:
            return "Can't reach your other devices. Check that Tailscale is on."
        }
    }
}
