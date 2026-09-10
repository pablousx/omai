package omai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Resolution struct {
	Choice      string `json:"choice"`
	Fingerprint string `json:"fingerprint"`
}

func fingerprint(v Values) string {
	var value any
	_ = json.Unmarshal(raw(v), &value)
	sum := sha256.Sum256(raw(value))
	return hex.EncodeToString(sum[:])
}
func (p Paths) resolutions() (map[string]Resolution, error) {
	out := map[string]Resolution{}
	e := readJSON(filepath.Join(p.State, "resolutions.json"), &out)
	if os.IsNotExist(e) {
		e = nil
	}
	return out, e
}
func (p Paths) rememberChoices(o SyncOptions) error {
	if len(o.Choices)+len(o.GitChoices) == 0 {
		return nil
	}
	cs, e := p.conflicts()
	if e != nil {
		return e
	}
	rs, e := p.resolutions()
	if e != nil {
		return e
	}
	requests := map[string]string{}
	for k, v := range o.Choices {
		requests[k] = v
	}
	for k, v := range o.GitChoices {
		requests[k] = v
	}
	for key, take := range requests {
		found := false
		for _, c := range cs {
			if c.Key != key {
				continue
			}
			if _, ok := c.Choices[take]; !ok {
				return fmt.Errorf("invalid choice for %s", key)
			}
			rs[key] = Resolution{take, fingerprint(c.Choices)}
			found = true
		}
		if !found {
			return errors.New("conflict changed or no longer exists; inspect omai conflicts again")
		}
	}
	return atomicJSON(filepath.Join(p.State, "resolutions.json"), rs)
}
func (p Paths) clearResolutions(kind string) error {
	rs, e := p.resolutions()
	if e != nil {
		return e
	}
	for key := range rs {
		git := strings.HasPrefix(key, "git/")
		if (kind == "git") == git {
			delete(rs, key)
		}
	}
	return atomicJSON(filepath.Join(p.State, "resolutions.json"), rs)
}
func validChoices(rs map[string]Resolution, cs []Conflict) map[string]string {
	out := map[string]string{}
	for _, c := range cs {
		if r, ok := rs[c.Key]; ok && r.Fingerprint == fingerprint(c.Choices) {
			out[c.Key] = r.Choice
		}
	}
	return out
}
