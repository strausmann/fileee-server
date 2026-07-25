package main

import (
	"bytes"
	"errors"
	"log/slog"
	"runtime/debug"
	"strings"
	"testing"
)

// fakeBuildInfoWithDeps baut ein *debug.BuildInfo mit einer einzigen Dependency (path/version),
// als Injektions-Ersatz für debug.ReadBuildInfo — testet, dass logStartupBanner die
// Huma-Version aus den echten Build-Deps abliest statt sie hart zu codieren.
func fakeBuildInfoWithDeps(depPath, depVersion string, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{
			Deps: []*debug.Module{{Path: depPath, Version: depVersion}},
		}, true
	}
}

// TestLogStartupBanner_HappyPath prüft, dass ein einziger Boot-Diagnostics-Log-Eintrag alle vier
// erwarteten Felder trägt: fileee-server-Version, Go-Runtime-Version, Huma-Version (aus den
// Build-Deps abgeleitet, NICHT hartcodiert) und infisical-CLI-Version.
func TestLogStartupBanner_HappyPath(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	infisicalVersionFn := func() (string, error) { return "infisical version 0.41.90", nil }

	logStartupBanner(log, "v0.2.0", fakeBuildInfoWithDeps(humaModulePath, "v2.35.0", true), infisicalVersionFn)

	out := buf.String()
	for _, want := range []string{
		`"version":"v0.2.0"`,
		`"huma_version":"v2.35.0"`,
		`"infisical_cli_version":"infisical version 0.41.90"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Banner-Log fehlt %q, war:\n%s", want, out)
		}
	}
	// go_version wird von runtime.Version() geliefert (z.B. "go1.24.1") — nur das Präfix "go"
	// wird geprüft, um nicht an eine konkrete lokale Go-Version gebunden zu sein.
	if !strings.Contains(out, `"go_version":"go`) {
		t.Errorf("Banner-Log fehlt go_version, war:\n%s", out)
	}
}

// TestLogStartupBanner_HumaVersionUnknownWhenDepMissing prüft, dass ein fehlender
// huma-Dependency-Eintrag (z. B. falscher Modulpfad oder leere Deps) zu "unknown" statt einem
// leeren oder falsch geratenen Wert führt.
func TestLogStartupBanner_HumaVersionUnknownWhenDepMissing(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	logStartupBanner(log, "v0.2.0", fakeBuildInfoWithDeps("github.com/other/module", "v1.0.0", true),
		func() (string, error) { return "x", nil })

	if !strings.Contains(buf.String(), `"huma_version":"unknown"`) {
		t.Errorf("erwartet huma_version=unknown, war:\n%s", buf.String())
	}
}

// TestLogStartupBanner_ReadBuildInfoNotOk prüft, dass ok=false von readBuildInfo (statt eines
// Panics oder Nil-Dereference) ebenfalls zu huma_version=unknown führt.
func TestLogStartupBanner_ReadBuildInfoNotOk(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	logStartupBanner(log, "v0.2.0", fakeBuildInfoWithDeps(humaModulePath, "v2.35.0", false),
		func() (string, error) { return "x", nil })

	if !strings.Contains(buf.String(), `"huma_version":"unknown"`) {
		t.Errorf("erwartet huma_version=unknown bei ok=false, war:\n%s", buf.String())
	}
}

// TestLogStartupBanner_InfisicalUnavailableOnError prüft den Env-Modus-Degradationspfad: liefert
// infisicalVersionFn einen Fehler (Binary fehlt/nicht ausführbar), wird "unavailable" statt eines
// Fehlerabbruchs geloggt — der Banner darf den Boot niemals blockieren.
func TestLogStartupBanner_InfisicalUnavailableOnError(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	logStartupBanner(log, "v0.2.0", fakeBuildInfoWithDeps(humaModulePath, "v2.35.0", true),
		func() (string, error) {
			return "", errors.New("exec: \"/infisical\": stat /infisical: no such file or directory")
		})

	if !strings.Contains(buf.String(), `"infisical_cli_version":"unavailable"`) {
		t.Errorf("erwartet infisical_cli_version=unavailable, war:\n%s", buf.String())
	}
}

// TestLogStartupBanner_InfisicalNilFn prüft, dass ein nil infisicalVersionFn (z. B. wenn ein
// Aufrufer den Check ganz weglassen will) NICHT paniert und ebenfalls "unavailable" liefert.
func TestLogStartupBanner_InfisicalNilFn(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	logStartupBanner(log, "v0.2.0", fakeBuildInfoWithDeps(humaModulePath, "v2.35.0", true), nil)

	if !strings.Contains(buf.String(), `"infisical_cli_version":"unavailable"`) {
		t.Errorf("erwartet infisical_cli_version=unavailable bei nil-Fn, war:\n%s", buf.String())
	}
}
