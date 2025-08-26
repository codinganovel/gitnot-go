# ⚙️ gitnot
### 🚫 not git > should be self explanatory

gitnot is a snapshot-based changelog tool that lets you create lightweight version checkpoints of your code or notes. It tracks only what's changed, bumps the version after each run, and logs your edits like a personal ledger — without the overhead of full version control.

## 🧠 why
gitnot is built on a simple idea: versioning doesn't need to be complex. It's a simplified version control system for individuals who prefer intuitive snapshots over branching and commit workflows — built for those who think in checkpoints, not merges.

## 📦 Installation

### Build from source
```bash
git clone https://github.com/codinganovel/gitnot
cd gitnot
go build -o gitnot .
```

### Or download binary
Download the latest binary from the releases page and add it to your PATH.

## 🚀 Usage

### `gitnot`
The main command. Run this in any folder that has already been initialized with `gitnot --init`. It checks your files for changes, and if anything has been added, removed, or modified, it:

- Saves a snapshot of the current state
- Bumps the version number
- Logs the changes in a human-readable changelog

Think of this like a personal "commit" — but simpler and without ceremony. If nothing has changed, it does nothing.

### `gitnot --init`
Bootstraps the current folder to start using gitnot. This sets up a `.gitnot/` directory where all version data and history will be stored. Run this once per project — before your first gitnot command.

### `gitnot --show`
Displays the current version of the folder you're in — simple and clean. Run it anytime you want to know which version you're working on.

### `gitnot --status`
Shows pending changes without committing them. Use this to see what files have been added, modified, or deleted since the last version.

### `gitnot --help`
Shows usage information and available commands.

## 📁 What it creates

When you run `gitnot --init`, it creates a hidden `.gitnot/` folder inside your current directory. This folder contains all the versioning and change-tracking data for the project. Here's what's inside:

### 📂 `.gitnot/` Directory Structure

| File/Folder    | Purpose |
|----------------|---------|
| `version.txt`  | Tracks the current version number (e.g., `0.2`) of the folder. |
| `hashes.json`  | Internal tracker that stores the SHA1 hash of every file to detect changes. |
| `config.json`  | Configuration file defining which file extensions to track and ignore patterns. |
| `changelogs/`  | A folder containing per-file markdown logs. Each tracked file gets its own `.log` file with version history and diffs. |
| `snapshot/`    | Stores complete snapshots of all tracked files at the current version (used for diffing). |
| `deleted/`     | A folder where deleted files are moved and preserved, so you can always retrieve removed content if needed. |

This entire `.gitnot/` folder is **self-contained**, lightweight, and designed to be ignored by Git if you want to keep your version history personal.

You can safely add `.gitnot/` to your `.gitignore`.


## 🔧 Configuration

After running `gitnot --init`, you can customize which files are tracked by editing `.gitnot/config.json`:

```json
{
  "extensions": [
    ".txt", ".md", ".csv", ".log", ".py", ".js", ".sh",
    ".html", ".css", ".c", ".java", ".json", ".yaml",
    ".yml", ".ini", ".toml", ".xml", ".rtf", ".go"
  ],
  "ignore_patterns": ["*.tmp", "*.bak", "node_modules/*"]
}
```

- **extensions**: File extensions to track for changes
- **ignore_patterns**: Glob patterns for files/directories to ignore

## 🛠 Contributing

Pull requests welcome. Open an issue or suggest an idea.

## 📄 License

under ☕️, check out [the-coffee-license](https://github.com/codinganovel/The-Coffee-License)

I've included both licenses with the repo, do what you know is right. The licensing works by assuming you're operating under good faith.

> built by **Sam** with ☕️&❤️