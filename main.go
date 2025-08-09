package main

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"io/ioutil"

	"github.com/pmezard/go-difflib/difflib"
)

// --- Constants & paths ---
const (
	gitnotDir     = ".gitnot"
	snapshotDir   = ".gitnot/snapshot"
	changelogDir  = ".gitnot/changelogs"
	deletedDir    = ".gitnot/deleted"
	hashesFile    = ".gitnot/hashes.json"
	versionFile   = ".gitnot/version.txt"
	configFile    = ".gitnot/config.json"
)

// --- Config ---

type Config struct {
	Extensions     []string `json:"extensions"`
	IgnorePatterns []string `json:"ignore_patterns"`
}

var defaultConfig = Config{
	Extensions: []string{
		".txt", ".md", ".csv", ".log", ".py", ".js", ".sh",
		".html", ".css", ".c", ".java", ".json", ".yaml",
		".yml", ".ini", ".toml", ".xml", ".rtf", ".go",
	},
	IgnorePatterns: []string{"*.tmp", "*.bak"},
}

// --- Utilities ---

func safeMkdirAllForFile(p string) error {
	d := filepath.Dir(p)
	if d == "." || d == "" {
		return nil
	}
	return os.MkdirAll(d, 0o755)
}

func loadJSON[T any](p string, out *T) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func saveJSON(p string, v any) error {
	if err := safeMkdirAllForFile(p); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func readVersion() (float64, error) {
	b, err := os.ReadFile(versionFile)
	if errors.Is(err, os.ErrNotExist) {
		return 0.0, nil
	}
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0, nil
	}
	return v, nil
}

func writeVersion(v float64) error {
	if err := os.MkdirAll(gitnotDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(versionFile, []byte(fmt.Sprintf("%.1f", v)), 0o644)
}

func bumpVersion() (float64, error) {
	v, err := readVersion()
	if err != nil {
		return 0, err
	}
	v = float64(int((v+0.1)*10+0.5)) / 10.0 // keep one decimal, avoid fp drift
	if err := writeVersion(v); err != nil {
		return 0, err
	}
	return v, nil
}

// --- Config & filters ---

func loadConfig() Config {
	var cfg Config
	if err := loadJSON(configFile, &cfg); err != nil || len(cfg.Extensions) == 0 {
		return defaultConfig
	}
	return cfg
}

func hasAnySuffix(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, strings.ToLower(e)) {
			return true
		}
	}
	return false
}

func shouldIgnore(p string, patterns []string) bool {
	pp := filepath.ToSlash(p)
	base := path.Base(pp)
	for _, pat := range patterns {
		if strings.HasSuffix(pat, "/*") { // directory pattern
			d := strings.TrimSuffix(pat, "/*")
			// match whole path segments (e.g., node_modules)
			segRe := regexp.MustCompile(`/` + regexp.QuoteMeta(d) + `(/|$)`)
			if strings.Contains(pp, "/"+d+"/") || strings.HasPrefix(pp, d+"/") || segRe.MatchString("/"+pp) {
				return true
			}
			continue
		}
		if strings.ContainsAny(pat, "*?") { // glob
			if ok, _ := path.Match(pat, base); ok {
				return true
			}
			if ok, _ := path.Match(pat, pp); ok {
				return true
			}
			continue
		}
		// exact filename
		if base == pat {
			return true
		}
	}
	return false
}

// --- File scanning & hashing ---

func hashFile(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return fmt.Sprintf("unreadable-%s", filepath.Base(p))
	}
	defer f.Close()
	
	h := sha1.New()
	buf := make([]byte, 8192)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func isUnderGitnot(p string) bool {
	return strings.HasPrefix(filepath.ToSlash(p), gitnotDir)
}

func getAllTextFiles(root string) ([]string, error) {
	cfg := loadConfig()
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			if isUnderGitnot(p) {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasAnySuffix(d.Name(), cfg.Extensions) {
			return nil
		}
		if shouldIgnore(p, cfg.IgnorePatterns) {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// --- Diff helpers ---

func unifiedDiff(oldPath, newPath string) (string, error) {
	oldB, _ := os.ReadFile(oldPath) // tolerate missing/encoding issues
	newB, _ := os.ReadFile(newPath)
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(oldB)),
		B:        difflib.SplitLines(string(newB)),
		FromFile: "before",
		ToFile:   "after",
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(ud)
	return text, err
}

func formatDiffAsMarkdown(diffText string) string {
	if diffText == "" {
		return "📄 File changed (no readable diff)\n"
	}
	
	lines := strings.Split(diffText, "\n")
	var added, removed []string
	oldLn, newLn := 0, 0
	i := 0
	
	for i < len(lines) {
		line := lines[i]
		
		// New hunk header --- @@ -a,b +c,d @@
		if strings.HasPrefix(line, "@@") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				if oldPart := strings.Split(parts[1], ","); len(oldPart) > 0 {
					if num, err := strconv.Atoi(strings.TrimPrefix(oldPart[0], "-")); err == nil {
						oldLn = num
					}
				}
				if newPart := strings.Split(parts[2], ","); len(newPart) > 0 {
					if num, err := strconv.Atoi(strings.TrimPrefix(newPart[0], "+")); err == nil {
						newLn = num
					}
				}
			}
			i++
			continue
		}
		
		// Look-ahead for identical -/+ pair (newline / whitespace change)
		if strings.HasPrefix(line, "-") && i+1 < len(lines) &&
			strings.HasPrefix(lines[i+1], "+") &&
			strings.TrimSpace(line[1:]) == strings.TrimSpace(lines[i+1][1:]) {
			// Skip both lines, just advance counters
			oldLn++
			newLn++
			i += 2
			continue
		}
		
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed = append(removed, fmt.Sprintf("L%d: %s", oldLn, strings.TrimSpace(line[1:])))
			oldLn++
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, fmt.Sprintf("L%d: %s", newLn, strings.TrimSpace(line[1:])))
			newLn++
		} else {
			// Context line or \ No newline at end of file
			if !strings.HasPrefix(line, "\\") {
				oldLn++
				newLn++
			}
		}
		i++
	}
	
	var b strings.Builder
	if len(added) > 0 {
		b.WriteString("### ➕ Added\n")
		for _, l := range added {
			b.WriteString(l)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(removed) > 0 {
		b.WriteString("### ➖ Removed\n")
		for _, l := range removed {
			b.WriteString(l)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	
	return b.String()
}

// --- Core ops ---

func initGitnot() error {
	// Create dirs
	for _, d := range []string{snapshotDir, changelogDir, deletedDir} {
		if err := os.MkdirAll(d, 0o755); err != nil { return err }
	}
	// Save default config if missing
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		if err := saveJSON(configFile, defaultConfig); err != nil { return err }
	}

	files, err := getAllTextFiles(".")
	if err != nil { return err }
	hashes := map[string]string{}
	for _, f := range files {
		rel := f
		snap := filepath.Join(snapshotDir, rel)
		if err := safeMkdirAllForFile(snap); err != nil { return err }
		if err := copyFile(f, snap); err != nil { continue }
		hashes[rel] = hashFile(f)

		// create initial changelog entry
		clPath := filepath.Join(changelogDir, rel+".log")
		_ = safeMkdirAllForFile(clPath)
		_ = appendToFile(clPath, fmt.Sprintf("# %s — original v0.0\n", rel))
	}
	if err := saveJSON(hashesFile, hashes); err != nil { return err }
	if err := writeVersion(0.0); err != nil { return err }
	fmt.Printf("✨ Initialized gitnot at version 0.0\n")
	fmt.Printf("📁 Tracking %d files\n", len(hashes))
	return nil
}

func updateGitnot() error {
	if _, err := os.Stat(gitnotDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("gitnot not initialized; run --init")
	}
	var oldHashes map[string]string
	if err := loadJSON(hashesFile, &oldHashes); err != nil { oldHashes = map[string]string{} }
	files, err := getAllTextFiles(".")
	if err != nil { return err }
	current := map[string]string{}
	for _, f := range files {
		current[f] = hashFile(f)
	}
	// detect changes
	var newFiles, changedFiles, deletedFiles []string
	for f := range current { if _, ok := oldHashes[f]; !ok { newFiles = append(newFiles, f) } }
	for f, h := range current { if oh, ok := oldHashes[f]; ok && oh != h { changedFiles = append(changedFiles, f) } }
	for f := range oldHashes { if _, ok := current[f]; !ok { deletedFiles = append(deletedFiles, f) } }
	if len(newFiles)+len(changedFiles)+len(deletedFiles) == 0 {
		fmt.Println("✅ No changes detected")
		return nil
	}
	ver, err := bumpVersion(); if err != nil { return err }
	ts := time.Now().Format("2006-01-02 15:04")

	// handle new and modified files - update changelogs first
	for _, rel := range newFiles {
		clPath := filepath.Join(changelogDir, rel+".log")
		_ = safeMkdirAllForFile(clPath)
		_ = appendToFile(clPath, fmt.Sprintf("\n## v%.1f – %s\n📄 New file added.\n", ver, ts))
	}
	
	for _, rel := range changedFiles {
		oldP := filepath.Join(snapshotDir, rel)
		newP := rel
		clPath := filepath.Join(changelogDir, rel+".log")
		_ = safeMkdirAllForFile(clPath)
		
		// Try to read files and generate diff
		if _, err := os.Stat(oldP); err == nil {
			diffText, _ := unifiedDiff(oldP, newP)
			if diffText != "" {
				_ = appendToFile(clPath, fmt.Sprintf("\n## v%.1f – %s\n%s", ver, ts, formatDiffAsMarkdown(diffText)))
			} else {
				_ = appendToFile(clPath, fmt.Sprintf("\n## v%.1f – %s\n📄 File changed (no readable diff)\n", ver, ts))
			}
		} else {
			_ = appendToFile(clPath, fmt.Sprintf("\n## v%.1f – %s\n📄 File changed (encoding issues, diff skipped)\n", ver, ts))
		}
	}
	// handle deleted files
	for _, rel := range deletedFiles {
		clPath := filepath.Join(changelogDir, rel+".log")
		_ = safeMkdirAllForFile(clPath)
		_ = appendToFile(clPath, fmt.Sprintf("\n## v%.1f – %s\n🔻 File was deleted.\n", ver, ts))
		
		// move snapshot to deleted store
		from := filepath.Join(snapshotDir, rel)
		to := filepath.Join(deletedDir, rel)
		if _, err := os.Stat(from); err == nil {
			_ = safeMkdirAllForFile(to)
			_ = copyFile(from, to) // Use copy instead of move for safety
			_ = os.Remove(from)
		}
	}

	// Atomic snapshot replacement using temporary directory
	if _, err := os.Stat(snapshotDir); err == nil {
		tempDir, err := ioutil.TempDir("", "gitnot_snapshot_")
		if err != nil {
			fmt.Printf("⚠️  Warning: Could not create temp directory: %v\n", err)
		} else {
			// Copy current files to temp location
			allOk := true
			for _, file := range files {
				rel := file
				target := filepath.Join(tempDir, rel)
				if err := safeMkdirAllForFile(target); err != nil {
					allOk = false
					break
				}
				if err := copyFile(file, target); err != nil {
					allOk = false
					break
				}
			}
			
			if allOk {
				// Atomic replacement
				if err := os.RemoveAll(snapshotDir); err != nil {
					fmt.Printf("⚠️  Warning: Could not remove old snapshot: %v\n", err)
				} else if err := os.Rename(tempDir, snapshotDir); err != nil {
					fmt.Printf("⚠️  Warning: Could not move new snapshot: %v\n", err)
				}
			} else {
				fmt.Printf("⚠️  Warning: Could not update snapshot\n")
				_ = os.RemoveAll(tempDir) // cleanup
			}
		}
	} else {
		fmt.Println("⚠️  Snapshot folder missing. Please reinitialize with 'gitnot --init'")
		return nil
	}

	// save hashes
	if err := saveJSON(hashesFile, current); err != nil { return err }
	fmt.Printf("⬆ Version bumped → v%.1f\n", ver)
	fmt.Printf("📝 %d files tracked\n", len(files))
	return nil
}

func showStatus() error {
	if _, err := os.Stat(gitnotDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("❌ gitnot not initialized")
	}
	var oldHashes map[string]string
	_ = loadJSON(hashesFile, &oldHashes)
	files, err := getAllTextFiles(".")
	if err != nil { return err }
	current := map[string]string{}
	for _, f := range files {
		current[f] = hashFile(f)
	}
	var newFiles, changedFiles, deletedFiles []string
	for f := range current { if _, ok := oldHashes[f]; !ok { newFiles = append(newFiles, f) } }
	for f, h := range current { if oh, ok := oldHashes[f]; ok && oh != h { changedFiles = append(changedFiles, f) } }
	for f := range oldHashes { if _, ok := current[f]; !ok { deletedFiles = append(deletedFiles, f) } }
	if len(newFiles)+len(changedFiles)+len(deletedFiles) == 0 {
		fmt.Println("✅ No changes detected")
		return nil
	}
	if len(newFiles) > 0 {
		fmt.Printf("📄 New files (%d): %s\n", len(newFiles), strings.Join(preview(newFiles, 3), ", "))
		if len(newFiles) > 3 { fmt.Printf("    ... and %d more\n", len(newFiles)-3) }
	}
	if len(changedFiles) > 0 {
		fmt.Printf("📝 Modified (%d): %s\n", len(changedFiles), strings.Join(preview(changedFiles, 3), ", "))
		if len(changedFiles) > 3 { fmt.Printf("    ... and %d more\n", len(changedFiles)-3) }
	}
	if len(deletedFiles) > 0 {
		fmt.Printf("🗑️  Deleted (%d): %s\n", len(deletedFiles), strings.Join(preview(deletedFiles, 3), ", "))
		if len(deletedFiles) > 3 { fmt.Printf("    ... and %d more\n", len(deletedFiles)-3) }
	}
	return nil
}

func preview(ss []string, n int) []string {
	if len(ss) <= n { return ss }
	return ss[:n]
}

func showVersion() error {
	v, err := readVersion()
	if err != nil { return err }
	fmt.Printf("📌 Current version: v%.1f\n", v)
	
	// Display actually tracked files from hashes.json
	var hashes map[string]string
	if err := loadJSON(hashesFile, &hashes); err != nil {
		fmt.Printf("⚠️ Could not load tracked files: %v\n", err)
		return nil
	}
	
	if len(hashes) == 0 {
		fmt.Printf("📁 No files are currently being tracked\n")
	} else {
		fmt.Printf("📁 Tracked files (%d):\n", len(hashes))
		// Sort file names for consistent output
		var files []string
		for file := range hashes {
			files = append(files, file)
		}
		sort.Strings(files)
		for _, file := range files {
			fmt.Printf("  • %s\n", file)
		}
	}
	
	return nil
}

func showHelp() {
	fmt.Print(`
🔧 gitnot - Simple version control for personal projects

Usage:
  gitnot          Track changes and bump version
  gitnot --init   Initialize gitnot in current folder  
  gitnot --show   Display current version
  gitnot --status Show pending changes (without committing)
  gitnot --help   Show this help message

Examples:
  gitnot --init   # Start tracking this folder
  gitnot          # Save current state as new version
  gitnot --status # See what's changed since last version

Configuration:
  Edit .gitnot/config.json to customize file extensions and ignore patterns
  
Features:
  • Lightweight snapshots without git complexity
  • Automatic change detection and version bumping
  • Human-readable markdown changelogs
  • Safe handling of file encoding issues
  • Personal project focused (not for large codebases)
`)
}

// --- Small file helpers ---

func copyFile(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil { return err }
	defer srcF.Close()
	if err := safeMkdirAllForFile(dst); err != nil { return err }
	dstF, err := os.Create(dst)
	if err != nil { return err }
	defer dstF.Close()
	_, err = io.Copy(dstF, srcF)
	return err
}

func appendToFile(p, text string) error {
	if err := safeMkdirAllForFile(p); err != nil { return err }
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil { return err }
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

// --- main ---

func main() {
	// allow either flags or positional args like python version
	initFlag := flag.Bool("init", false, "initialize gitnot")
	showFlag := flag.Bool("show", false, "show version")
	statusFlag := flag.Bool("status", false, "status only")
	helpFlag := flag.Bool("help", false, "help")
	flag.Parse()

	switch {
	case *helpFlag:
		showHelp()
		return
	case *initFlag:
		if err := initGitnot(); err != nil { fmt.Println("❌", err); os.Exit(1) }
		return
	case *showFlag:
		if err := showVersion(); err != nil { fmt.Println("❌", err); os.Exit(1) }
		return
	case *statusFlag:
		if err := showStatus(); err != nil { fmt.Println(err); os.Exit(1) }
		return
	default:
		if err := updateGitnot(); err != nil { 
			if os.IsPermission(err) {
				fmt.Println("❌ Permission denied. Check file/folder permissions.")
			} else {
				fmt.Printf("❌ Error: %v\n", err)
				fmt.Println("💡 Try 'gitnot --init' to reset if needed.")
			}
			os.Exit(1) 
		}
	}
}
