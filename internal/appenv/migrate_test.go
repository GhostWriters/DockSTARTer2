package appenv

import (
	"DockSTARTer2/internal/config"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvMigrate_PlainName(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("OLD_NAME='hello'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := EnvMigrate(context.Background(), "OLD_NAME", "NEW_NAME", conf); err != nil {
		t.Fatalf("EnvMigrate failed: %v", err)
	}

	val, _ := Get("NEW_NAME", envFile)
	if val != "hello" {
		t.Errorf("NEW_NAME = %q; want %q", val, "hello")
	}
	if oldVal, _ := Get("OLD_NAME", envFile); oldVal != "" {
		t.Errorf("OLD_NAME still present with value %q; want unset", oldVal)
	}
}

// TestEnvMigrate_PipeAlternation is the regression test for the bug this
// session found: fromVar can be a real regex (e.g. an "A|B" alternation,
// as used throughout DockSTARTer-Templates' .migrate files, mirroring DS1's
// env_migrate.sh grep -P behavior), not just a literal variable name.
func TestEnvMigrate_PipeAlternation(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	// Only the second alternative actually exists in the file.
	if err := os.WriteFile(envFile, []byte("OLD_NAME_B='world'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := EnvMigrate(context.Background(), "OLD_NAME_A|OLD_NAME_B", "NEW_NAME", conf); err != nil {
		t.Fatalf("EnvMigrate failed: %v", err)
	}

	val, _ := Get("NEW_NAME", envFile)
	if val != "world" {
		t.Errorf("NEW_NAME = %q; want %q (pipe-alternation match failed)", val, "world")
	}
	if oldVal, _ := Get("OLD_NAME_B", envFile); oldVal != "" {
		t.Errorf("OLD_NAME_B still present with value %q; want unset", oldVal)
	}
}

func TestEnvMigrate_SkipsIfTargetAlreadySet(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	content := "OLD_NAME='stale'\nNEW_NAME='already-set'\n"
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := EnvMigrate(context.Background(), "OLD_NAME", "NEW_NAME", conf); err != nil {
		t.Fatalf("EnvMigrate failed: %v", err)
	}

	val, _ := Get("NEW_NAME", envFile)
	if val != "already-set" {
		t.Errorf("NEW_NAME = %q; want unchanged %q", val, "already-set")
	}
	// Source is left alone when the target already has a value.
	if oldVal, _ := Get("OLD_NAME", envFile); oldVal != "stale" {
		t.Errorf("OLD_NAME = %q; want left as %q since target was already set", oldVal, "stale")
	}
}

// TestEnvMigrate_AppFilePrefix is the regression test for a bug found
// alongside the multi-service work: an "appname:VAR" fromVar/toVar targets
// the app's .env.app.<appname> file (as every .migrate file in
// DockSTARTer-Templates actually assumes), not the global .env -- but
// EnvMigrate was resolving the prefix via AppInstanceFile(ctx, appname,
// ".env"), which always pointed at the global .env instance file no matter
// what appname was. This also covers a multi-service per-service file
// (e.g. "immich-database:VAR"), which has no template of its own to
// resolve against and must work purely from the literal file name.
func TestEnvMigrate_AppFilePrefix(t *testing.T) {
	tmpDir := t.TempDir()
	appFile := filepath.Join(tmpDir, ".env.app.immich-database")
	if err := os.WriteFile(appFile, []byte("OLD_NAME='hello'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := EnvMigrate(context.Background(), "immich-database:OLD_NAME", "immich-database:NEW_NAME", conf); err != nil {
		t.Fatalf("EnvMigrate failed: %v", err)
	}

	val, _ := Get("NEW_NAME", appFile)
	if val != "hello" {
		t.Errorf("NEW_NAME = %q; want %q", val, "hello")
	}
	if oldVal, _ := Get("OLD_NAME", appFile); oldVal != "" {
		t.Errorf("OLD_NAME still present with value %q; want unset", oldVal)
	}

	// The global .env must be untouched -- confirms the migration didn't
	// silently fall back to it.
	envFile := filepath.Join(tmpDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		t.Errorf(".env was created/written; migration should only have touched .env.app.immich-database")
	}
}

func TestEnvMigrate_NoMatchIsNoop(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := EnvMigrate(context.Background(), "OLD_NAME_A|OLD_NAME_B", "NEW_NAME", conf); err != nil {
		t.Fatalf("EnvMigrate failed: %v", err)
	}

	if val, _ := Get("NEW_NAME", envFile); val != "" {
		t.Errorf("NEW_NAME = %q; want unset since neither source variable exists", val)
	}
}
