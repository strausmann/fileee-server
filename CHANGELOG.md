## [0.5.0](https://github.com/strausmann/fileee-server/compare/v0.4.1...v0.5.0) (2026-08-14)

### Features

* **handlers:** single-lookup routes for contacts and companies ([3d50033](https://github.com/strausmann/fileee-server/commit/3d5003346ccfe5191bca65c7fc5071c899735af9)), closes [#38](https://github.com/strausmann/fileee-server/issues/38) [#41](https://github.com/strausmann/fileee-server/issues/41)

## [0.4.1](https://github.com/strausmann/fileee-server/compare/v0.4.0...v0.4.1) (2026-08-14)

### Bug Fixes

* **handlers:** page GET /v1/documents via Documents.Query instead of Documents.Diff ([#40](https://github.com/strausmann/fileee-server/issues/40)) ([5398fce](https://github.com/strausmann/fileee-server/commit/5398fce60d3a2bacd498d58ea371464c95afad83)), closes [#39](https://github.com/strausmann/fileee-server/issues/39)

## [0.4.0](https://github.com/strausmann/fileee-server/compare/v0.3.0...v0.4.0) (2026-08-14)

### Features

* **handlers:** opt-in, gated exposure of document extraction attributes ([#38](https://github.com/strausmann/fileee-server/issues/38)) ([a04f713](https://github.com/strausmann/fileee-server/commit/a04f71375e0a913e1e50661e03081194e3a28d0b)), closes [#37](https://github.com/strausmann/fileee-server/issues/37) [Issue-#37-unrelated](https://github.com/strausmann/Issue-/issues/37-unrelated) [#37](https://github.com/strausmann/fileee-server/issues/37)

## [0.3.0](https://github.com/strausmann/fileee-server/compare/v0.2.1...v0.3.0) (2026-08-07)

### Features

* **server:** version-subcommand und fail-fast bei unbekannten argumenten ([#31](https://github.com/strausmann/fileee-server/issues/31)) ([7d0a9cd](https://github.com/strausmann/fileee-server/commit/7d0a9cdfa113b9e15a63d8469682860b6584217e))

### Bug Fixes

* **deploy:** infisical-cli-checksumme zur build-zeit aus checksums.txt verifizieren ([#28](https://github.com/strausmann/fileee-server/issues/28)) ([07a3939](https://github.com/strausmann/fileee-server/commit/07a3939885d7c099b44287735cfb4f91a81efeb7)), closes [#24](https://github.com/strausmann/fileee-server/issues/24)

## [0.2.1](https://github.com/strausmann/fileee-server/compare/v0.2.0...v0.2.1) (2026-08-03)

### Bug Fixes

* **deps:** update module github.com/danielgtaylor/huma/v2 to v2.39.0 ([#20](https://github.com/strausmann/fileee-server/issues/20)) ([d534bae](https://github.com/strausmann/fileee-server/commit/d534bae776566300d1b7b1fe4ec04d486f841b7c)), closes [#963](https://github.com/strausmann/fileee-server/issues/963)

## [0.2.0](https://github.com/strausmann/fileee-server/compare/v0.1.1...v0.2.0) (2026-07-25)

### Features

* **secrets:** secret-safe boot logging for infisical dual-mode ([7e28393](https://github.com/strausmann/fileee-server/commit/7e283939da7d5cba45a071dc4afbc7c283609ae7))
* **server:** startup boot-diagnostics banner + opt-in fileee selfcheck ([78426e0](https://github.com/strausmann/fileee-server/commit/78426e05a86dff0748521715c9864b23664a9df2))

### Bug Fixes

* **server:** resolve version from ldflags/build-info instead of hardcoded const ([276aede](https://github.com/strausmann/fileee-server/commit/276aedeef6e0b4f6a3be56d5d466726e450e70a0)), closes [#17](https://github.com/strausmann/fileee-server/issues/17)

## [0.1.1](https://github.com/strausmann/fileee-server/compare/v0.1.0...v0.1.1) (2026-07-25)

### Bug Fixes

* **deps:** release mit go-fileee v0.1.1 ([415e465](https://github.com/strausmann/fileee-server/commit/415e46555e876bc234fb61e84d8d96c1592f7ce3)), closes [#14](https://github.com/strausmann/fileee-server/issues/14)

## [0.1.0](https://github.com/strausmann/fileee-server/compare/v0.0.0...v0.1.0) (2026-07-25)

### Features

* **server:** initialer fileee-server, extrahiert aus go-fileee; go.mod pin v0.1.0 ([231af7e](https://github.com/strausmann/fileee-server/commit/231af7e3c2b67b1d1fc37562a475c6f06340d0d4))
