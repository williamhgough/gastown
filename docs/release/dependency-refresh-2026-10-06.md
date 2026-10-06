# Stable dependency refresh, 2026-10-06

The fork now selects Beads v1.3.1 in its Go module and E2E container. CI already resolves the bd CLI from go.mod. Dolt container and CI pins move from 2.0.7 to 2.4.1. Go builds and CI select 1.27.1, and golangci-lint selects 2.14.0.

Direct Go dependencies move to their current stable versions within existing module paths, including OpenTelemetry 1.47.0, the logging exporter 0.23.0, testcontainers 0.44.0, glamour 1.0.0, fsnotify 1.10.1, mysql 1.10.1, flock 0.13.1, and testify 1.12.1. Module sums and transitive selections were regenerated with go mod tidy. The separate Dolt snapshots plugin uses mysql 1.10.1. Import-path migrations to other module major versions are outside this refresh.

The stabilized OpenTelemetry logging API uses attribute.KeyValue and attribute.Value. Telemetry now uses those types while retaining existing event names, severity, run correlation, and opt-in output logging.

Nix selects the Beads v1.3.1 tag, refreshed locked inputs, Go 1.27, and ICU. The vendor hash was computed from the updated vendored module tree using Nix, and the Linux package derivation evaluates successfully.

Minimum Beads and Dolt compatibility checks are unchanged. Installed binaries and running town databases were not upgraded by this source change.

## Evidence and remaining gates

- Focused short Go tests passed for internal/beads, internal/deps, internal/telemetry, internal/daemon, and internal/convoy, using canonical Mac temporary paths and Homebrew ICU flags.
- The Dolt snapshots plugin's go test ./... passed.
- make build completed all three binaries into /private/tmp; Go build metadata confirms the updated dependencies.
- Nix derivation evaluation and vendor hashing passed. The full Nix build was interrupted when Docker was closed to reduce Mac load.
- The first full short Go suite exited in internal/cmd because the Mac BuiltProperly guard rejects unmarked test binaries. A rerun with the standard build marker was stopped to reduce Mac load; it is not a passing full-suite result.
- Full CI, lint, and container integration remain required before landing.

Release references: [Beads v1.3.1](https://github.com/gastownhall/beads/releases/tag/v1.3.1), [Dolt v2.4.1](https://github.com/dolthub/dolt/releases/tag/v2.4.1).
