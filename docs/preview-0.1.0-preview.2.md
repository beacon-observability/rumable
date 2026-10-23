# Historical 0.1.0-preview.2 delivery verification

This records immutable historical artifacts. Current source and usage follow the README/current guides; old packages retain their own included instructions. Date recorded at the time: 2026-10-20 (Asia/Singapore). Only local Git was used; no remote was configured or pushed.

## Build provenance

- Source: ec99b898c5ae177f8ce10a45b3036139321a0e30; local tag v0.1.0-preview.2.
- Directory: dist/0.1.0-preview.2/; aggregate ZIP: dist/0.1.0-preview.2.zip with adjacent .sha256.
- Ten packages: modern Windows/Linux x86/x64/ARM64 and legacy x86/x64. Modern Go 1.26.9, legacy Go 1.20.14, CGO=0.
- SDK 3.3.21 was synchronized and verified before build; each archive includes source/digest metadata. Native OTel modules, .NET Agent, and Collector are separate dependencies.
- dist remains ignored and local. Source, build scripts, documents, and verification records were committed. Earlier preview.1 artifacts were preserved.

All build.json versions/revisions/compilers/targets/binary hashes, PE/ELF machine types, ZIP CRC, Linux execution bits, document links, and notices passed inspection. All ten final executables were byte-identical to preview2-rc3 candidates. Included files matched the pinned source; no uncommitted source marker was used.

SHA256SUMS digest: 6d0c11534cf46b6769e135aff686169d0f0e6ab4a5978ec2b30895edba0f11e1.

Aggregate ZIP digest: 4ef12b284595f96c3655d16f48eef7bdd2309196a5788f7aa00aa22eaef47f73. CRC and file-list checks passed.

## Runtime evidence and limits

- OrbStack tests passed actual native Nginx 1.25.3/1.28.3 and then-supported Apache 2.4.69 OTel flows through Collector/private OTLP intake: gRPC, HTTP/protobuf/token gateway branch, path/site isolation, sampling, independent controls, reload recovery, and detach.
- Minimum RUM attachment passed on Linux x86/Nginx 1.9.4 and Linux x64/then-supported Apache 2.4.23, covering compression/cache/inheritance/concurrent-change refusal/lifecycle/backups.
- Five Linux packages ran version, RUM init, and three historical OTel render/status examples. x86/x64 used emulation on a modern shared kernel.
- Both compiler test/vet, SDK/archive contracts, Go 1.26.9 race, and portable IIS PowerShell snapshot/mock/parse checks passed.
- Windows artifacts were cross-compiled and inspected, not run natively. Windows/IIS/Agent, old kernels, native TLS, and actual receiver ingestion were unverified. Private intake was not production acceptance. SDK/init was unchanged; Chrome was not rerun.

Apache is no longer supported by current source. See [verification](verification.md), [user guide](../USER-GUIDE.md), and [OTel](otel.md). This record was committed after artifact inspection without altering the build commit/tag.

## Archive SHA-256

| File | SHA-256 |
|---|---|
| `rumable_0.1.0-preview.2_linux_386_legacy.tar.gz` | `021bc7618daa07abbe9e40228372634fa90898889ee6de0365084d575e07abef` |
| `rumable_0.1.0-preview.2_linux_amd64_legacy.tar.gz` | `074fd8a660bb3f92b308e7f4e9aa7d2e15f6e237c62da8ad285b9cbb73f6623a` |
| `rumable_0.1.0-preview.2_linux_amd64.tar.gz` | `3cc3a25a3267bdb3a39dbb12a5cd21983cd44c41b46fd69ba837d3b6746bf330` |
| `rumable_0.1.0-preview.2_linux_386.tar.gz` | `420cd5fbf914c764f32ef4bad3977c65648260413ab0b4723c40507d853c90d1` |
| `rumable_0.1.0-preview.2_linux_arm64.tar.gz` | `43fa2b71e89e4e0486d6728cd01ad2050cfe57bc3ebd7817645a64edcd1552d2` |
| `rumable_0.1.0-preview.2_windows_386.zip` | `755e3c49e01e27a1d4c3c83785e41bb28a51caf8c83d98bf431c447f74394b8f` |
| `rumable_0.1.0-preview.2_windows_arm64.zip` | `82fe2f5d3e9e3d6713a0b76ba8074b36c2c72e379798b8faf61bcb7ce2cf5604` |
| `rumable_0.1.0-preview.2_windows_amd64_legacy.zip` | `8b92ae219dcebdde6950a0471387eda1596eb03f6b250cd8b14ab45adb511a1a` |
| `rumable_0.1.0-preview.2_windows_amd64.zip` | `b446608602fbd6cf4d288151ae48da20ee06aef7d3013b03a8de318d7951229a` |
| `rumable_0.1.0-preview.2_windows_386_legacy.zip` | `e37bee4bffad666ec08729d2c5b080d0c0323ebb0e795fc9bce7d6a6a2d28fbf` |
