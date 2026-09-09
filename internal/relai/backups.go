package relai

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Retention struct {
	Days  int   `json:"days,omitempty"`
	Count int   `json:"count,omitempty"`
	Bytes int64 `json:"bytes,omitempty"`
}

func (r Retention) validate() error {
	if r.Days < 0 || r.Days > 36500 || r.Count < 0 || r.Bytes < 0 {
		return errors.New("retention limits must be nonnegative; days must not exceed 36500")
	}
	return nil
}
func (r Retention) defaults() Retention {
	if r.Days == 0 {
		r.Days = 30
	}
	if r.Count == 0 {
		r.Count = 100
	}
	if r.Bytes == 0 {
		r.Bytes = 512 << 20
	}
	return r
}

type Backup struct {
	ID        string `json:"id"`
	Phase     string `json:"phase"`
	Bytes     int64  `json:"bytes"`
	Protected bool   `json:"protected"`
	Reason    string `json:"reason,omitempty"`
	Remove    bool   `json:"remove"`
}
type BackupReport struct {
	Items          []Backup `json:"items"`
	Bytes          int64    `json:"bytes"`
	RemainingBytes int64    `json:"remaining_bytes"`
	Warnings       []string `json:"warnings"`
}

// Status refreshes only inspect metadata, never deserialize backup payloads.
func (p Paths) backupUsage() (int64, int, error) {
	root := filepath.Join(p.State, "transactions")
	if e := noSymlink(root); e != nil {
		return 0, 0, e
	}
	var bytes int64
	count := 0
	e := filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		bytes += info.Size()
		if entry.Name() == "journal.json" {
			count++
		}
		return nil
	})
	return bytes, count, e
}

// Planning and deletion share the operation lock. Only a validated journal in
// an otherwise empty directory can be removed; unfamiliar contents stay local.
func (p Paths) backupPlan(r Retention, now time.Time) (BackupReport, error) {
	out := BackupReport{Items: []Backup{}, Warnings: []string{}}
	r = r.defaults()
	ids, e := p.journals()
	if e != nil {
		return out, e
	}
	latest := ""
	for _, id := range ids {
		b := Backup{ID: id, Phase: "unreadable", Protected: true}
		path := filepath.Join(p.State, "transactions", id, "journal.json")
		st, err := os.Lstat(path)
		if err == nil {
			b.Bytes = st.Size()
		}
		j, err := p.readJournal(id)
		if err != nil {
			b.Reason = "Unreadable journal"
		} else {
			b.Phase = j.Phase
			b.Protected = j.Phase != "complete" && j.Phase != "recovered"
			if b.Protected {
				b.Reason = "Recovery or unknown phase"
			}
			if j.Phase == "complete" {
				latest = id
			}
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			b.Protected = true
			b.Reason = "Unrecognized directory contents"
		}
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			b.Protected = true
			b.Reason = "Unknown creation time"
		}
		out.Bytes += b.Bytes
		out.Items = append(out.Items, b)
	}
	out.RemainingBytes = out.Bytes
	remaining := len(out.Items)
	cutoff := now.AddDate(0, 0, -r.Days).UnixNano()
	for i := range out.Items {
		b := &out.Items[i]
		if b.ID == latest {
			b.Protected = true
			b.Reason = "Latest rollback generation"
		}
		created, _ := strconv.ParseInt(b.ID, 10, 64)
		if !b.Protected && created < cutoff {
			b.Remove = true
			remaining--
			out.RemainingBytes -= b.Bytes
		}
	}
	for i := range out.Items {
		b := &out.Items[i]
		if !b.Protected && !b.Remove && (remaining > r.Count || out.RemainingBytes > r.Bytes) {
			b.Remove = true
			remaining--
			out.RemainingBytes -= b.Bytes
		}
		if b.Protected && b.ID != latest {
			out.Warnings = append(out.Warnings, "Backup "+b.ID+": "+b.Reason)
		}
	}
	if remaining > r.Count || out.RemainingBytes > r.Bytes {
		out.Warnings = append(out.Warnings, "Protected recovery data exceeds retention limits")
	}
	return out, nil
}
func (p Paths) Backups(prune, dry bool) (BackupReport, error) {
	u, e := lock(filepath.Join(p.State, "operation.lock"))
	if e != nil {
		return BackupReport{}, e
	}
	defer u()
	c, e := p.LoadConfig()
	if e != nil {
		return BackupReport{}, e
	}
	return p.pruneBackups(c.Retention, prune && !dry)
}
func (p Paths) pruneBackups(r Retention, apply bool) (BackupReport, error) {
	out, e := p.backupPlan(r, time.Now())
	if e != nil || !apply {
		return out, e
	}
	for _, b := range out.Items {
		if !b.Remove {
			continue
		}
		dir := filepath.Join(p.State, "transactions", b.ID)
		current, e := p.readJournal(b.ID)
		if e != nil || current.Phase != b.Phase {
			return out, fmt.Errorf("backup %s changed during cleanup; retry", b.ID)
		}
		entries, e := os.ReadDir(dir)
		if e != nil || len(entries) != 1 {
			return out, fmt.Errorf("backup directory %s changed during cleanup; retry", b.ID)
		}
		if e = atomicFile(filepath.Join(dir, "journal.json"), FileImage{}); e != nil {
			return out, fmt.Errorf("pruning backup %s: %w", b.ID, e)
		}
		// A crash after journal removal leaves an empty directory, ignored by recovery.
		if e = os.Remove(dir); e != nil {
			return out, e
		}
	}
	if len(out.Items) > 0 {
		if e = syncDir(filepath.Join(p.State, "transactions")); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (p Paths) autoPrune(r Retention) ([]string, error) {
	path := filepath.Join(p.State, "last-prune.json")
	var last time.Time
	if e := readJSON(path, &last); e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	if time.Since(last) < 24*time.Hour {
		return nil, nil
	}
	out, e := p.pruneBackups(r, true)
	if e != nil {
		return nil, e
	}
	return out.Warnings, atomicJSON(path, time.Now().UTC())
}
