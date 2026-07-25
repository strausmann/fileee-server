package main

import "runtime/debug"

// version ist der ldflags-Override der fileee-server-Version, gesetzt beim CI-/Docker-Build via
// `-ldflags "-X main.version=vX.Y.Z"` (siehe deploy/Dockerfile). Leer bei lokalem `go build`/
// `go run`/`go test` ohne diesen Override — resolveVersion greift in diesem Fall auf
// runtime/debug.ReadBuildInfo zurück (Fixes #17: die vorherige `const version = "0.1.0"` in
// server.go lief jedem Release hinterher, weil sie nie manuell nachgezogen wurde).
var version string

// resolveVersion liefert die effektive fileee-server-Version für die OpenAPI `info.version`
// (api.go), den Startup-Banner (banner.go) und potenziell weitere Stellen, die EINE einzige
// Versionsquelle brauchen. Ruft resolveVersionFrom mit dem package-globalen ldflags-Override
// und der echten runtime/debug.ReadBuildInfo auf.
func resolveVersion() string {
	return resolveVersionFrom(version, debug.ReadBuildInfo)
}

// resolveVersionFrom ist der injizierbare Kern von resolveVersion (testbar ohne echten
// ldflags-Build): ldflagsVersion gewinnt, wenn nicht leer. Sonst wird readBuildInfo befragt —
// dessen Main.Version wird verwendet, AUSSER sie ist leer oder der Go-Toolchain-eigene
// devel-Sentinel "(devel)" (gesetzt, wenn das Modul ohne `go install modul@version` gebaut
// wurde, z. B. lokal per `go build`/`go test`/`go run` im Checkout) — in beiden Fällen greift
// der letzte Fallback "dev". readBuildInfo darf nil sein (z. B. in Tests, die den
// build-info-Pfad gar nicht prüfen wollen).
func resolveVersionFrom(ldflagsVersion string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ldflagsVersion != "" {
		return ldflagsVersion
	}
	if readBuildInfo != nil {
		if bi, ok := readBuildInfo(); ok && bi != nil {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				return bi.Main.Version
			}
		}
	}
	return "dev"
}
