# Historical 0.1.0-preview.3 delivery verification

Date recorded at the time: 2026-10-20 (Asia/Singapore). Only local Git was used; no remote was configured or pushed.

## Build provenance

- Source: 0ee5e1c0ae2b04bee60a44f9a42a0a9838e90093; local tag v0.1.0-preview.3.
- Directory: dist/0.1.0-preview.3/; aggregate ZIP: dist/0.1.0-preview.3.zip with adjacent .sha256.
- Ten Windows/Linux modern x86/x64/ARM64 and legacy x86/x64 packages; no macOS package. Modern Go 1.26.9, legacy Go 1.20.14, CGO=0.
- Includes install-otel/uninstall and independent Nginx/IIS components. Apache was removed. Modules/Agents are obtained separately; Collector is not installed by the package.
- SDK 3.3.21 remained synchronized/verified. Enable upgrades a site's browser assets; OTel-only operations preserve its browser version.
- dist stayed local/ignored. Source and records were committed. Earlier preview.1/.2 artifacts were not overwritten and their checksums were rechecked.

## Artifact checks

The clean pinned source passed both compiler test/vet and SDK/archive contracts, with source rechecked at build end. build.json records actual revision/version/architecture/compiler/binary digest.

All ten archives passed SHA256SUMS, ZIP CRC, PE/ELF types, execution bits, and local document links. Included documents/scripts/examples/notices matched source bytes. Final executables were byte-identical to preview3-rc1 candidates. Aggregate ZIP entries and CRC also passed.

SHA256SUMS digest: 6248531227704d57103cb7e3fad6fd64de53fa4963939b6ec1c32d4f0a97972a.

Aggregate ZIP digest: 7fbdf61c4ff6e336e2045e9ea7e41cd7c81c1b5692a006943bde6d8b2a66e66b; size 32,487,174 bytes.

## Runtime evidence and limits

- Five Linux packages ran version, both RUM reporting templates, and both OTel render/status examples. x86/x64 used emulation on a modern kernel.
- Modern Linux x86 passed Nginx 1.9.4 RUM; legacy Linux x64 passed complete attachment, including compression/cache/inheritance/independent controls/rollback/concurrent edits/failure recovery/exact undo/backups.
- Linux ARM64 passed Nginx 1.25.3/module 0.1.0 and 1.28.3/module 0.1.2 with Collector 0.163.1/private OTLP intake. Tests covered actual spans, both transport branches, sampling/path/site/shared exporter, unavailable receiver during RUM actions, split-file attachment, partial recovery retry, rollback, and uninstall.
- A temporary in-container APT repository exercised real download/digest/extraction/idempotency/dependency cleanup. This did not validate every remote repository or distribution ABI.
- PowerShell 7.4/Linux amd64 passed snapshots, IIS OTel mocks, parsing, and real Linux CLI portable boundaries. This was not IIS API execution.
- Windows archives were cross-compiled/inspected only. Native Windows/IIS/Agent, old kernels, real receivers, and actual application ingestion remained unverified. SDK/init/page interaction was unchanged, so Chrome was not rerun.

See [user guide](../USER-GUIDE.md), [dependency management](otel-installation.md), [verification](verification.md), and [Windows acceptance](windows-acceptance.md). This record did not change the build source/tag.

## Archive SHA-256

| File | SHA-256 |
|---|---|
| `rumable_0.1.0-preview.3_linux_386.tar.gz` | `0313307da5e5e50e717e9aa907f9432fc0c5b9599158cdc9dda0b6c479f6aae1` |
| `rumable_0.1.0-preview.3_linux_386_legacy.tar.gz` | `f73854046a9479d2cdf3dfb4d0bc63abb1a42d0485e29ebea653fa0db27f143d` |
| `rumable_0.1.0-preview.3_linux_amd64.tar.gz` | `82fbb964291b09a1588bdeb1fac9933a4f13b647fc3ceff9ade0a02938f7834b` |
| `rumable_0.1.0-preview.3_linux_amd64_legacy.tar.gz` | `f408b9cd1465b5cc2fd6375280b17c2926d170534b71cd171738b9d4b7c3a33a` |
| `rumable_0.1.0-preview.3_linux_arm64.tar.gz` | `9c7903162a19d546e9a88f5ebb4e02b8127dcc5a71bbabf727ce94c3c9d53032` |
| `rumable_0.1.0-preview.3_windows_386.zip` | `e173b356d5184cba14b5e7ba02c5cb728d9a6e0c414bbb90c6bd8b38ba3fa591` |
| `rumable_0.1.0-preview.3_windows_386_legacy.zip` | `81db9ec8596f6c5ddbea84d49aff6db761cfa6d9a93e9dd5549e8d48f5b12933` |
| `rumable_0.1.0-preview.3_windows_amd64.zip` | `a98437746281a462e8647fdafc4f8b3979df5fa19bf14ef37a2f9a4064fdf036` |
| `rumable_0.1.0-preview.3_windows_amd64_legacy.zip` | `32a7b457812d8bc73787c8d08ee0a26e318a245ca15767d1478af006ff7970b1` |
| `rumable_0.1.0-preview.3_windows_arm64.zip` | `c9ae62dc2673f9851306041ca8f1351b2ed567d731f4ad511bbd9aeb94e797ba` |
