import Foundation
import Testing
@testable import HerdrUI

/// Decoding and event handling against payloads recorded from a live core
/// (bridge/testdata, bundled as the "testdata" folder).
@MainActor
struct FixtureTests {
    func fixture(_ name: String) throws -> Data {
        let bundle = Bundle(for: BundleToken.self)
        let url = try #require(bundle.url(forResource: name, withExtension: nil, subdirectory: "testdata"))
        return try Data(contentsOf: url)
    }

    func result<T: Decodable>(_ name: String, as: T.Type) throws -> T {
        try JSONDecoder().decode(ResultEnvelope<T>.self, from: fixture(name)).result
    }

    @Test func decodesTerminalList() throws {
        let list = try result("terminal.list.json", as: TerminalList.self)
        #expect(!list.terminals.isEmpty)
        let agent = try #require(list.terminals.first { $0.isAgent })
        #expect(agent.agentState != nil)
        #expect(agent.labels?["where"] != nil, "labels keys must survive decoding unchanged")
        let remote = try #require(list.terminals.first { $0.machine != nil })
        #expect(remote.machine == "devbox")
        #expect(list.machines?.first?.name == "devbox")
    }

    @Test func decodesReadAndGet() throws {
        let read = try result("terminal.read.t10.json", as: ReadResult.self)
        #expect(!read.text.isEmpty)
        let t = try result("terminal.get.t10.json", as: Terminal.self)
        #expect(t.address == "t10")
    }

    @Test func derivesWorkspacesFromLabels() throws {
        let list = try result("terminal.list.json", as: TerminalList.self)
        let store = HerdrStore()
        store.load(list.terminals)
        let spaces = store.spaces(on: nil)
        #expect(!spaces.isEmpty)
        #expect(spaces.allSatisfy { $0.id.hasPrefix("sp") && !$0.tabs.isEmpty && $0.tabs.allSatisfy { $0.id.hasPrefix("tb") } })
        #expect(!store.directories(on: nil).isEmpty)
        #expect(NewAgentView.isValidName("fixer-2"))
        #expect(!NewAgentView.isValidName("Bad Name"))
    }

    @Test func appliesRecordedEventStream() throws {
        let lines = try String(decoding: fixture("events.subscribe.jsonl"), as: UTF8.self)
            .split(separator: "\n").map { Data($0.utf8) }
        let store = HerdrStore()

        store.apply(lines[0]) // snapshot
        let initial = store.terminals.count
        #expect(initial > 0)
        #expect(store.terminals["t11"] == nil)

        store.apply(lines[1]) // created t11
        #expect(store.terminals["t11"]?.name == "scratch-fixture")
        #expect(store.terminals.count == initial + 1)

        for line in lines[2...] { store.apply(line) } // labels…, finished, removed, heartbeat
        #expect(store.terminals["t11"] == nil)
        #expect(store.terminals.count == initial)
        #expect((store.revisions["t11"] ?? 0) >= 2)
    }

    @Test func decodesCoreErrorPassedThroughBridge() throws {
        struct Env: Decodable { var error: BridgeError }
        let err = try JSONDecoder().decode(Env.self, from: fixture("error.unknown_terminal.json")).error
        #expect(err.errorKind == "not_found")
        #expect(err.message.contains("t9999"))
    }

    @Test func decodesBridgeOwnError() throws {
        struct Env: Decodable { var error: BridgeError }
        let data = Data(#"{"error":{"kind":"method_not_allowed","message":"the bridge does not forward server.stop"}}"#.utf8)
        #expect(try JSONDecoder().decode(Env.self, from: data).error.errorKind == "method_not_allowed")
    }
}

private final class BundleToken {}

private struct ResultEnvelope<T: Decodable>: Decodable { var result: T }

struct TerminalHelperTests {
    @Test func shortDirectoryHandlesBothPathStyles() {
        #expect(Terminal(address: "t1", cwd: "/Users/dev/src/herdr-ui").shortDirectory == "src/herdr-ui")
        #expect(Terminal(address: "devbox/t3", cwd: "C:\\Users\\dev\\api-server").shortDirectory == "dev/api-server")
    }

    @Test func machineFromAddress() {
        #expect(Terminal(address: "t1").machine == nil)
        #expect(Terminal(address: "erics-macbook-pro/t3").machine == "erics-macbook-pro")
    }

    @Test @MainActor func parsesTerminalLinks() {
        #expect(AgentsView.address(from: URL(string: "herdr://terminal/simrig/t9")!) == "simrig/t9")
        #expect(AgentsView.address(from: URL(string: "herdr://terminal/t1")!) == "t1")
        #expect(AgentsView.address(from: URL(string: "herdr://terminal/")!) == nil)
        #expect(AgentsView.address(from: URL(string: "https://terminal/t1")!) == nil)
        #expect(Terminal(address: "simrig/t9").localAddress == "t9")
    }

    @Test @MainActor func approveKeysPerAgent() {
        #expect(PushManager.approveKeys(for: "claude-code") == ["1"])
        #expect(PushManager.approveKeys(for: "codex") == ["y"])
        #expect(PushManager.approveKeys(for: nil) == ["enter"])
        #expect(PushManager.denyKeys == ["esc"])
    }

    @Test func parsesPairLinks() {
        #expect(PairLink.hub(from: "herdr://pair?hub=https%3A%2F%2Fhub.x.ts.net%3A8443")?.absoluteString == "https://hub.x.ts.net:8443")
        #expect(PairLink.hub(from: "hub.x.ts.net")?.absoluteString == "https://hub.x.ts.net:8443")
        #expect(PairLink.hub(from: "herdr://terminal/t1") == nil)
        #expect(PairLink.hub(from: "herdr://pair") == nil)
    }

    @Test func normalizesHubAddress() {
        #expect(ConnectView.normalize("simrig.x.ts.net")?.absoluteString == "https://simrig.x.ts.net:8443")
        #expect(ConnectView.normalize("https://simrig.x.ts.net:9000/v1/health")?.absoluteString == "https://simrig.x.ts.net:9000")
        #expect(ConnectView.normalize("  ") == nil)
    }
}
