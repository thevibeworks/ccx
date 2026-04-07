package provider

import (
	"testing"

	"github.com/spf13/viper"
)

func TestDefaultHonorsEnabledProvidersWithoutExistingHomes(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("claude_code_home", "/tmp/missing-claude-home")
	viper.Set("codex_home", "/tmp/missing-codex-home")
	viper.Set("providers.claude-code.enabled", false)
	viper.Set("providers.codex.enabled", true)

	backend := Default()
	if backend.ID() != "codex" {
		t.Fatalf("Default().ID() = %q, want codex", backend.ID())
	}
	if got := backend.Homes(); len(got) != 1 || got[0] != "/tmp/missing-codex-home" {
		t.Fatalf("Default().Homes() = %v, want [/tmp/missing-codex-home]", got)
	}
}
