package relai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var execLookPath = exec.LookPath

func validRemote(remote string) error {
	if remote == "" {
		return nil
	}
	if strings.ContainsAny(remote, "\x00\r\n") || strings.HasPrefix(remote, "-") {
		return errors.New("invalid Git remote")
	}
	if filepath.IsAbs(remote) {
		return nil
	}
	if strings.Contains(remote, "://") {
		u, e := url.Parse(remote)
		if e != nil {
			return errors.New("invalid remote URL")
		}
		if u.Scheme != "ssh" && u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "file" && u.Scheme != "git" {
			return errors.New("unsupported Git transport")
		}
		if u.User != nil {
			if _, pass := u.User.Password(); pass || u.Scheme != "ssh" {
				return errors.New("keep remote credentials in your local Git credential helper")
			}
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return errors.New("remote URL queries and fragments are unsupported")
		}
		return nil
	}
	// Ordinary scp-style SSH URLs; arbitrary remote helpers are forbidden.
	if strings.Contains(remote, ":") && !strings.Contains(remote, "::") && !strings.ContainsAny(remote, " \t") {
		return nil
	}
	return errors.New("use an absolute local path or an ordinary Git URL")
}

func validBranch(branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, ".") || strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.Contains(branch, "//") || strings.ContainsAny(branch, " ~^:?*[\\") {
		return errors.New("invalid sync branch")
	}
	for _, ch := range branch {
		if ch < 32 || ch == 127 {
			return errors.New("invalid sync branch")
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return errors.New("invalid sync branch")
		}
	}
	return nil
}

type gitStore struct {
	p   Paths
	dir string
	ctx context.Context
}

type boundedOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("Git output exceeds the safety limit")
	}
	return b.buffer.Write(p)
}

func (g gitStore) run(input []byte, extra []string, args ...string) ([]byte, error) {
	parent := g.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	base := []string{"--git-dir=" + g.dir, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=always", "-c", "gc.auto=0"}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Stdin = bytes.NewReader(input)
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			env = append(env, v)
		}
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME=Relai", "GIT_AUTHOR_EMAIL=relai@localhost", "GIT_COMMITTER_NAME=Relai", "GIT_COMMITTER_EMAIL=relai@localhost", "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=10")
	cmd.Env = append(env, extra...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = time.Second
	stderr := boundedOutput{limit: 64 << 10}
	stdout := boundedOutput{limit: maxSourceSize * 2}
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	e := cmd.Run()
	if e != nil {
		return nil, fmt.Errorf("git %s failed (transport details withheld)", args[0])
	}
	return stdout.buffer.Bytes(), nil
}
func (p Paths) gitStore(c Config) (gitStore, error) {
	g := gitStore{p: p, dir: filepath.Join(p.Data, "repository.git")}
	if e := validRemote(c.Remote); e != nil {
		return g, e
	}
	if e := noSymlink(g.dir); e != nil {
		return g, e
	}
	if e := os.MkdirAll(g.dir, 0700); e != nil {
		return g, e
	}
	if _, e := os.Stat(filepath.Join(g.dir, "HEAD")); os.IsNotExist(e) {
		if _, e = g.run(nil, nil, "init", "--bare"); e != nil {
			return g, e
		}
	}
	if _, e := g.run(nil, nil, "check-ref-format", "refs/heads/"+c.Branch); e != nil {
		return g, errors.New("invalid sync branch")
	}
	return g, nil
}
func (g gitStore) ref(name string) string {
	b, e := g.run(nil, nil, "rev-parse", "--verify", name)
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func (g gitStore) fetch(c Config) (string, error) {
	branch := "refs/heads/" + c.Branch
	tracking := "refs/remotes/origin/" + c.Branch
	incoming := fmt.Sprintf("refs/relai/incoming/%d", time.Now().UnixNano())
	defer func() {
		cleanup := g
		cleanup.ctx = context.Background()
		_, _ = cleanup.run(nil, nil, "update-ref", "-d", incoming)
	}()
	prior := g.ref(tracking)
	if _, e := g.run(nil, nil, "fetch", "--no-tags", "--no-write-fetch-head", "--", c.Remote, branch+":"+incoming); e != nil {
		return "", ErrOffline
	}
	remote := g.ref(incoming)
	if remote == "" {
		return "", errors.New("fetched branch has no commit")
	}
	if prior != "" && prior != remote {
		if _, e := g.run(nil, nil, "merge-base", "--is-ancestor", prior, remote); e != nil {
			if e = g.p.setConflicts("git", []Conflict{{Key: "git/history", Kind: "git", Reason: "the remote branch history was rewritten; restore its history or explicitly choose a new sync branch", Choices: Values{}}}); e != nil {
				return "", e
			}
			return "", ErrConflict
		}
	}
	if _, e := g.snapshot(remote); e != nil {
		return "", fmt.Errorf("remote rejected: %w", e)
	}
	args := []string{"update-ref", tracking, remote}
	if prior != "" {
		args = append(args, prior)
	}
	if _, e := g.run(nil, nil, args...); e != nil {
		return "", e
	}
	return remote, nil
}
func (g gitStore) push(c Config) error {
	branch := "refs/heads/" + c.Branch
	local := g.ref(branch)
	tracking := "refs/remotes/origin/" + c.Branch
	prior := g.ref(tracking)
	if _, e := g.run(nil, nil, "push", "--", c.Remote, branch+":"+branch); e != nil {
		return ErrOffline
	}
	args := []string{"update-ref", tracking, local}
	if prior != "" {
		args = append(args, prior)
	}
	_, e := g.run(nil, nil, args...)
	return e
}
func (g gitStore) snapshot(ref string) (Tree, error) {
	out := Tree{}
	if ref == "" {
		return out, nil
	}
	b, e := g.run(nil, nil, "ls-tree", "-rz", "--full-tree", ref)
	if e != nil {
		return nil, e
	}
	total := 0
	for _, line := range bytes.Split(b, []byte{0}) {
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte{'\t'}, 2)
		if len(parts) != 2 {
			return nil, errors.New("invalid Git tree")
		}
		meta := strings.Fields(string(parts[0]))
		path := string(parts[1])
		if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
			return nil, errors.New("remote tree contains links or unsupported objects")
		}
		if e = safeRel(path); e != nil {
			return nil, e
		}
		if !allowedSource(path) {
			return nil, fmt.Errorf("remote contains unapproved source path %s", path)
		}
		size, e := g.run(nil, nil, "cat-file", "-s", meta[2])
		if e != nil {
			return nil, e
		}
		n, e := strconv.Atoi(strings.TrimSpace(string(size)))
		if e != nil || n > maxFileSize {
			return nil, errors.New("remote file exceeds size limit")
		}
		total += n
		if total > maxSourceSize || len(out) >= 2048 {
			return nil, errors.New("remote source exceeds limits")
		}
		data, e := g.run(nil, nil, "cat-file", "blob", meta[2])
		if e != nil {
			return nil, e
		}
		if e = scanContent(path, data); e != nil {
			return nil, e
		}
		mode := uint32(0600)
		if meta[0] == "100755" {
			mode = 0700
		}
		out[path] = Blob{data, mode}
	}
	if len(out) > 0 {
		if _, _, e := parseSource(out); e != nil {
			return nil, e
		}
	}
	return out, nil
}
func equalBlob(a Blob, aok bool, b Blob, bok bool) bool {
	return aok == bok && (!aok || (bytes.Equal(a.Data, b.Data) && a.Mode == b.Mode))
}
func equalTree(a, b Tree) bool {
	if len(a) != len(b) {
		return false
	}
	for k, x := range a {
		y, ok := b[k]
		if !equalBlob(x, true, y, ok) {
			return false
		}
	}
	return true
}
func (g gitStore) commit(t Tree, parents []string) (string, error) {
	// A fresh index stages only validated source blobs. No add -A, filters,
	// working-tree hooks, copied Git settings, or host identity can enter a commit.
	if _, _, e := parseSource(t); e != nil {
		return "", e
	}
	index, e := os.CreateTemp(g.p.State, "git-index-*")
	if e != nil {
		return "", e
	}
	name := index.Name()
	_ = index.Close()
	_ = os.Remove(name)
	defer os.Remove(name)
	defer os.Remove(name + ".lock")
	env := []string{"GIT_INDEX_FILE=" + name}
	if _, e = g.run(nil, env, "read-tree", "--empty"); e != nil {
		return "", e
	}
	var input bytes.Buffer
	for _, path := range sortedKeys(t) {
		if e = safeRel(path); e != nil {
			return "", e
		}
		if !allowedSource(path) {
			return "", errors.New("unapproved commit path")
		}
		blob := t[path]
		if e = scanContent(path, blob.Data); e != nil {
			return "", e
		}
		oid, e := g.run(blob.Data, env, "hash-object", "-w", "--stdin")
		if e != nil {
			return "", e
		}
		mode := "100644"
		if blob.Mode&0100 != 0 {
			mode = "100755"
		}
		fmt.Fprintf(&input, "%s %s\t%s%c", mode, strings.TrimSpace(string(oid)), path, 0)
	}
	if _, e = g.run(input.Bytes(), env, "update-index", "-z", "--index-info"); e != nil {
		return "", e
	}
	tree, e := g.run(nil, env, "write-tree")
	if e != nil {
		return "", e
	}
	args := []string{"commit-tree", strings.TrimSpace(string(tree))}
	for _, parent := range parents {
		if parent != "" {
			args = append(args, "-p", parent)
		}
	}
	args = append(args, "-m", "Relay configuration")
	oid, e := g.run(nil, nil, args...)
	return strings.TrimSpace(string(oid)), e
}
func mergeTrees(base, local, remote Tree, takes map[string]string) (Tree, []Conflict) {
	out := Tree{}
	keys := map[string]bool{}
	for k := range base {
		keys[k] = true
	}
	for k := range local {
		keys[k] = true
	}
	for k := range remote {
		keys[k] = true
	}
	cs := []Conflict{}
	for _, k := range sortedKeys(keys) {
		b, bok := base[k]
		l, lok := local[k]
		r, rok := remote[k]
		pick, pok := l, lok
		switch {
		case equalBlob(l, lok, r, rok):
		case equalBlob(l, lok, b, bok):
			pick, pok = r, rok
		case equalBlob(r, rok, b, bok):
		default:
			variants := Values{"local": nil, "remote": nil}
			if lok {
				variants["local"] = raw(l)
			}
			if rok {
				variants["remote"] = raw(r)
			}
			key := "git/" + k
			take := takes[key]
			if take == "remote" {
				pick, pok = r, rok
			} else if take != "local" {
				cs = append(cs, Conflict{key, "git", "both computers changed this file since their common revision", variants})
			}
		}
		if pok {
			out[k] = pick
		}
	}
	return out, cs
}
func (p Paths) adoptTree(t, expected Tree) error {
	old, e := loadTree(p.Source)
	if e != nil {
		return e
	}
	if !equalTree(old, expected) {
		return errors.New("canonical source changed during remote sync; retrying without overwriting the edit")
	}
	if _, _, e = parseSource(t); e != nil {
		return e
	}
	changes, e := treeChanges(p.Source, old, t)
	if e != nil {
		return e
	}
	_, e = p.transaction(changes)
	return e
}
func (p Paths) gitSync(c Config, takes map[string]string, ctx context.Context) error {
	g, e := p.gitStore(c)
	if e != nil {
		return e
	}
	g.ctx = ctx
	branch := "refs/heads/" + c.Branch
	t, e := loadTree(p.Source)
	if e != nil {
		return e
	}
	local := g.ref(branch)
	lt, e := g.snapshot(local)
	if e != nil {
		return e
	}
	// Join an existing remote without manufacturing an empty first commit.
	if local == "" && c.Remote != "" && len(t) == 1 {
		var initial Manifest
		if json.Unmarshal(t["relai.json"].Data, &initial) == nil && same(raw(initial), raw(defaultManifest())) {
			refs, err := g.run(nil, nil, "ls-remote", "--heads", "--", c.Remote, branch)
			if err == nil && len(bytes.TrimSpace(refs)) > 0 {
				remote, e := g.fetch(c)
				if e != nil {
					return e
				}
				rt, e := g.snapshot(remote)
				if e != nil {
					return e
				}
				if e = p.adoptTree(rt, t); e != nil {
					return e
				}
				_, e = g.run(nil, nil, "update-ref", branch, remote)
				return e
			}
		}
	}
	if !equalTree(t, lt) {
		oid, e := g.commit(t, []string{local})
		if e != nil {
			return e
		}
		args := []string{"update-ref", branch, oid}
		if local != "" {
			args = append(args, local)
		}
		if _, e = g.run(nil, nil, args...); e != nil {
			return e
		}
		local = oid
		lt = t
	}
	if c.Remote == "" {
		return nil
	}
	refs, e := g.run(nil, nil, "ls-remote", "--heads", "--", c.Remote, branch)
	if e != nil {
		return ErrOffline
	}
	if len(bytes.TrimSpace(refs)) == 0 {
		if g.ref("refs/remotes/origin/"+c.Branch) != "" {
			if e = p.setConflicts("git", []Conflict{{Key: "git/history", Kind: "git", Reason: "the remote sync branch was deleted; restore it or explicitly choose a new branch", Choices: Values{}}}); e != nil {
				return e
			}
			return ErrConflict
		}
		return g.push(c)
	}
	remote, e := g.fetch(c)
	if e != nil {
		return e
	}
	rt, e := g.snapshot(remote)
	if e != nil {
		return fmt.Errorf("remote rejected: %w", e)
	}
	if remote == local {
		return nil
	}
	if _, e = g.run(nil, nil, "merge-base", "--is-ancestor", remote, local); e == nil {
		return g.push(c)
	}
	// A fast-forward still goes through validation and a journaled source update.
	if _, e = g.run(nil, nil, "merge-base", "--is-ancestor", local, remote); e == nil {
		if e = p.adoptTree(rt, lt); e != nil {
			return e
		}
		_, e = g.run(nil, nil, "update-ref", branch, remote, local)
		return e
	}
	baseID := ""
	if b, err := g.run(nil, nil, "merge-base", local, remote); err == nil {
		baseID = strings.TrimSpace(string(b))
	}
	base, e := g.snapshot(baseID)
	if e != nil {
		return e
	}
	_, candidates := mergeTrees(base, lt, rt, nil)
	rs, e := p.resolutions()
	if e != nil {
		return e
	}
	merged, cs := mergeTrees(base, lt, rt, validChoices(rs, candidates))
	if len(cs) > 0 {
		if e = p.setConflicts("git", cs); e != nil {
			return e
		}
		return ErrConflict
	}
	if _, _, e = parseSource(merged); e != nil {
		return e
	}
	oid, e := g.commit(merged, []string{local, remote})
	if e != nil {
		return e
	}
	if e = p.adoptTree(merged, lt); e != nil {
		return e
	}
	if _, e = g.run(nil, nil, "update-ref", branch, oid, local); e != nil {
		return e
	}
	return g.push(c)
}

// SnapshotIDs is diagnostic metadata only; Git author identity is deliberately constant.
func (p Paths) SnapshotIDs() (map[string]string, error) {
	c, e := p.LoadConfig()
	if e != nil {
		return nil, e
	}
	g, e := p.gitStore(c)
	if e != nil {
		return nil, e
	}
	return map[string]string{"local": g.ref("refs/heads/" + c.Branch), "remote": g.ref("refs/remotes/origin/" + c.Branch)}, nil
}
