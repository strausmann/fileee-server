package main

import (
	"runtime/debug"
	"testing"
)

// fakeBuildInfo baut ein *debug.BuildInfo mit genau den Feldern, die resolveVersionFrom
// auswertet (Main.Version), als Injektions-Ersatz für debug.ReadBuildInfo in Tests.
func fakeBuildInfo(mainVersion string, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}}, true
	}
}

// TestResolveVersionFrom deckt die drei Prioritätsstufen ab: ldflags-Override zuerst, dann
// runtime/debug.ReadBuildInfo (außer beim devel-Sentinel "(devel)", der wie leer behandelt
// wird), zuletzt der statische Default "dev".
func TestResolveVersionFrom(t *testing.T) {
	cases := []struct {
		name           string
		ldflagsVersion string
		readBuildInfo  func() (*debug.BuildInfo, bool)
		want           string
	}{
		{
			name:           "ldflags override gewinnt immer",
			ldflagsVersion: "v0.2.0",
			readBuildInfo:  fakeBuildInfo("v9.9.9", true),
			want:           "v0.2.0",
		},
		{
			name:           "build-info Main.Version als Fallback",
			ldflagsVersion: "",
			readBuildInfo:  fakeBuildInfo("v1.2.3", true),
			want:           "v1.2.3",
		},
		{
			name:           "devel-Sentinel wird wie leer behandelt",
			ldflagsVersion: "",
			readBuildInfo:  fakeBuildInfo("(devel)", true),
			want:           "dev",
		},
		{
			name:           "leere Main.Version faellt auf dev zurueck",
			ldflagsVersion: "",
			readBuildInfo:  fakeBuildInfo("", true),
			want:           "dev",
		},
		{
			name:           "ReadBuildInfo liefert ok=false",
			ldflagsVersion: "",
			readBuildInfo:  fakeBuildInfo("v1.2.3", false),
			want:           "dev",
		},
		{
			name:           "ReadBuildInfo ist nil",
			ldflagsVersion: "",
			readBuildInfo:  nil,
			want:           "dev",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersionFrom(tc.ldflagsVersion, tc.readBuildInfo)
			if got != tc.want {
				t.Errorf("resolveVersionFrom(%q, ...) = %q, erwartet %q", tc.ldflagsVersion, got, tc.want)
			}
		})
	}
}

// TestResolveVersion prüft nur, dass die öffentliche Wrapper-Funktion nicht paniert und ein
// nicht-leeres Ergebnis liefert — die eigentliche Logik ist bereits über resolveVersionFrom
// vollständig abgedeckt (resolveVersion ruft sie lediglich mit der package-globalen Variable
// `version` und der echten debug.ReadBuildInfo auf).
func TestResolveVersion(t *testing.T) {
	if got := resolveVersion(); got == "" {
		t.Error("resolveVersion() darf nie einen leeren String liefern")
	}
}
