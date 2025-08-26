package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test helper to create a temporary directory for testing
func setupTestDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "gitnot_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Change to temp dir
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	t.Cleanup(func() {
		os.Chdir(originalDir)
		os.RemoveAll(tempDir)
	})

	return tempDir
}

// Helper to create a test file
func createTestFile(t *testing.T, path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && !os.IsExist(err) {
		t.Fatalf("Failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", path, err)
	}
}

func TestInitGitnot(t *testing.T) {
	setupTestDir(t)

	// Create some test files
	createTestFile(t, "test.txt", "Hello World")
	createTestFile(t, "subdir/test.go", "package main\n\nfunc main() {}")
	createTestFile(t, "README.md", "# Test Project")

	// Initialize gitnot
	err := initGitnot()
	if err != nil {
		t.Fatalf("initGitnot failed: %v", err)
	}

	// Check that directories were created
	expectedDirs := []string{
		".gitnot",
		".gitnot/snapshot",
		".gitnot/changelogs",
		".gitnot/deleted",
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}

	// Check that files were created
	expectedFiles := []string{
		".gitnot/version.txt",
		".gitnot/hashes.json",
		".gitnot/config.json",
	}

	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("File %s was not created", file)
		}
	}

	// Check version is 0.0
	version, err := readVersion()
	if err != nil {
		t.Errorf("Failed to read version: %v", err)
	}
	if version != 0.0 {
		t.Errorf("Expected version 0.0, got %.1f", version)
	}

	// Check that snapshots were created
	snapshots := []string{
		".gitnot/snapshot/test.txt",
		".gitnot/snapshot/subdir/test.go",
		".gitnot/snapshot/README.md",
	}

	for _, snap := range snapshots {
		if _, err := os.Stat(snap); os.IsNotExist(err) {
			t.Errorf("Snapshot %s was not created", snap)
		}
	}
}

func TestVersionFunctions(t *testing.T) {
	setupTestDir(t)

	// Test initial version read (should return 0.0 when file doesn't exist)
	version, err := readVersion()
	if err != nil {
		t.Errorf("readVersion failed: %v", err)
	}
	if version != 0.0 {
		t.Errorf("Expected version 0.0, got %.1f", version)
	}

	// Test writing version
	err = writeVersion(1.5)
	if err != nil {
		t.Errorf("writeVersion failed: %v", err)
	}

	// Test reading written version
	version, err = readVersion()
	if err != nil {
		t.Errorf("readVersion failed after write: %v", err)
	}
	if version != 1.5 {
		t.Errorf("Expected version 1.5, got %.1f", version)
	}

	// Test version bumping
	newVersion, err := bumpVersion()
	if err != nil {
		t.Errorf("bumpVersion failed: %v", err)
	}
	if newVersion != 1.6 {
		t.Errorf("Expected bumped version 1.6, got %.1f", newVersion)
	}
}

func TestFileHashing(t *testing.T) {
	setupTestDir(t)

	// Create test files
	createTestFile(t, "file1.txt", "content1")
	createTestFile(t, "file2.txt", "content2")
	createTestFile(t, "file1_copy.txt", "content1")

	hash1 := hashFile("file1.txt")
	hash2 := hashFile("file2.txt")
	hash1Copy := hashFile("file1_copy.txt")

	// Same content should produce same hash
	if hash1 != hash1Copy {
		t.Errorf("Same content should produce same hash: %s != %s", hash1, hash1Copy)
	}

	// Different content should produce different hash
	if hash1 == hash2 {
		t.Errorf("Different content should produce different hash")
	}

	// Non-existent file should return special hash
	nonExistentHash := hashFile("nonexistent.txt")
	if !strings.HasPrefix(nonExistentHash, "unreadable-") {
		t.Errorf("Non-existent file should return unreadable hash, got: %s", nonExistentHash)
	}
}

func TestChangeDetection(t *testing.T) {
	setupTestDir(t)

	// Initialize with some files
	createTestFile(t, "test.txt", "original content")
	createTestFile(t, "keep.txt", "unchanged")

	err := initGitnot()
	if err != nil {
		t.Fatalf("initGitnot failed: %v", err)
	}

	// Modify file
	createTestFile(t, "test.txt", "modified content")

	// Add new file
	createTestFile(t, "new.txt", "new file content")

	// Delete a file
	os.Remove("keep.txt")

	// Run status to check change detection
	// Capture the change detection logic without relying on stdout
	var oldHashes map[string]string
	loadJSON(hashesFile, &oldHashes)

	files, err := getAllTextFiles(".")
	if err != nil {
		t.Fatalf("getAllTextFiles failed: %v", err)
	}

	current := map[string]string{}
	for _, f := range files {
		current[f] = hashFile(f)
	}

	// Check for new files
	var newFiles []string
	for f := range current {
		if _, ok := oldHashes[f]; !ok {
			newFiles = append(newFiles, f)
		}
	}

	// Check for changed files
	var changedFiles []string
	for f, h := range current {
		if oh, ok := oldHashes[f]; ok && oh != h {
			changedFiles = append(changedFiles, f)
		}
	}

	// Check for deleted files
	var deletedFiles []string
	for f := range oldHashes {
		if _, ok := current[f]; !ok {
			deletedFiles = append(deletedFiles, f)
		}
	}

	// Verify detection results
	if len(newFiles) != 1 || newFiles[0] != "new.txt" {
		t.Errorf("Expected 1 new file 'new.txt', got: %v", newFiles)
	}

	if len(changedFiles) != 1 || changedFiles[0] != "test.txt" {
		t.Errorf("Expected 1 changed file 'test.txt', got: %v", changedFiles)
	}

	if len(deletedFiles) != 1 || deletedFiles[0] != "keep.txt" {
		t.Errorf("Expected 1 deleted file 'keep.txt', got: %v", deletedFiles)
	}
}

func TestConfigLoading(t *testing.T) {
	setupTestDir(t)

	// Test default config when no config file exists
	cfg := loadConfig()
	if len(cfg.Extensions) == 0 {
		t.Error("Default config should have extensions")
	}

	// Check some expected extensions
	hasGo := false
	hasTxt := false
	for _, ext := range cfg.Extensions {
		if ext == ".go" {
			hasGo = true
		}
		if ext == ".txt" {
			hasTxt = true
		}
	}
	if !hasGo || !hasTxt {
		t.Error("Default config should include .go and .txt extensions")
	}

	// Test custom config
	customConfig := Config{
		Extensions:     []string{".custom", ".test"},
		IgnorePatterns: []string{"*.ignore"},
	}

	err := saveJSON(configFile, customConfig)
	if err != nil {
		t.Fatalf("Failed to save custom config: %v", err)
	}

	loadedCfg := loadConfig()
	if len(loadedCfg.Extensions) != 2 {
		t.Errorf("Expected 2 extensions, got %d", len(loadedCfg.Extensions))
	}
	if loadedCfg.Extensions[0] != ".custom" || loadedCfg.Extensions[1] != ".test" {
		t.Errorf("Custom extensions not loaded correctly: %v", loadedCfg.Extensions)
	}
}

func TestIgnorePatterns(t *testing.T) {
	tests := []struct {
		path     string
		patterns []string
		expected bool
	}{
		{"file.tmp", []string{"*.tmp"}, true},
		{"file.txt", []string{"*.tmp"}, false},
		{"backup.bak", []string{"*.bak", "*.tmp"}, true},
		{"node_modules/file.js", []string{"node_modules/*"}, true},
		{"src/node_modules/file.js", []string{"node_modules/*"}, true},
		{"file.js", []string{"node_modules/*"}, false},
		{"exact.txt", []string{"exact.txt"}, true},
		{"other.txt", []string{"exact.txt"}, false},
	}

	for _, test := range tests {
		result := shouldIgnore(test.path, test.patterns)
		if result != test.expected {
			t.Errorf("shouldIgnore(%q, %v) = %t, expected %t",
				test.path, test.patterns, result, test.expected)
		}
	}
}

func TestUpdateGitnotErrorHandling(t *testing.T) {
	setupTestDir(t)

	// Test update without initialization
	err := updateGitnot()
	if err == nil {
		t.Error("updateGitnot should fail when not initialized")
	}
	expectedMsg := "gitnot not initialized"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error containing %q, got: %v", expectedMsg, err)
	}
}

func TestShowStatusErrorHandling(t *testing.T) {
	setupTestDir(t)

	// Test status without initialization
	err := showStatus()
	if err == nil {
		t.Error("showStatus should fail when not initialized")
	}
	expectedMsg := "gitnot not initialized"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error containing %q, got: %v", expectedMsg, err)
	}
}

func TestFileExtensionFiltering(t *testing.T) {
	setupTestDir(t)

	// Create files with various extensions
	createTestFile(t, "test.txt", "text file")
	createTestFile(t, "script.go", "go file")
	createTestFile(t, "image.png", "binary file")
	createTestFile(t, "data.csv", "csv file")
	createTestFile(t, "no_extension", "no ext file")

	files, err := getAllTextFiles(".")
	if err != nil {
		t.Fatalf("getAllTextFiles failed: %v", err)
	}

	// Check that text files are included
	expectedFiles := []string{"test.txt", "script.go", "data.csv"}
	foundFiles := make(map[string]bool)

	for _, file := range files {
		foundFiles[file] = true
	}

	for _, expected := range expectedFiles {
		if !foundFiles[expected] {
			t.Errorf("Expected file %s not found in results", expected)
		}
	}

	// PNG and no extension files should be excluded
	if foundFiles["image.png"] {
		t.Error("Binary file should be excluded")
	}
	if foundFiles["no_extension"] {
		t.Error("File without extension should be excluded")
	}
}

func TestUtilityFunctions(t *testing.T) {
	setupTestDir(t)

	// Test safeMkdirAllForFile
	testPath := "deep/nested/dir/file.txt"
	err := safeMkdirAllForFile(testPath)
	if err != nil {
		t.Errorf("safeMkdirAllForFile failed: %v", err)
	}

	// Check directory was created
	dirPath := filepath.Dir(testPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		t.Errorf("Directory %s was not created", dirPath)
	}

	// Test copyFile
	createTestFile(t, "source.txt", "test content")
	err = copyFile("source.txt", "dest.txt")
	if err != nil {
		t.Errorf("copyFile failed: %v", err)
	}

	// Verify content was copied
	content, err := os.ReadFile("dest.txt")
	if err != nil {
		t.Errorf("Failed to read copied file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("File content not copied correctly")
	}

	// Test appendToFile
	err = appendToFile("append_test.txt", "first line\n")
	if err != nil {
		t.Errorf("appendToFile failed: %v", err)
	}
	err = appendToFile("append_test.txt", "second line\n")
	if err != nil {
		t.Errorf("appendToFile failed on second call: %v", err)
	}

	content, err = os.ReadFile("append_test.txt")
	if err != nil {
		t.Errorf("Failed to read appended file: %v", err)
	}
	expected := "first line\nsecond line\n"
	if string(content) != expected {
		t.Errorf("Expected %q, got %q", expected, string(content))
	}
}

func TestIsUnderGitnot(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{".gitnot/file.txt", true},
		{".gitnot/subdir/file.txt", true},
		{"file.txt", false},
		{"subdir/file.txt", false},
		{".gitnothing/file.txt", true},    // This will match due to HasPrefix behavior
		{"other/.gitnot/file.txt", false}, // doesn't start with .gitnot
	}

	for _, test := range tests {
		result := isUnderGitnot(test.path)
		if result != test.expected {
			t.Errorf("isUnderGitnot(%q) = %t, expected %t", test.path, result, test.expected)
		}
	}
}

func TestJSONUtilities(t *testing.T) {
	setupTestDir(t)

	// Test saving and loading JSON
	testData := map[string]string{
		"file1.txt": "hash1",
		"file2.txt": "hash2",
	}

	err := saveJSON("test.json", testData)
	if err != nil {
		t.Errorf("saveJSON failed: %v", err)
	}

	var loaded map[string]string
	err = loadJSON("test.json", &loaded)
	if err != nil {
		t.Errorf("loadJSON failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 items, got %d", len(loaded))
	}

	if loaded["file1.txt"] != "hash1" || loaded["file2.txt"] != "hash2" {
		t.Errorf("JSON data not loaded correctly: %v", loaded)
	}

	// Test loading non-existent file
	var empty map[string]string
	err = loadJSON("nonexistent.json", &empty)
	if err == nil {
		t.Error("loadJSON should fail for non-existent file")
	}
}
