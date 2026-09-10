package omai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type FileImage struct {
	Exists bool   `json:"exists"`
	Data   []byte `json:"data,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
}
type Change struct {
	Path   string    `json:"path"`
	Before FileImage `json:"before"`
	After  FileImage `json:"after"`
}
type Journal struct {
	ID      string   `json:"id"`
	Phase   string   `json:"phase"`
	Changes []Change `json:"changes"`
	Temps   []string `json:"temps,omitempty"`
}

func fileImage(path string) (FileImage, error) {
	if e := noSymlink(path); e != nil {
		return FileImage{}, e
	}
	s, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return FileImage{}, nil
	}
	if e != nil {
		return FileImage{}, e
	}
	b, e := readRegularLimit(path, maxInternalFileSize)
	if e != nil {
		return FileImage{}, e
	}
	return FileImage{true, b, uint32(s.Mode().Perm())}, nil
}
func equalImage(a, b FileImage) bool {
	return a.Exists == b.Exists && (!a.Exists || (bytes.Equal(a.Data, b.Data) && a.Mode == b.Mode))
}
func atomicFile(path string, f FileImage) error {
	return atomicFileTemp(path, f, "")
}
func atomicFileTemp(path string, f FileImage, temp string) error {
	if e := noSymlink(path); e != nil {
		return e
	}
	parent, base, e := openParent(path, true)
	if e != nil {
		return e
	}
	defer parent.Close()
	dirFD := int(parent.Fd())
	if !f.Exists {
		e = syscall.Unlinkat(dirFD, base)
		if e != nil && e != syscall.ENOENT {
			return e
		}
		return parent.Sync()
	}
	name := ""
	if temp != "" {
		if filepath.Dir(temp) != filepath.Dir(path) {
			return errors.New("temporary file must share the destination directory")
		}
		name = filepath.Base(temp)
	}
	tmp, name, e := tempAt(parent, name)
	if e != nil {
		return e
	}
	defer syscall.Unlinkat(dirFD, name)
	mode := os.FileMode(f.Mode)
	if mode == 0 {
		mode = 0600
	}
	if e = tmp.Chmod(mode); e == nil {
		_, e = tmp.Write(f.Data)
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	// Re-check the leaf through the pinned parent. Renaming never follows it.
	check, err := syscall.Openat(dirFD, base, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err == nil {
		_ = syscall.Close(check)
	} else if err != syscall.ENOENT {
		return err
	}
	if e = syscall.Renameat(dirFD, name, dirFD, base); e != nil {
		return e
	}
	return parent.Sync()
}

func atomicJSON(path string, v any) error {
	return atomicFile(path, FileImage{true, jsonBytes(v), 0600})
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func lock(path string) (func(), error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("omai requires absolute HOME and XDG paths")
	}
	if e := noSymlink(path); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	parent, name, e := openParent(path, true)
	if e != nil {
		return nil, e
	}
	defer parent.Close()
	fd, e := syscall.Openat(int(parent.Fd()), name, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("omai is busy; another operation holds the lock")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func treeChanges(dir string, old, new Tree) ([]Change, error) {
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range new {
		keys[k] = true
	}
	out := []Change{}
	for _, k := range sortedKeys(keys) {
		if e := safeRel(k); e != nil {
			return nil, e
		}
		path := filepath.Join(dir, k)
		before, e := fileImage(path)
		if e != nil {
			return nil, e
		}
		previous, existed := old[k]
		if before.Exists != existed || (existed && (!bytes.Equal(before.Data, previous.Data) || (before.Mode&0111 != 0) != (previous.Mode&0111 != 0))) {
			return nil, fmt.Errorf("source changed during planning; retry: %s", path)
		}
		after := FileImage{}
		if b, ok := new[k]; ok {
			after = FileImage{true, b.Data, b.Mode}
			if before.Exists {
				after.Mode = before.Mode
				if b.Mode&0100 != 0 {
					after.Mode |= 0100
				} else {
					after.Mode &^= 0111
				}
			}
		}
		if !equalImage(before, after) {
			out = append(out, Change{path, before, after})
		}
	}
	return out, nil
}
func (p Paths) transaction(changes []Change) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	seen := map[string]bool{}
	for _, c := range changes {
		if seen[c.Path] {
			return "", errors.New("duplicate transaction path")
		}
		seen[c.Path] = true
		cur, e := fileImage(c.Path)
		if e != nil {
			return "", e
		}
		if !equalImage(cur, c.Before) {
			return "", fmt.Errorf("file changed during planning: %s", c.Path)
		}
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	j := Journal{ID: id, Phase: "pending", Changes: changes}
	for i, c := range changes {
		j.Temps = append(j.Temps, filepath.Join(filepath.Dir(c.Path), fmt.Sprintf(".omai-%s-%d.tmp", id, i)))
	}
	dir := filepath.Join(p.State, "transactions", id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	if e := atomicJSON(filepath.Join(dir, "journal.json"), j); e != nil {
		return "", e
	}
	if e := syncDir(filepath.Dir(dir)); e != nil {
		return "", e
	}
	for i, c := range changes {
		cur, e := fileImage(c.Path)
		if e == nil && !equalImage(cur, c.Before) {
			e = fmt.Errorf("file changed during apply: %s", c.Path)
		}
		if e == nil {
			e = atomicFileTemp(c.Path, c.After, j.Temps[i])
		}
		if e != nil {
			return id, fmt.Errorf("transaction interrupted; recovery is required: %w", e)
		}
	}
	j.Phase = "complete"
	if e := atomicJSON(filepath.Join(dir, "journal.json"), j); e != nil {
		return id, e
	}
	if e := atomicJSON(filepath.Join(p.State, "latest.json"), id); e != nil {
		return id, e
	}
	return id, nil
}
func (p Paths) journals() ([]string, error) {
	if e := noSymlink(filepath.Join(p.State, "transactions")); e != nil {
		return nil, e
	}
	ds, e := os.ReadDir(filepath.Join(p.State, "transactions"))
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	ids := map[string]bool{}
	for _, d := range ds {
		if d.IsDir() {
			if _, e := os.Lstat(filepath.Join(p.State, "transactions", d.Name(), "journal.json")); os.IsNotExist(e) {
				continue
			}
			ids[d.Name()] = true
		}
	}
	return sortedKeys(ids), nil
}
func (p Paths) readJournal(id string) (Journal, error) {
	var j Journal
	if e := validName(id); e != nil {
		return j, e
	}
	e := readJSON(filepath.Join(p.State, "transactions", id, "journal.json"), &j)
	if e == nil && j.ID != id {
		e = errors.New("journal ID does not match its directory")
	}
	return j, e
}
func (p Paths) recover() error {
	ids, e := p.journals()
	if e != nil {
		return e
	}
	for _, id := range ids {
		j, e := p.readJournal(id)
		if e != nil {
			return e
		}
		if j.Phase != "pending" {
			continue
		}
		for _, c := range j.Changes {
			cur, e := fileImage(c.Path)
			if e != nil {
				return e
			}
			if !equalImage(cur, c.Before) && !equalImage(cur, c.After) {
				return fmt.Errorf("recovery blocked: %s changed externally; backup %s", c.Path, id)
			}
		}
		for i := len(j.Changes) - 1; i >= 0; i-- {
			c := j.Changes[i]
			temp := ""
			if len(j.Temps) == len(j.Changes) {
				temp = j.Temps[i]
				want := filepath.Join(filepath.Dir(c.Path), fmt.Sprintf(".omai-%s-%d.tmp", j.ID, i))
				if temp != want {
					return errors.New("invalid recovery temporary path")
				}
				if e := noSymlink(temp); e != nil {
					return e
				}
				if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
					return e
				}
			}
			if e := atomicFileTemp(c.Path, c.Before, temp); e != nil {
				return e
			}
		}
		j.Phase = "recovered"
		if e := atomicJSON(filepath.Join(p.State, "transactions", id, "journal.json"), j); e != nil {
			return e
		}
	}
	return nil
}
func (p Paths) Rollback(id string) error {
	unlock, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	if e = p.recover(); e != nil {
		return e
	}
	if id == "" {
		ids, e := p.journals()
		if e != nil {
			return e
		}
		for i := len(ids) - 1; i >= 0; i-- {
			j, e := p.readJournal(ids[i])
			if e != nil {
				return e
			}
			if j.Phase == "complete" {
				id = ids[i]
				break
			}
		}
		if id == "" {
			return errors.New("no transaction to roll back")
		}
	}
	j, e := p.readJournal(id)
	if e != nil {
		return e
	}
	if j.Phase != "complete" {
		return errors.New("transaction is not complete")
	}
	// Canonical edits often happened in an editor before this transaction.
	// The previous baseline retains that source revision so rollback can
	// restore the whole setup, including hand-edited canonical files.
	var priorSource, appliedSource Tree
	for _, c := range j.Changes {
		if c.Path == filepath.Join(p.State, "baseline.json") && c.Before.Exists {
			var old, next State
			if e := json.Unmarshal(c.Before.Data, &old); e != nil {
				return e
			}
			if e := json.Unmarshal(c.After.Data, &next); e != nil {
				return e
			}
			priorSource, appliedSource = old.Source, next.Source
		}
	}
	changes := []Change{}
	for i := len(j.Changes) - 1; i >= 0; i-- {
		c := j.Changes[i]
		cur, e := fileImage(c.Path)
		if e != nil {
			return e
		}
		if !equalImage(cur, c.After) {
			return fmt.Errorf("rollback refused: newer edits in %s; backup %s remains available", c.Path, id)
		}
		if len(priorSource) > 0 && strings.HasPrefix(c.Path, p.Source+string(filepath.Separator)) {
			continue
		}
		changes = append(changes, Change{c.Path, cur, c.Before})
	}
	if len(priorSource) > 0 {
		current, e := loadTree(p.Source)
		if e != nil {
			return e
		}
		if !equalTree(current, appliedSource) {
			return errors.New("rollback refused: canonical source has newer edits")
		}
		restore, e := treeChanges(p.Source, current, priorSource)
		if e != nil {
			return e
		}
		changes = append(restore, changes...)
	}
	if e = atomicJSON(filepath.Join(p.State, "paused.json"), map[string]string{"reason": "rollback; inspect files, then omai daemon resume"}); e != nil {
		return e
	}
	_, e = p.transaction(changes)
	return e
}
func (p Paths) IsPaused() bool {
	_, e := os.Stat(filepath.Join(p.State, "paused.json"))
	return e == nil
}
func (p Paths) Resume() error {
	u, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return e
	}
	defer u()
	if e = p.recover(); e != nil {
		return e
	}
	e = os.Remove(filepath.Join(p.State, "paused.json"))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func cloneValues(v Values) Values {
	b, _ := json.Marshal(v)
	out := Values{}
	_ = json.Unmarshal(b, &out)
	for k, v := range out {
		if string(v) == "null" {
			out[k] = nil
		}
	}
	return out
}
