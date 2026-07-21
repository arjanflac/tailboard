import SwiftUI

/// Shared motion tokens. Critically damped springs only (damping 1.0):
/// graceful, non-distracting, and interruptible by nature because they are
/// state-driven. Bounce is reserved for symbol effects on confirmed actions.
/// Callers must pass `.none` when Reduce Motion is enabled.
extension Animation {
    /// Default UI spring: quick settle, no overshoot.
    static let tgSpring = Animation.spring(response: 0.3, dampingFraction: 1.0)
    /// Slower spring for larger surface moves (banners, section inserts).
    static let tgGentle = Animation.spring(response: 0.45, dampingFraction: 1.0)
}
