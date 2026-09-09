package relai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type installation struct {
	Changes []Change `json:"changes"`
	Active  bool     `json:"active"`
	Enabled bool     `json:"enabled"`
	Service bool     `json:"service"`
}

func serviceQuery(query string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "systemctl", "--user", query, "--quiet", "relai.service").Run() == nil
}
func (p Paths) installTargets() (string, string, string) {
	return filepath.Join(p.Data, "bin/relai"), filepath.Join(filepath.Dir(p.Config), "systemd/user/relai.service"), filepath.Join(p.Home, ".local/bin/relai")
}
func (p Paths) restoreInstallation(j installation) error {
	// Check every preimage before restoring any file, including retries after a crash.
	dest, unit, cli := p.installTargets()
	allowed := map[string]bool{dest: true, unit: true, cli: true, dest + ".previous": true}
	for _, c := range j.Changes {
		if !allowed[c.Path] {
			return errors.New("invalid installation recovery path")
		}
		cur, e := fileImage(c.Path)
		if e != nil {
			return e
		}
		if !equalImage(cur, c.Before) && !equalImage(cur, c.After) {
			return fmt.Errorf("installation recovery blocked by newer edits: %s", c.Path)
		}
	}
	if j.Service {
		if _, e := os.Lstat(unit); e == nil {
			if e := systemctl("stop", "relai.service"); e != nil {
				return e
			}
			if !j.Enabled {
				if e := systemctl("disable", "relai.service"); e != nil {
					return e
				}
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	for i := len(j.Changes) - 1; i >= 0; i-- {
		c := j.Changes[i]
		if e := atomicFile(c.Path, c.Before); e != nil {
			return e
		}
	}
	if j.Service {
		if e := systemctl("daemon-reload"); e != nil {
			return e
		}
		if j.Enabled {
			if e := systemctl("enable", "relai.service"); e != nil {
				return e
			}
		}
		if j.Active {
			if e := systemctl("start", "relai.service"); e != nil {
				return e
			}
		}
	}
	return atomicFile(filepath.Join(p.State, "install-pending.json"), FileImage{})
}

// Install executes from a staged binary. The old executable is never used to
// interpret a new installation record. Running provider sync is stopped before
// acquiring its operation lock, avoiding systemd/daemon lock inversion.
func (p Paths) Install(activate, binaryOnly bool) (err error) {
	if e := p.validatePaths(); e != nil {
		return e
	}
	u, e := lock(filepath.Join(p.State, "install.lock"))
	if e != nil {
		return e
	}
	defer u()
	pending := filepath.Join(p.State, "install-pending.json")
	var prior installation
	if e = readJSON(pending, &prior); e == nil {
		if e = p.restoreInstallation(prior); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	dest, unit, cli := p.installTargets()
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	data, e := readExecutable(exe)
	if e != nil {
		return e
	}
	requested := map[string]FileImage{dest: {true, data, 0700}}
	_, configErr := p.LoadConfig()
	if !binaryOnly && configErr == nil {
		requested[unit] = FileImage{true, []byte(p.ServiceUnit()), 0600}
		requested[cli] = FileImage{true, []byte("#!/bin/sh\n# Managed by Relai\nexec " + shellQuote(dest) + " \"$@\"\n"), 0700}
	} else if !binaryOnly && !os.IsNotExist(configErr) {
		return configErr
	}
	j := installation{Service: requested[unit].Exists}
	if j.Service {
		j.Active = serviceQuery("is-active")
		j.Enabled = serviceQuery("is-enabled")
	}
	for _, path := range sortedKeys(requested) {
		before, e := fileImage(path)
		if e != nil {
			return e
		}
		if before.Exists && path != dest && !strings.HasPrefix(string(before.Data), "# Managed by Relai\n") && !strings.HasPrefix(string(before.Data), "#!/bin/sh\n# Managed by Relai\n") {
			return fmt.Errorf("refusing unrelated installation file: %s", path)
		}
		if path == dest && before.Exists && !equalImage(before, requested[path]) {
			previous, e := fileImage(dest + ".previous")
			if e != nil {
				return e
			}
			j.Changes = append(j.Changes, Change{dest + ".previous", previous, before})
		}
		if !equalImage(before, requested[path]) {
			j.Changes = append(j.Changes, Change{path, before, requested[path]})
		}
	}
	// Persist intent before even stopping the daemon, so a killed updater can
	// restore its original running/enabled state on the next invocation.
	if e = atomicJSON(pending, j); e != nil {
		return e
	}
	var op func()
	defer func() {
		if op != nil {
			op()
		}
		if err != nil {
			err = errors.Join(err, p.restoreInstallation(j))
		}
	}()
	if j.Active {
		if e = systemctl("stop", "relai.service"); e != nil {
			return e
		}
	}
	op, e = lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	for _, c := range j.Changes {
		cur, e := fileImage(c.Path)
		if e != nil {
			return e
		}
		if !equalImage(cur, c.Before) {
			return fmt.Errorf("installation file changed: %s", c.Path)
		}
		if e = atomicFile(c.Path, c.After); e != nil {
			return e
		}
	}
	op()
	op = nil
	if j.Service {
		if e = systemctl("daemon-reload"); e != nil {
			return e
		}
		if activate {
			if e = systemctl("enable", "--now", "relai.service"); e != nil {
				return e
			}
		} else if j.Active {
			if e = systemctl("start", "relai.service"); e != nil {
				return e
			}
		}
		if (activate || j.Active) && !serviceQuery("is-active") {
			return errors.New("updated daemon failed to start; restoring installation")
		}
	}
	// Retain the last actual installation, including its service state.
	if len(j.Changes) > 0 {
		if e = atomicJSON(filepath.Join(p.State, "install-backups", "latest.json"), j); e != nil {
			return e
		}
	}
	return atomicFile(pending, FileImage{})
}

func (p Paths) UninstallService() error {
	u, e := lock(filepath.Join(p.State, "install.lock"))
	if e != nil {
		return e
	}
	defer u()
	if _, e := os.Lstat(filepath.Join(p.State, "install-pending.json")); !os.IsNotExist(e) {
		return errors.New("finish installation recovery with relai install --recover before uninstalling")
	}
	_, unit, cli := p.installTargets()
	images := map[string]FileImage{}
	for _, path := range []string{unit, cli} {
		before, e := fileImage(path)
		if e != nil {
			return e
		}
		if before.Exists && !strings.HasPrefix(string(before.Data), "# Managed by Relai\n") && !strings.HasPrefix(string(before.Data), "#!/bin/sh\n# Managed by Relai\n") {
			return fmt.Errorf("refusing unrelated file: %s", path)
		}
		images[path] = before
	}
	if images[unit].Exists {
		if e = systemctl("disable", "--now", "relai.service"); e != nil {
			return e
		}
	}
	op, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer op()
	for _, path := range []string{unit, cli} {
		cur, e := fileImage(path)
		if e != nil {
			return e
		}
		if !equalImage(cur, images[path]) {
			return fmt.Errorf("file changed: %s", path)
		}
		if cur.Exists {
			if e = atomicFile(path, FileImage{}); e != nil {
				return e
			}
		}
	}
	return systemctl("daemon-reload")
}

// RecoverInstall is also available offline from an already installed 1.x binary.
func (p Paths) RecoverInstall(rollback bool) error {
	u, e := lock(filepath.Join(p.State, "install.lock"))
	if e != nil {
		return e
	}
	defer u()
	pending := filepath.Join(p.State, "install-pending.json")
	var j installation
	e = readJSON(pending, &j)
	if os.IsNotExist(e) && rollback {
		e = readJSON(filepath.Join(p.State, "install-backups", "latest.json"), &j)
		if e != nil {
			return e
		}
		if e = atomicJSON(pending, j); e != nil {
			return e
		}
	} else if os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	return p.restoreInstallation(j)
}
