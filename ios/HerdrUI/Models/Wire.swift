import Foundation

/// `/v1/health`.
struct Health: Decodable, Sendable {
    struct Core: Decodable, Sendable {
        struct Identity: Decodable, Sendable {
            var id: String
            var name: String
        }
        var core: Identity
        var version: String
    }
    struct Machines: Decodable, Sendable {
        var machines: [Machine]
    }
    var bridgeVersion: String
    var core: Core?
    var machines: Machines?
    var coreError: String?
    var pushEnabled: Bool?

    enum CodingKeys: String, CodingKey {
        case bridgeVersion = "bridge_version"
        case core, machines
        case coreError = "core_error"
        case pushEnabled = "push_enabled"
    }
}

/// A machine the hub's core has added.
struct Machine: Decodable, Hashable, Identifiable, Sendable {
    var name: String
    var host: String?
    var state: String
    var version: String?
    var id: String { name }
}

/// `terminal.list` result.
struct TerminalList: Decodable, Sendable {
    var terminals: [Terminal]
    var machines: [Machine]?
}

/// `terminal.read` result.
struct ReadResult: Decodable, Sendable {
    var text: String
    var complete: Bool
    var how: String
    var why: String?
}

/// `terminal.send` / `terminal.keys` result.
struct TurnResult: Decodable, Sendable {
    var turn: Int
    var status: String?
    var started: Bool?
}

/// `terminal.interrupt` result.
struct InterruptResult: Decodable, Sendable {
    var key: String
    /// stopped, exited, still_working or unknown.
    var outcome: String
}

/// One line on the event WebSocket: the subscribe response (snapshot) or a
/// core notification (`event`, `heartbeat`, `missed`).
struct StreamMessage: Decodable, Sendable {
    struct Snapshot: Decodable, Sendable {
        var seq: Int64
        var terminals: [Terminal]
    }
    struct Params: Decodable, Sendable {
        var type: String?
        var terminal: String?
        var seq: Int64?
    }
    var result: Snapshot?
    var method: String?
    var params: Params?
}

/// A `created` event, whose data is a full terminal.
struct CreatedEvent: Decodable, Sendable {
    struct Params: Decodable, Sendable { var data: Terminal }
    var params: Params
}
