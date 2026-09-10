package omai

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClearSettings stops synchronization and removes only local preferences.
// Source/provider files, reconciliation state, transport history and recovery
// journals deliberately survive so onboarding can safely reuse existing data.
func (p Paths) ClearSettings() error {
	if e := p.validatePaths(); e != nil {
		return e
	}
	u, e := lock(filepath.Join(p.State, "install.lock"))
	if e != nil {
		return e
	}
	defer u()
	if _, e := os.Lstat(filepath.Join(p.State, "install-pending.json")); !os.IsNotExist(e) {
		return errors.New("finish installation recovery before clearing settings")
	}
	_, unit, _ := p.installTargets()
	service, e := fileImage(unit)
	if e != nil {
		return e
	}
	if service.Exists {
		if !strings.HasPrefix(string(service.Data), "# Managed by omai\n") {
			return fmt.Errorf("refusing unrelated service: %s", unit)
		}
		if e = systemctl("disable", "--now", "omai.service"); e != nil {
			return e
		}
	}
	// A manually started daemon must not outlive the reset. Holding its lock
	// also prevents a second daemon starting while settings are removed.
	d, e := lock(filepath.Join(p.State, "daemon.lock"))
	if e != nil {
		return errors.New("stop the foreground omai daemon before clearing settings")
	}
	defer d()
	op, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer op()
	settings := filepath.Join(p.Config, "config.json")
	paused := filepath.Join(p.State, "paused.json")
	// Validate both targets before the first removal, including malformed JSON
	// (which can be reset) and symlinks/nonregular files (which cannot).
	for _, path := range []string{settings, paused} {
		if _, e = fileImage(path); e != nil {
			return e
		}
	}
	if e = atomicFile(paused, FileImage{}); e != nil {
		return e
	}
	return atomicFile(settings, FileImage{})
}
