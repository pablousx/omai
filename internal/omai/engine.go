package omai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrConflict = errors.New("unresolved conflicts; use omai conflicts")
var ErrOffline = errors.New("remote unavailable; local changes are safe and will retry")

type State struct {
	Canonical  Values             `json:"canonical"`
	Source     Tree               `json:"source"`
	Expected   Values             `json:"expected"`
	Bindings   map[string]Binding `json:"bindings"`
	Generation int                `json:"generation"`
}
type Conflict struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
	Choices Values `json:"choices"`
}
type ProviderStatus struct {
	Name         string   `json:"name"`
	Detected     bool     `json:"detected"`
	Managed      int      `json:"managed"`
	Notes        []string `json:"notes"`
	Instructions int      `json:"instructions"`
	Settings     int      `json:"settings"`
	Skills       int      `json:"skills"`
	MCP          int      `json:"mcp"`
}
type MachineStatus struct {
	Name   string `json:"name"`
	Local  bool   `json:"local"`
	Detail string `json:"detail"`
}
type Status struct {
	BackupBytes      int64             `json:"backup_bytes"`
	BackupCount      int               `json:"backup_count"`
	Version          string            `json:"version"`
	Health           string            `json:"health"`
	Configured       bool              `json:"configured"`
	RemoteConfigured bool              `json:"remote_configured"`
	Daemon           bool              `json:"daemon"`
	Paused           bool              `json:"paused"`
	Pending          bool              `json:"pending"`
	LastSync         string            `json:"last_sync,omitempty"`
	LastApply        string            `json:"last_apply,omitempty"`
	Error            string            `json:"error,omitempty"`
	Generation       int               `json:"generation"`
	Providers        []ProviderStatus  `json:"providers"`
	Machines         []MachineStatus   `json:"machines"`
	Conflicts        []ConflictSummary `json:"conflicts"`
	Warnings         []string          `json:"warnings"`
}
type ConflictSummary struct {
	Key     string   `json:"key"`
	Kind    string   `json:"kind"`
	Reason  string   `json:"reason"`
	Choices []string `json:"choices"`
}
type SyncOptions struct {
	Context    context.Context
	LocalOnly  bool
	Choices    map[string]string
	GitChoices map[string]string
}

func emptyState() State {
	return State{Canonical: Values{}, Source: Tree{}, Expected: Values{}, Bindings: map[string]Binding{}}
}
func (p Paths) loadState() (State, error) {
	s := emptyState()
	e := readJSON(filepath.Join(p.State, "baseline.json"), &s)
	if os.IsNotExist(e) {
		e = nil
	}
	if s.Canonical == nil {
		s.Canonical = Values{}
	}
	if s.Expected == nil {
		s.Expected = Values{}
	}
	if s.Bindings == nil {
		s.Bindings = map[string]Binding{}
	}
	for k, v := range s.Expected {
		if string(v) == "null" {
			s.Expected[k] = nil
		}
	}
	return s, e
}
func (p Paths) conflicts() ([]Conflict, error) {
	var cs []Conflict
	e := readJSON(filepath.Join(p.State, "conflicts.json"), &cs)
	if os.IsNotExist(e) {
		return []Conflict{}, nil
	}
	return cs, e
}
func (p Paths) setConflicts(kind string, next []Conflict) error {
	previous, e := p.conflicts()
	if e != nil {
		return e
	}
	out := []Conflict{}
	for _, c := range previous {
		if c.Kind != kind {
			out = append(out, c)
		}
	}
	out = append(out, next...)
	return atomicJSON(filepath.Join(p.State, "conflicts.json"), out)
}
func summaries(cs []Conflict) []ConflictSummary {
	out := []ConflictSummary{}
	for _, c := range cs {
		out = append(out, ConflictSummary{c.Key, c.Kind, c.Reason, sortedKeys(c.Choices)})
	}
	return out
}
func (p Paths) reconcile(c Config, choices map[string]string) ([]string, error) {
	t, e := loadTree(p.Source)
	if e != nil {
		return nil, e
	}
	m, v, e := parseSource(t)
	if e != nil {
		return nil, e
	}
	s, e := p.loadState()
	if e != nil {
		return nil, e
	}
	a, e := buildAdapters(p, c, m, v, s.Bindings)
	if e != nil {
		return nil, e
	}
	proposals := map[string]Values{}
	allNative := map[string]Values{}
	conflicted := map[string]bool{}
	for _, id := range sortedKeys(a.Bindings) {
		b := a.Bindings[id]
		n, e := a.read(b)
		if e != nil {
			if _, owned := s.Bindings[id]; !owned && v[b.Key] == nil {
				a.Warnings = append(a.Warnings, b.Provider+": unimportable "+b.Key+" left local: "+e.Error())
				delete(a.Bindings, id)
				continue
			}
			return a.Warnings, fmt.Errorf("%s %s: %w", b.Provider, b.Key, e)
		}
		if allNative[b.Key] == nil {
			allNative[b.Key] = Values{}
		}
		allNative[b.Key][b.Provider] = n
		prior, owned := s.Expected[id]
		if !owned {
			if n == nil {
				continue
			}
			if v[b.Key] != nil && !same(v[b.Key], n) {
				conflicted[b.Key] = true
			}
			if b.Provider == "personal" {
				for _, f := range m.Personal {
					if f.Source == b.Key && f.Deleted {
						conflicted[b.Key] = true
					}
				}
			}
			if same(v[b.Key], n) {
				continue
			}
		}
		if owned && same(n, prior) {
			continue
		}
		if proposals[b.Key] == nil {
			proposals[b.Key] = Values{}
		}
		proposals[b.Key][b.Provider] = n
	}
	next := cloneValues(v)
	rs, e := p.resolutions()
	if e != nil {
		return a.Warnings, e
	}
	cs := []Conflict{}
	for _, key := range sortedKeys(proposals) {
		ps := proposals[key]
		var picked json.RawMessage
		first := true
		for _, who := range sortedKeys(ps) {
			n := ps[who]
			if first {
				picked = n
				first = false
			} else if !same(n, picked) {
				conflicted[key] = true
			}
		}
		if !same(v[key], s.Canonical[key]) && !same(v[key], picked) {
			conflicted[key] = true
		}
		if conflicted[key] {
			variants := cloneValues(allNative[key])
			variants["canonical"] = v[key]
			cs = append(cs, Conflict{key, "provider", "concurrent or pre-existing values differ", variants})
			if take, ok := validChoices(rs, cs)[key]; ok {
				value, ok := variants[take]
				if !ok {
					return a.Warnings, fmt.Errorf("invalid resolution for %s", key)
				}
				if value == nil {
					delete(next, key)
				} else {
					next[key] = value
				}
				cs = cs[:len(cs)-1]
			}
			continue
		}
		if picked == nil {
			delete(next, key)
		} else {
			next[key] = picked
		}
	}
	if len(cs) > 0 {
		if e := p.setConflicts("provider", cs); e != nil {
			return a.Warnings, e
		}
		return a.Warnings, ErrConflict
	}
	out, e := sourceWithValues(t, m, next)
	if e != nil {
		return a.Warnings, e
	}
	// Newly imported keys get all provider bindings in the same transaction.
	mm, _, e := parseSource(out)
	if e != nil {
		return a.Warnings, e
	}
	for _, key := range sortedKeys(next) {
		for _, provider := range append(detect(p, c), "personal") {
			if b, ok := makeBinding(p, provider, key, mm); ok {
				a.Bindings[b.ID()] = b
			}
		}
	}
	// Keep immutable preimages. Document maps are changed only after every read above.
	pre := map[string]FileImage{}
	dirty := map[string]bool{}
	newState := emptyState()
	newState.Canonical = next
	newState.Source = out
	newState.Generation = s.Generation
	for _, id := range sortedKeys(a.Bindings) {
		b := a.Bindings[id]
		d, e := a.doc(b)
		if e != nil {
			return a.Warnings, e
		}
		if _, ok := pre[b.Path]; !ok {
			pre[b.Path] = d.Image
		}
		n, e := a.read(b)
		if e != nil {
			return a.Warnings, e
		}
		want := next[b.Key]
		newState.Bindings[id] = b
		newState.Expected[id] = want
		if same(n, want) {
			continue
		}
		if e := a.write(b, want); e != nil {
			return a.Warnings, e
		}
		dirty[b.Path] = true
	}
	changes, e := treeChanges(p.Source, t, out)
	if e != nil {
		return a.Warnings, e
	}
	for _, path := range sortedKeys(dirty) {
		d := a.Docs[path]
		after := d.Image
		if d.Format != "file" && after.Exists {
			after.Data, e = d.encode()
			if e != nil {
				return a.Warnings, e
			}
			if after.Mode == 0 {
				after.Mode = 0600
			}
		}
		before := pre[path]
		if !equalImage(before, after) {
			changes = append(changes, Change{path, before, after})
		}
	}
	if len(changes) > 0 || !equalTree(s.Source, out) {
		newState.Generation++
	}
	before, e := fileImage(filepath.Join(p.State, "baseline.json"))
	if e != nil {
		return a.Warnings, e
	}
	after := FileImage{true, jsonBytes(newState), 0600}
	if !equalImage(before, after) {
		changes = append(changes, Change{filepath.Join(p.State, "baseline.json"), before, after})
	}
	if len(changes) > 0 {
		if _, e = p.transaction(changes); e != nil {
			return a.Warnings, e
		}
		if e := atomicJSON(filepath.Join(p.State, "last-apply.json"), time.Now().UTC().Format(time.RFC3339)); e != nil {
			return a.Warnings, fmt.Errorf("cannot persist last application time: %w", e)
		}
	}
	if e = p.setConflicts("provider", nil); e != nil {
		return a.Warnings, e
	}
	if e = p.clearResolutions("provider"); e != nil {
		return a.Warnings, e
	}
	return a.Warnings, nil
}
func (p Paths) Sync(opt SyncOptions) (result Status, err error) {
	unlock, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return p.Status(), e
	}
	defer unlock()
	c, e := p.LoadConfig()
	if e != nil {
		return p.Status(), errors.New("setup required: run omai setup")
	}
	warnings := []string{}
	previous := p.Status()
	defer func() {
		if err == nil && !p.IsPaused() {
			more, pruneErr := p.autoPrune(c.Retention)
			warnings = append(warnings, more...)
			if pruneErr != nil {
				warnings = append(warnings, "Backup cleanup: "+pruneErr.Error())
			}
		}
		result = p.Status()
		result.Warnings = uniqueStrings(warnings)
		if err != nil {
			result.Error = err.Error()
			result.Health = "error"
			if errors.Is(err, ErrConflict) {
				result.Health = "conflict"
			}
			if errors.Is(err, ErrOffline) {
				result.Health = "offline"
			}
		} else {
			result.Error = ""
			switch {
			case result.Paused:
				result.Health = "paused"
			case len(result.Conflicts) > 0:
				result.Health = "conflict"
			case opt.LocalOnly && c.Remote != "" && previous.Health == "offline":
				result.Health = "offline"
				result.Error = previous.Error
			case result.Pending:
				result.Health = "pending"
			case c.Remote == "":
				result.Health = "local"
			default:
				result.Health = "healthy"
			}
		}
		if e := atomicJSON(filepath.Join(p.State, "status.json"), result); e != nil {
			err = errors.Join(err, fmt.Errorf("cannot persist sync status: %w", e))
			result.Health, result.Error = "error", err.Error()
		}
	}()
	if err = p.checkProviderRoots(); err != nil {
		return
	}
	if err = p.recover(); err != nil {
		return
	}
	if p.IsPaused() {
		return
	}
	if err = p.rememberChoices(opt); err != nil {
		return
	}
	warnings, err = p.reconcile(c, opt.Choices)
	if err != nil {
		return
	}
	if !opt.LocalOnly {
		err = p.gitSync(c, opt.GitChoices, opt.Context)
		if err != nil {
			return
		}
		if err = p.setConflicts("git", nil); err != nil {
			return
		}
		if err = p.clearResolutions("git"); err != nil {
			return
		}
		more, e := p.reconcile(c, nil)
		warnings = append(warnings, more...)
		if e != nil {
			err = e
			return
		}
		if c.Remote != "" {
			err = atomicJSON(filepath.Join(p.State, "last-sync.json"), time.Now().UTC().Format(time.RFC3339))
			if err == nil {
				current, e := p.loadState()
				if e != nil {
					err = e
				} else {
					g, e := p.gitStore(c)
					if e != nil {
						err = e
						return
					}
					committed, e := g.snapshot(g.ref("refs/heads/" + c.Branch))
					if e != nil {
						err = e
						return
					}
					if equalTree(current.Source, committed) {
						err = atomicJSON(filepath.Join(p.State, "synced-generation.json"), current.Generation)
					}
				}
			}
		}
	}
	return
}
func (p Paths) Status() Status {
	s := Status{Version: Version, Health: "setup", Providers: []ProviderStatus{}, Machines: []MachineStatus{}, Conflicts: []ConflictSummary{}, Warnings: []string{}}
	statusErr := readJSON(filepath.Join(p.State, "status.json"), &s)
	s.Version = Version
	var readErrors []string
	check := func(name string, err error) {
		if err != nil && !os.IsNotExist(err) {
			readErrors = append(readErrors, name+": "+err.Error())
		}
	}
	check("Unreadable status", statusErr)
	c, e := p.LoadConfig()
	s.Configured = e == nil
	s.RemoteConfigured = c.Remote != ""
	s.Paused = p.IsPaused()
	s.Daemon = daemonAlive(p)
	if e != nil {
		// A missing settings file is a fresh onboarding state, even if older
		// persisted status still contains conflicts, labels or error messages.
		s = Status{Version: Version, Health: "setup", Daemon: s.Daemon, Providers: []ProviderStatus{}, Machines: []MachineStatus{}, Conflicts: []ConflictSummary{}, Warnings: []string{}}
		if !os.IsNotExist(e) {
			s.Health, s.Error = "error", "Invalid local configuration: "+e.Error()
		}
		return s
	}
	check("Unreadable last sync", readJSON(filepath.Join(p.State, "last-sync.json"), &s.LastSync))
	check("Unreadable last apply", readJSON(filepath.Join(p.State, "last-apply.json"), &s.LastApply))
	bs, baselineErr := p.loadState()
	check("Unreadable baseline", baselineErr)
	s.Generation = bs.Generation
	var syncedGeneration int
	check("Unreadable synced generation", readJSON(filepath.Join(p.State, "synced-generation.json"), &syncedGeneration))
	s.Pending = c.Remote != "" && (s.Generation != syncedGeneration || s.LastSync == "")
	s.Providers = []ProviderStatus{}
	det := detect(p, c)
	for _, name := range Providers {
		present := false
		for _, n := range det {
			if n == name {
				present = true
			}
		}
		count := 0
		coverage := ProviderStatus{Name: name, Detected: present}
		skills := map[string]bool{}
		for _, b := range bs.Bindings {
			if b.Provider == name && bs.Expected[b.ID()] != nil {
				count++
				key := strings.TrimPrefix(b.Key, "providers/"+name+"/")
				switch {
				case key == "instructions.md" || key == "AGENTS.override.md":
					coverage.Instructions++
				case strings.HasPrefix(key, "settings/"):
					coverage.Settings++
				case strings.HasPrefix(key, "skills/"):
					parts := strings.Split(key, "/")
					if len(parts) >= 3 {
						skills[parts[1]] = true
					}
				case strings.HasPrefix(key, "mcp/"):
					coverage.MCP++
				}
			}
		}
		notes := []string{}
		if name == "codex" {
			notes = append(notes, "Commands are explicit skills: $omai-command-NAME")
		}
		if name == "opencode" {
			notes = append(notes, "Hooks use native JavaScript plugins")
		}
		coverage.Managed, coverage.Notes, coverage.Skills = count, notes, len(skills)
		s.Providers = append(s.Providers, coverage)
	}
	s.Machines = []MachineStatus{{c.Machine, true, "This computer; labels stay local"}}
	for _, n := range c.Peers {
		s.Machines = append(s.Machines, MachineStatus{n, false, "User-listed peer; no device telemetry is synchronized"})
	}
	cs, conflictsErr := p.conflicts()
	check("Unreadable conflicts", conflictsErr)
	s.Conflicts = summaries(cs)
	if len(cs) > 0 {
		s.Health = "conflict"
	}
	if s.Paused {
		s.Health = "paused"
	}
	backupBytes, backupCount, backupErr := p.backupUsage()
	check("Unreadable backups", backupErr)
	s.BackupBytes, s.BackupCount = backupBytes, backupCount
	limits := c.Retention.defaults()
	if backupCount > limits.Count || backupBytes > limits.Bytes {
		s.Warnings = uniqueStrings(append(s.Warnings, "Recovery history exceeds retention limits; run omai backups list"))
	}
	if len(readErrors) > 0 {
		s.Health, s.Error = "error", strings.Join(readErrors, "; ")
	}
	return s
}
func (p Paths) Doctor() (Status, []string) {
	s := p.Status()
	issues := []string{}
	if e := p.checkProviderRoots(); e != nil {
		issues = append(issues, e.Error())
	}
	if !s.Configured {
		if s.Error != "" {
			issues = append(issues, s.Error)
		}
		issues = append(issues, "Setup required: omai setup")
		return s, issues
	}
	if _, e := execLookPath("git"); e != nil {
		issues = append(issues, "Git is missing")
	}
	if s.Error != "" {
		issues = append(issues, s.Error)
	}
	t, e := loadTree(p.Source)
	if e == nil {
		m, v, err := parseSource(t)
		e = err
		if e == nil {
			c, _ := p.LoadConfig()
			baseline, _ := p.loadState()
			a, err := buildAdapters(p, c, m, v, baseline.Bindings)
			e = err
			if e == nil {
				for _, id := range sortedKeys(a.Bindings) {
					if _, err := a.read(a.Bindings[id]); err != nil {
						issues = append(issues, a.Bindings[id].Provider+" "+a.Bindings[id].Key+": "+err.Error())
					}
				}
				issues = append(issues, a.Warnings...)
			}
		}
	}
	if e != nil {
		issues = append(issues, e.Error())
	}
	if !s.Daemon {
		issues = append(issues, "Background daemon is stopped")
	}
	ids, e := p.journals()
	if e != nil {
		issues = append(issues, e.Error())
	}
	for _, id := range ids {
		j, e := p.readJournal(id)
		if e != nil {
			issues = append(issues, "Unreadable recovery journal "+id)
		} else if j.Phase == "pending" {
			issues = append(issues, "Pending recovery journal "+id)
		}
	}
	if len(s.Conflicts) > 0 {
		issues = append(issues, "Unresolved conflicts")
	}
	if s.Paused {
		issues = append(issues, "Paused after rollback")
	}
	for _, key := range []string{"OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT"} {
		if os.Getenv(key) != "" {
			issues = append(issues, key+" adds a local override outside omai's global config")
		}
	}
	for _, w := range s.Warnings {
		issues = append(issues, w)
	}
	return s, uniqueStrings(issues)
}
func (p Paths) Conflicts() ([]ConflictSummary, error) {
	cs, e := p.conflicts()
	return summaries(cs), e
}
func (p Paths) ConflictDetail(key string) (Conflict, error) {
	cs, e := p.conflicts()
	if e != nil {
		return Conflict{}, e
	}
	for _, c := range cs {
		if c.Key == key {
			return c, nil
		}
	}
	return Conflict{}, errors.New("conflict not found")
}
func validateChoices(choices map[string]string) error {
	for k, v := range choices {
		if strings.TrimSpace(k) == "" || v == "" {
			return errors.New("resolution needs a key and choice")
		}
	}
	return nil
}
