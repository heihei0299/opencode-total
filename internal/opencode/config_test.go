package opencode

import "testing"

func TestResolveDataDirPriority(t *testing.T) {
	for _, test := range []struct {
		name     string
		value    string
		envValue string
		want     string
	}{
		{name: "explicit", value: " cli-dir ", envValue: "env-dir", want: "cli-dir"},
		{name: "environment", envValue: " env-dir ", want: "env-dir"},
		{name: "default", want: defaultDataDir},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveDataDir(test.value, test.envValue); got != test.want {
				t.Fatalf("ResolveDataDir(%q, %q) = %q, want %q", test.value, test.envValue, got, test.want)
			}
		})
	}
}
