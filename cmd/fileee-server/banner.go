package main

import (
	"log/slog"
	"runtime"
	"runtime/debug"
)

// humaModulePath identifiziert die Huma-Dependency in den Build-Info-Deps — genutzt, um ihre
// Version im Startup-Banner auszuweisen, OHNE sie hart zu codieren (Renovate hebt sie
// unabhängig von fileee-server an; ein hartcodierter Wert würde wie die alte
// `const version = "0.1.0"` (#17) sofort veralten).
const humaModulePath = "github.com/danielgtaylor/huma/v2"

// logStartupBanner loggt EINEN strukturierten Boot-Diagnostics-Eintrag: fileee-servers eigene
// Version (appVersion, siehe resolveVersion in version.go), die Go-Runtime-Version
// (runtime.Version()), die Huma-Version (aus readBuildInfo().Deps abgeleitet, "unknown" wenn
// nicht auffindbar) und die infisical-CLI-Version (über infisicalVersionFn ermittelt,
// "unavailable" wenn die Binary fehlt/fehlschlägt/nicht abgefragt werden kann — z. B. im reinen
// Env-Modus ohne Infisical-Dual-Mode). readBuildInfo und infisicalVersionFn sind injiziert,
// damit Tests beide Pfade (gefunden/nicht gefunden, verfügbar/nicht verfügbar) ohne echten Go-
// Build bzw. echte infisical-Binary abdecken können; infisicalVersionFn darf nil sein.
func logStartupBanner(
	log *slog.Logger,
	appVersion string,
	readBuildInfo func() (*debug.BuildInfo, bool),
	infisicalVersionFn func() (string, error),
) {
	humaVersion := "unknown"
	if readBuildInfo != nil {
		if bi, ok := readBuildInfo(); ok && bi != nil {
			for _, dep := range bi.Deps {
				if dep != nil && dep.Path == humaModulePath {
					humaVersion = dep.Version
					break
				}
			}
		}
	}

	infisicalVersion := "unavailable"
	if infisicalVersionFn != nil {
		if v, err := infisicalVersionFn(); err == nil && v != "" {
			infisicalVersion = v
		}
	}

	log.Info("fileee-server boot-diagnostics",
		"version", appVersion,
		"go_version", runtime.Version(),
		"huma_version", humaVersion,
		"infisical_cli_version", infisicalVersion,
	)
}
