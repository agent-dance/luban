package theme

import "testing"

func TestConfigureUsesTerminalDefaultsAsTheSafeDefault(t *testing.T) {
	t.Cleanup(func() { Configure("system") })

	for _, name := range []string{"", "system", "default", "auto", "light", "unknown"} {
		Configure(name)
		palette := Current()
		if palette.Name != "system" || palette.Background != "" || palette.Foreground != "" {
			t.Fatalf("Configure(%q) = %+v, want terminal-owned foreground and background", name, palette)
		}
	}
}

func TestConfigureKeepsExplicitDarkTheme(t *testing.T) {
	t.Cleanup(func() { Configure("system") })

	for _, name := range []string{"burgundy", "wine", "crimson", "dark"} {
		Configure(name)
		palette := Current()
		if palette.Name != "burgundy" || palette.Background != "#1A090B" || palette.Foreground != "#F7E6E8" {
			t.Fatalf("Configure(%q) = %+v, want explicit burgundy palette", name, palette)
		}
	}
}
