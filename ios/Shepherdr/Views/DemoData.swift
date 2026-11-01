#if DEBUG
import Foundation

/// Made-up agents for App Store screenshots: launch with `-demo YES`.
/// No hub is contacted.
@MainActor
enum DemoData {
    static var isOn: Bool { UserDefaults.standard.bool(forKey: "demo") }

    private static let now = Int64(Date().timeIntervalSince1970 * 1000)

    static let health: Health = try! JSONDecoder().decode(Health.self, from: Data(#"""
    {"bridge_version":"0.1.0","push_enabled":true,
     "core":{"core":{"id":"demo","name":"studio"},"version":"0.1.16"},
     "machines":{"machines":[{"name":"laptop","state":"connected"},{"name":"devbox","state":"connected"}]}}
    """#.utf8))

    static let terminals: [Terminal] = [
        Terminal(address: "laptop/t4", cwd: "/Users/sam/src/storefront", title: "Migrate checkout to the new payments API", turn: 6,
                 state: "live", labels: ["where": "storefront / 1", "herdr.space": "spa1", "herdr.tab": "tba1"],
                 status: AgentStatus(app: "claude-code", state: "blocked", kind: "permission", msg: "Run npm test -- checkout?"),
                 statusMs: now - 20_000),
        Terminal(address: "t2", cwd: "/home/sam/api", title: "Add rate limiting to /v2/orders", turn: 3, state: "live",
                 labels: ["where": "api / 1", "herdr.space": "spa2", "herdr.tab": "tba2"],
                 status: AgentStatus(app: "codex", state: "working", progress: 60), statusMs: now - 60_000),
        Terminal(address: "devbox/t7", cwd: "/srv/ingest", title: "Fix flaky retry test", turn: 2, state: "live",
                 labels: ["where": "ingest / 2", "herdr.space": "spa3", "herdr.tab": "tba3"],
                 status: AgentStatus(app: "claude-code", state: "working"), statusMs: now - 90_000),
        Terminal(address: "laptop/t9", cwd: "/Users/sam/src/docs-site", title: "Write the v3 migration guide", turn: 4, state: "live",
                 labels: ["where": "docs / 1", "herdr.space": "spa4", "herdr.tab": "tba4"],
                 status: AgentStatus(app: "claude-code", state: "done"), statusMs: now - 300_000),
        Terminal(address: "t5", cwd: "/home/sam/infra", title: "Bump Terraform providers", turn: 1, state: "live",
                 labels: ["where": "infra / 1", "herdr.space": "spa5", "herdr.tab": "tba5"],
                 status: AgentStatus(app: "codex", state: "idle"), statusMs: now - 900_000),
    ]

    static func screen(for address: String) -> ReadResult {
        ReadResult(text: screens[address] ?? screens["laptop/t4"]!, complete: true, how: "screen", why: nil)
    }

    private static let screens: [String: String] = [
        "laptop/t4": """
        ╭───────────────────────────────────────╮
        │ ✻ Claude Code                         │
        │   ~/src/storefront · main             │
        ╰───────────────────────────────────────╯

        > The payments team shipped the v2 client.
          Move checkout over, keep v1 behind a flag.

        ● Search(pattern: "legacyPayments", path: "src")
          ⎿  Found 4 files

        ● Read src/checkout/useCheckout.ts (140 lines)

        ● Plan:
          1. Add a payments_v2 feature flag
          2. Switch PaymentForm + useCheckout to v2
          3. Keep v1 as the fallback
          4. Run the checkout tests

        ● Update(src/lib/flags.ts)
          ⎿  Updated with 3 additions

        > Migrate checkout to the new payments API

        ● Read src/checkout/PaymentForm.tsx (212 lines)
        ● Read src/lib/payments/client.ts (88 lines)

        ● I'll switch PaymentForm to the v2 client and keep the
          old one behind a feature flag for the rollout.

        ● Update(src/checkout/PaymentForm.tsx)
          ⎿  Updated with 24 additions and 9 removals

        ● Update(src/lib/payments/client.ts)
          ⎿  Updated with 41 additions

        ╭────────────────────────────────────────────╮
        │ Bash command                               │
        │                                            │
        │   npm test -- checkout                     │
        │   Run the checkout test suite              │
        │                                            │
        │ Do you want to proceed?                    │
        │ ❯ 1. Yes                                   │
        │   2. Yes, and don't ask again for npm test │
        │   3. No, and tell Claude what to do        │
        ╰────────────────────────────────────────────╯
        """,
        "t2": """
        › Add rate limiting to /v2/orders

        • Explored
          └ Read routes/orders.go, middleware/auth.go

        • Added a token-bucket limiter keyed by API key
          └ middleware/ratelimit.go (+96)

        • Wired it into the /v2/orders group
          └ routes/orders.go (+4 -1)

        • Running go test ./...
        """,
    ]

    /// What the live terminal shows in demo mode: a colorful agent session.
    static let liveANSI: String = [
        "\u{1b}[90m~/src/storefront\u{1b}[0m \u{1b}[32mmain\u{1b}[0m \u{1b}[1m❯\u{1b}[0m claude",
        "",
        "\u{1b}[1;38;5;141m╭─ Claude Code ─────────────────────────╮\u{1b}[0m",
        "\u{1b}[1;38;5;141m│\u{1b}[0m storefront · \u{1b}[32mmain\u{1b}[0m · 3 files changed    \u{1b}[1;38;5;141m│\u{1b}[0m",
        "\u{1b}[1;38;5;141m╰───────────────────────────────────────╯\u{1b}[0m",
        "",
        "\u{1b}[1m> \u{1b}[0mMigrate checkout to the new payments API",
        "",
        "\u{1b}[38;5;114m●\u{1b}[0m Update(\u{1b}[1msrc/checkout/PaymentForm.tsx\u{1b}[0m)",
        "  \u{1b}[90m⎿\u{1b}[0m  Updated with \u{1b}[32m24 additions\u{1b}[0m and \u{1b}[31m9 removals\u{1b}[0m",
        "     \u{1b}[90m41\u{1b}[0m \u{1b}[31m- const client = legacyPayments()\u{1b}[0m",
        "     \u{1b}[90m41\u{1b}[0m \u{1b}[32m+ const client = payments.v2({ flag })\u{1b}[0m",
        "",
        "\u{1b}[38;5;114m●\u{1b}[0m Bash(\u{1b}[1mnpm test -- checkout\u{1b}[0m)",
        "  \u{1b}[90m⎿\u{1b}[0m  \u{1b}[32m✓\u{1b}[0m PaymentForm renders saved cards",
        "     \u{1b}[32m✓\u{1b}[0m submits through payments v2",
        "     \u{1b}[32m✓\u{1b}[0m falls back when the flag is off",
        "     \u{1b}[1;32mTests: 18 passed\u{1b}[0m, 18 total",
        "",
        "\u{1b}[38;5;114m●\u{1b}[0m Bash(\u{1b}[1mnpm run typecheck\u{1b}[0m)",
        "  \u{1b}[90m⎿\u{1b}[0m  \u{1b}[32mNo errors\u{1b}[0m",
        "",
        "\u{1b}[38;5;114m●\u{1b}[0m Done. Checkout now uses payments v2:",
        "  • \u{1b}[1mpayments_v2\u{1b}[0m flag, on for staff",
        "  • v1 stays as the fallback",
        "  • \u{1b}[32m18/18\u{1b}[0m checkout tests pass",
        "",
        "  Want me to open a PR?",
        "",
        "\u{1b}[90m────────────────────────────────────────\u{1b}[0m",
        "\u{1b}[1m>\u{1b}[0m \u{1b}[7m \u{1b}[0m",
    ].joined(separator: "\r\n")
}
#endif
