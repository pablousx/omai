package omai

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type InventoryEntry struct {
	Provider string `json:"provider"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Bytes    int64  `json:"bytes"`
}

func (p Paths) Inventory() []InventoryEntry {
	out := []InventoryEntry{}
	for _, provider := range Providers {
		root := nativeRoot(p, provider)
		cfg, _ := configFile(p, provider)
		paths := []string{cfg, filepath.Join(root, "AGENTS.md"), filepath.Join(root, "skills"), filepath.Join(root, "agents"), filepath.Join(root, "commands"), filepath.Join(root, "rules")}
		if provider == "claude" {
			paths = append(paths, filepath.Join(root, "CLAUDE.md"), filepath.Join(p.Home, ".claude.json"))
		}
		if provider == "codex" {
			paths = append(paths, filepath.Join(p.Home, ".agents/skills"))
		}
		for _, path := range paths {
			s, e := os.Lstat(path)
			if e != nil {
				continue
			}
			kind := "file"
			if s.IsDir() {
				kind = "directory"
			}
			if s.Mode()&os.ModeSymlink != 0 {
				kind = "symlink (will not be followed)"
			}
			rel, _ := filepath.Rel(p.Home, path)
			out = append(out, InventoryEntry{provider, "~/" + rel, kind, s.Size()})
		}
	}
	return out
}

type SetupOptions struct {
	Remote, Branch, Machine, Seed string
	Providers                     []string
	Peers                         []string
	Service                       bool
}

func (p Paths) checkProviderRoots() error {
	if p.Home != os.Getenv("HOME") {
		return nil
	}
	for _, entry := range [][2]string{{"CODEX_HOME", filepath.Join(p.Home, ".codex")}, {"CLAUDE_CONFIG_DIR", filepath.Join(p.Home, ".claude")}} {
		if value := os.Getenv(entry[0]); value != "" && filepath.Clean(value) != entry[1] {
			return fmt.Errorf("%s uses an alternate provider root; omai manages standard global roots only and has not written provider files", entry[0])
		}
	}
	return nil
}
func (p Paths) Setup(o SetupOptions) error {
	if e := p.setupFiles(o); e != nil {
		return e
	}
	if o.Service {
		return p.InstallService()
	}
	return nil
}
func (p Paths) setupFiles(o SetupOptions) error {
	if e := p.validatePaths(); e != nil {
		return e
	}
	if e := p.checkProviderRoots(); e != nil {
		return e
	}
	if e := validRemote(o.Remote); e != nil {
		return e
	}
	if o.Branch == "" {
		o.Branch = "main"
	}
	if e := validBranch(o.Branch); e != nil {
		return e
	}
	if o.Machine == "" {
		o.Machine = "This computer"
	}
	if strings.ContainsAny(o.Machine, "\r\n\x00") {
		return errors.New("invalid local machine label")
	}
	for _, provider := range o.Providers {
		if !isProvider(provider) {
			return fmt.Errorf("unknown provider %s", provider)
		}
	}
	if o.Seed != "" && !isProvider(o.Seed) {
		return errors.New("seed must name a supported provider")
	}
	// Inventory is deliberately metadata-only and precedes source import or target writes.
	_ = p.Inventory()
	for _, dir := range []string{p.Config, p.Source, p.Data, p.State} {
		if e := noSymlink(dir); e != nil {
			return e
		}
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
	}
	unlock, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	if e = p.recover(); e != nil {
		return e
	}
	if _, e := os.Lstat(filepath.Join(p.Config, "config.json")); !os.IsNotExist(e) {
		c, err := p.LoadConfig()
		if err != nil {
			return err
		}
		if o.Remote != "" && c.Remote != o.Remote {
			return errors.New("already configured with a different remote; edit config.json explicitly")
		}
		t, err := loadTree(p.Source)
		if err != nil {
			return err
		}
		_, _, err = parseSource(t)
		return err
	}
	c := Config{Version: 1, Remote: o.Remote, Branch: o.Branch, Providers: o.Providers, Machine: o.Machine, Peers: o.Peers, PollSeconds: 2, SyncSeconds: 30}
	if _, e := os.Lstat(filepath.Join(p.Source, "omai.json")); os.IsNotExist(e) {
		if e = atomicJSON(filepath.Join(p.Source, "omai.json"), defaultManifest()); e != nil {
			return e
		}
	}
	if o.Seed != "" {
		t, e := loadTree(p.Source)
		if e != nil {
			return e
		}
		m, v, e := parseSource(t)
		if e != nil {
			return e
		}
		seed := c
		seed.Providers = []string{o.Seed}
		a, e := buildAdapters(p, seed, m, v, nil)
		if e != nil {
			return e
		}
		for _, id := range sortedKeys(a.Bindings) {
			b := a.Bindings[id]
			n, e := a.read(b)
			if e != nil {
				continue
			}
			if n != nil {
				if old, ok := v[b.Key]; ok && !same(old, n) {
					return fmt.Errorf("seed conflicts with existing canonical %s", b.Key)
				}
				v[b.Key] = n
			}
		}
		out, e := sourceWithValues(t, m, v)
		if e != nil {
			return e
		}
		changes, e := treeChanges(p.Source, t, out)
		if e != nil {
			return e
		}
		if _, e = p.transaction(changes); e != nil {
			return e
		}
	}
	tree, e := loadTree(p.Source)
	if e != nil {
		return e
	}
	if _, _, e = parseSource(tree); e != nil {
		return e
	}
	if e = atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		return e
	}
	return nil
}
func (p Paths) GuidedSetup(in io.Reader, out io.Writer) error {
	if e := p.checkProviderRoots(); e != nil {
		return e
	}
	for _, command := range []string{"git", "systemctl"} {
		if _, e := exec.LookPath(command); e != nil {
			return fmt.Errorf("setup requires %s on PATH", command)
		}
	}
	reader := bufio.NewReader(in)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		s, e := reader.ReadString('\n')
		if e != nil {
			return "", errors.New("setup cancelled; run omai setup to continue")
		}
		return strings.TrimSpace(s), nil
	}
	accept := func(prompt string) error {
		answer, e := ask(prompt)
		if e != nil {
			return e
		}
		if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes" {
			return errors.New("setup cancelled")
		}
		return nil
	}
	if _, e := p.LoadConfig(); e == nil {
		fmt.Fprintln(out, "omai configuration is already saved. You can finish or repair the user service without changing your source.")
		if e = accept("Install the user service and start syncing? [y/N]: "); e != nil {
			return e
		}
		return p.Setup(SetupOptions{Service: true})
	} else if !os.IsNotExist(e) {
		return fmt.Errorf("repair existing config.json before setup: %w", e)
	}
	fmt.Fprintln(out, "omai — Your AI setup, in sync.\n\nRelevant file inventory (metadata only):")
	for _, entry := range p.Inventory() {
		fmt.Fprintf(out, "  %s: %s (%s, %d bytes)\n", entry.Provider, entry.Path, entry.Kind, entry.Bytes)
	}
	fmt.Fprintln(out, "\nOnly non-secret configuration and text resources belong in the source. Credentials stay with each tool. Git uses your existing local authentication. Uploading starts only after you finish setup and start sync.")
	var remote, machine, seed string
	for {
		value, e := ask("Existing Git remote (blank for local only): ")
		if e != nil {
			return e
		}
		if e = validRemote(value); e != nil {
			fmt.Fprintln(out, e)
			continue
		}
		remote = value
		break
	}
	for {
		value, e := ask("Local display label (blank: This computer; never uploaded): ")
		if e != nil {
			return e
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			fmt.Fprintln(out, "Use a single-line label without control characters.")
			continue
		}
		machine = value
		break
	}
	for {
		value, e := ask("Import initial content from codex, claude, opencode, or blank for automatic safe import: ")
		if e != nil {
			return e
		}
		if value != "" && !isProvider(value) {
			fmt.Fprintln(out, "Choose codex, claude, opencode, or leave blank.")
			continue
		}
		seed = value
		break
	}
	fmt.Fprintln(out, "\nSetup will create the canonical source, install a user service, and start automatic sync. Differing existing files will be reported as conflicts. Your remote receives only validated source files.")
	if e := accept("Finish setup and start syncing? [y/N]: "); e != nil {
		return e
	}
	if e := p.Setup(SetupOptions{Remote: remote, Machine: machine, Seed: seed, Service: true}); e != nil {
		return fmt.Errorf("setup incomplete; fix the reported issue and rerun omai setup: %w", e)
	}
	fmt.Fprintln(out, "Setup complete. Edit "+p.Source+"; omai watches it automatically.")
	return nil
}

func systemdQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}
func (p Paths) ServiceUnit() string {
	dest := filepath.Join(p.Data, "bin/omai")
	unit := "# Managed by omai\n[Unit]\nDescription=omai global configuration relay\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + systemdQuote(dest) + " daemon run\nRestart=on-failure\nRestartSec=5\nTimeoutStopSec=30\nUMask=0077\nNoNewPrivileges=true\nEnvironment=PATH=%h/.local/share/mise/shims:%h/.local/bin:/usr/local/bin:/usr/bin\n"
	for _, pair := range [][2]string{{"XDG_CONFIG_HOME", filepath.Dir(p.Config)}, {"XDG_DATA_HOME", filepath.Dir(p.Data)}, {"XDG_STATE_HOME", filepath.Dir(p.State)}} {
		unit += "Environment=" + systemdQuote(pair[0]+"="+pair[1]) + "\n"
	}
	return unit + "\n[Install]\nWantedBy=default.target\n"
}
func (p Paths) InstallService() error {
	if _, e := p.LoadConfig(); e != nil {
		return fmt.Errorf("valid setup required: %w", e)
	}
	return p.Install(true, false)
}

func readExecutable(path string) ([]byte, error) {
	return readRegularLimit(path, 64<<20)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func systemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func (p Paths) Control(action string) error {
	switch action {
	case "uninstall":
		return p.UninstallService()
	case "install":
		return p.InstallService()
	case "start":
		if _, e := os.Stat(filepath.Join(filepath.Dir(p.Config), "systemd/user/omai.service")); os.IsNotExist(e) {
			return p.InstallService()
		}
		return systemctl("start", "omai.service")
	case "stop", "restart":
		return systemctl(action, "omai.service")
	case "resume":
		return p.Resume()
	case "pause":
		u, e := lock(filepath.Join(p.State, "operation.lock"))
		if e != nil {
			return e
		}
		defer u()
		return atomicJSON(filepath.Join(p.State, "paused.json"), map[string]string{"reason": "user paused"})
	default:
		return errors.New("daemon action must be run, install, start, stop, restart, pause, resume or uninstall")
	}
}
func daemonAlive(p Paths) bool {
	f, e := openRegularNoFollow(filepath.Join(p.State, "daemon.lock"))
	if e != nil {
		return false
	}
	defer f.Close()
	e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if e == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false
	}
	return e == syscall.EWOULDBLOCK
}
func (p Paths) Daemon(ctx context.Context) error {
	if _, e := p.LoadConfig(); e != nil {
		return errors.New("setup required")
	}
	unlock, e := lock(filepath.Join(p.State, "daemon.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	sigctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	nextRemote := time.Time{}
	for {
		c, e := p.LoadConfig()
		if e != nil {
			return e
		}
		now := time.Now()
		remote := !now.Before(nextRemote)
		_, e = p.Sync(SyncOptions{LocalOnly: !remote, Context: sigctx})
		if sigctx.Err() != nil {
			return nil
		}
		if remote {
			nextRemote = now.Add(time.Duration(c.SyncSeconds) * time.Second)
		}
		// No payloads or provider file contents are written to daemon logs.
		if e != nil && !errors.Is(e, ErrConflict) && !errors.Is(e, ErrOffline) {
			fmt.Fprintln(os.Stderr, "omai needs attention; run omai doctor")
		}
		timer := time.NewTimer(time.Duration(c.PollSeconds) * time.Second)
		select {
		case <-sigctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func (p Paths) Enroll(name, source, destination string) error {
	if e := validName(name); e != nil {
		return e
	}
	if e := safePersonal(destination); e != nil {
		return e
	}
	if e := safeRel(source); e != nil {
		return e
	}
	if !strings.HasPrefix(source, "personal/") {
		return errors.New("source must be under personal/")
	}
	u, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer u()
	t, e := loadTree(p.Source)
	if e != nil {
		return e
	}
	m, _, e := parseSource(t)
	if e != nil {
		return e
	}
	if _, ok := m.Personal[name]; ok {
		return errors.New("personal enrollment already exists")
	}
	path := filepath.Join(p.Home, strings.TrimPrefix(destination, "~/"))
	b, e := readRegular(path)
	if e != nil {
		return e
	}
	if e = scanContent(source, b); e != nil {
		return e
	}
	if _, ok := t[source]; ok {
		return errors.New("canonical source already exists")
	}
	m.Personal[name] = Personal{Path: destination, Source: source}
	next := Tree{}
	for k, v := range t {
		next[k] = v
	}
	next[source] = Blob{b, 0600}
	next["omai.json"] = Blob{jsonBytes(m), 0600}
	if _, _, e = parseSource(next); e != nil {
		return e
	}
	changes, e := treeChanges(p.Source, t, next)
	if e != nil {
		return e
	}
	_, e = p.transaction(changes)
	return e
}
