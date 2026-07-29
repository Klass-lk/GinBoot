package ginboot

import (
	"os"
	"path/filepath"
)

// DefaultAirConfigTOML returns the default .air.toml configuration content for Ginboot projects.
func DefaultAirConfigTOML() string {
	return `root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ./cmd/main.go"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html", "yml", "yaml", "env"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  post_cmd = []
  pre_cmd = []
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = true

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
`
}

// EnsureAirConfig ensures that a .air.toml file exists in target directory (or current directory if dir is empty).
// If missing, it writes DefaultAirConfigTOML() automatically.
func EnsureAirConfig(dir string) error {
	if dir == "" {
		dir = "."
	}
	targetPath := filepath.Join(dir, ".air.toml")
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return os.WriteFile(targetPath, []byte(DefaultAirConfigTOML()), 0644)
	}
	return nil
}
