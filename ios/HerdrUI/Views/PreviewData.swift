import Foundation

/// Sample data for SwiftUI previews.
@MainActor
enum PreviewData {
    static let terminals: [Terminal] = [
        Terminal(address: "laptop/t3", cwd: "/Users/me/src/webapp", title: "Fix flaky tests",
                 turn: 3, state: "live", labels: ["where": "webapp / 1"],
                 status: AgentStatus(app: "claude-code", state: "blocked", kind: "permission", msg: "Run npm test?")),
        Terminal(address: "t9", cwd: "C:\\src\\api", title: "Add pagination", turn: 4, state: "live",
                 labels: ["where": "api / 1"], status: AgentStatus(app: "codex", state: "working", progress: 40)),
        Terminal(address: "laptop/t1", cwd: "/Users/me", title: "Update dependencies", turn: 3,
                 state: "live", status: AgentStatus(app: "claude-code", state: "done")),
        Terminal(address: "t3", cwd: "C:\\src", state: "live",
                 foreground: Foreground(argv: ["powershell.exe"], cwd: "C:\\src", name: "powershell.exe", pid: 1)),
    ]

    static let store: HerdrStore = {
        let s = HerdrStore()
        s.load(terminals)
        return s
    }()
}
