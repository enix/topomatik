package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSetup_PopulatesBufferFromLocalFiles(t *testing.T) {
	dir := t.TempDir()
	zonePath := filepath.Join(dir, "zone")
	regionPath := filepath.Join(dir, "region")
	writeFile(t, zonePath, "eu-west\n")
	writeFile(t, regionPath, "  europe  ")

	engine := &FilesDiscoveryEngine{Config: Config{
		"zone":   &File{Path: zonePath},
		"region": &File{Path: regionPath},
	}}

	if err := engine.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = engine.watcher.Close() })

	if got, want := engine.buffer["zone"], "eu-west"; got != want {
		t.Errorf("buffer[zone] = %q, want %q", got, want)
	}
	if got, want := engine.buffer["region"], "europe"; got != want {
		t.Errorf("buffer[region] = %q (TrimSpace expected), want %q", got, want)
	}
}

func TestSetup_AbsolutePathIsLocalRelativeIsRemote(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local")
	writeFile(t, localPath, "x")

	local := &File{Path: localPath}
	engine := &FilesDiscoveryEngine{Config: Config{"local": local}}

	if err := engine.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = engine.watcher.Close() })

	if local.remote {
		t.Errorf("absolute path should not be flagged remote: %q", local.Path)
	}
}

func TestSetup_PropagatesReadError(t *testing.T) {
	engine := &FilesDiscoveryEngine{Config: Config{
		"missing": &File{Path: "/nonexistent/topomatik-test-file"},
	}}

	if err := engine.Setup(context.Background()); err == nil {
		t.Fatal("expected error for missing file")
	}
	if engine.watcher != nil {
		_ = engine.watcher.Close()
	}
}

func TestUpdateDataFromFile_RereadsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "zone")
	writeFile(t, p, "v1")

	engine := &FilesDiscoveryEngine{Config: Config{"zone": &File{Path: p}}}
	if err := engine.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = engine.watcher.Close() })

	if got := engine.buffer["zone"]; got != "v1" {
		t.Fatalf("initial buffer: got %q, want v1", got)
	}

	writeFile(t, p, "v2")
	before := engine.Config["zone"].lastUpdate

	if err := engine.updateDataFromFile("zone"); err != nil {
		t.Fatalf("updateDataFromFile: %v", err)
	}
	if got := engine.buffer["zone"]; got != "v2" {
		t.Errorf("after update: got %q, want v2", got)
	}
	if !engine.Config["zone"].lastUpdate.After(before) {
		t.Errorf("lastUpdate not advanced (before=%v after=%v)", before, engine.Config["zone"].lastUpdate)
	}
}

func TestRead_LocalFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data")
	writeFile(t, p, "hello")

	file := &File{Path: p, remote: false}
	got, err := file.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestRead_LocalFileMissing(t *testing.T) {
	file := &File{Path: filepath.Join(t.TempDir(), "ghost"), remote: false}
	if _, err := file.Read(); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSetup_LocalFileIsAddedToWatcher(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "zone")
	writeFile(t, p, "x")

	engine := &FilesDiscoveryEngine{Config: Config{"zone": &File{Path: p}}}
	if err := engine.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = engine.watcher.Close() })

	if got, want := engine.filesByPath[p], "zone"; got != want {
		t.Errorf("filesByPath[%s] = %q, want %q", p, got, want)
	}
}

func TestSetup_FileWithIntervalIsNotWatched(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "zone")
	writeFile(t, p, "x")

	engine := &FilesDiscoveryEngine{Config: Config{
		"zone": &File{Path: p, Interval: time.Second},
	}}
	if err := engine.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = engine.watcher.Close() })

	if _, ok := engine.filesByPath[p]; ok {
		t.Errorf("polled file should not be in filesByPath, got: %v", engine.filesByPath)
	}
}
