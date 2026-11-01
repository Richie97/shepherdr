import Foundation

/// A program's self-reported status (herdr-core "7501 record").
struct AgentStatus: Codable, Hashable, Sendable {
    var app: String?
    /// idle, working, done, blocked or error.
    var state: String
    /// permission, question or auth — only when blocked.
    var kind: String?
    var msg: String?
    var progress: Int?
    var title: String?
}

/// What runs in a terminal right now.
struct Foreground: Codable, Hashable, Sendable {
    var argv: [String]
    var cwd: String
    var name: String
    var pid: Int32
}

/// A herdr-core terminal. Addresses are `t7` on the hub's own core and
/// `machine/t7` on machines it has added. Field names follow the core;
/// explicit coding keys because `labels` keys must not be case-converted.
struct Terminal: Codable, Identifiable, Hashable, Sendable {
    var address: String
    var argv: [String]?
    var cwd: String?
    var title: String?
    var name: String?
    var turn: Int?
    var state: String?
    var labels: [String: String]?
    var status: AgentStatus?
    var foreground: Foreground?
    var exitCode: Int?
    var createdMs: Int64?
    var statusMs: Int64?

    var id: String { address }

    enum CodingKeys: String, CodingKey {
        case address, argv, cwd, title, name, turn, state, labels, status, foreground
        case exitCode = "exit_code"
        case createdMs = "created_ms"
        case statusMs = "status_ms"
    }
}

/// The states the UI groups by, most urgent first.
enum AgentState: String, CaseIterable, Sendable {
    case blocked, error, done, working, idle

    var title: String {
        switch self {
        case .blocked: "Needs you"
        case .error: "Error"
        case .done: "Done"
        case .working: "Working"
        case .idle: "Idle"
        }
    }

    var symbol: String {
        switch self {
        case .blocked: "hand.raised.fill"
        case .error: "exclamationmark.triangle.fill"
        case .done: "checkmark.circle.fill"
        case .working: "circle.dotted"
        case .idle: "moon.zzz"
        }
    }
}

extension Terminal {
    /// The machine part of a remote address; nil for the hub's own terminals.
    var machine: String? {
        guard let slash = address.firstIndex(of: "/") else { return nil }
        return String(address[..<slash])
    }

    /// The address on its own machine: `t7`.
    var localAddress: String {
        address.split(separator: "/").last.map(String.init) ?? address
    }

    /// Whether the program reports agent status. Plain shells don't.
    var isAgent: Bool { status != nil }

    var isLive: Bool { state == nil || state == "live" }

    var agentState: AgentState? { status.flatMap { AgentState(rawValue: $0.state) } }

    /// "claude-code", "codex", … or the foreground program for a shell.
    var kind: String {
        status?.app ?? foreground?.name ?? argv?.first.map(Self.lastPathComponent) ?? "terminal"
    }

    var displayName: String { name ?? kind }

    /// The Herdr app's workspace / tab, e.g. "webapp / 1".
    var whereLabel: String? { labels?["where"] }

    /// The current working directory, preferring the foreground program's.
    var currentDirectory: String? { foreground?.cwd ?? cwd }

    /// The last two path components, for rows. Handles Windows paths.
    var shortDirectory: String? {
        guard let dir = currentDirectory else { return nil }
        let parts = dir.split(whereSeparator: { $0 == "/" || $0 == "\\" })
        return parts.suffix(2).joined(separator: "/")
    }

    static func lastPathComponent(_ path: String) -> String {
        path.split(whereSeparator: { $0 == "/" || $0 == "\\" }).last.map(String.init) ?? path
    }
}
