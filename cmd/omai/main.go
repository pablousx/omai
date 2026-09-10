package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pablousx/omai/internal/omai"
)

func printJSON(v any) { e := json.NewEncoder(os.Stdout); e.SetIndent("", "  "); _ = e.Encode(v) }
func usage() {
	fmt.Print(`omai — Your AI setup, in sync.

Usage: omai <command> [options]
  setup        Guided first-run setup; --yes --remote URL for unattended setup
  inventory    List relevant file metadata without reading contents
  status       Sync health (--json for QML and automation)
  doctor       Check setup, source, daemon, conflicts and recovery
  sync         Reconcile providers and Git (--local skips the remote)
  conflicts    List conflicts and available choices
  resolve KEY --take CHOICE
               Provider conflict: canonical, codex, claude, opencode, personal
               Git conflict: local, remote (KEY starts with git/)
  rollback [ID] Restore a transaction and pause automatic sync
  daemon       run | install | start | stop | restart | pause | resume | uninstall
  personal add NAME --source personal/FILE --path '~/destination'
  backups list | prune [--dry-run] [--json]
  settings clear --yes
               Stop sync and clear omai preferences; keep configuration files
  install      Install this staged binary; --binary-only skips service integration
  version

omai never installs AI runtimes or packages; manage those with mise.
`)
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "omai:", e)
		if errors.Is(e, omai.ErrConflict) {
			os.Exit(2)
		}
		if errors.Is(e, omai.ErrOffline) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		usage()
		return nil
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	p := omai.NewPaths(home)
	hasJSON := false
	clean := []string{}
	for _, s := range args {
		if s == "--json" {
			hasJSON = true
		} else {
			clean = append(clean, s)
		}
	}
	args = clean
	switch args[0] {
	case "settings":
		if len(args) != 3 || args[1] != "clear" || args[2] != "--yes" {
			return errors.New("use settings clear --yes to stop sync and clear omai preferences while keeping configuration files")
		}
		if e = p.ClearSettings(); e != nil {
			return e
		}
		fmt.Println("Settings cleared. Configuration files are kept. Set up omai in the plugin to start syncing again.")
		return nil
	case "install":
		fs := flag.NewFlagSet("install", flag.ContinueOnError)
		only := fs.Bool("binary-only", false, "install executable only")
		recoverInstall := fs.Bool("recover", false, "restore an interrupted installation")
		rollbackInstall := fs.Bool("rollback", false, "restore the previous installation")
		if e = fs.Parse(args[1:]); e != nil {
			return e
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected install argument")
		}
		if *recoverInstall || *rollbackInstall {
			return p.RecoverInstall(*rollbackInstall)
		}
		return p.Install(false, *only)
	case "backups":
		if len(args) < 2 || (args[1] != "list" && args[1] != "prune") {
			return errors.New("usage: omai backups list|prune [--dry-run] [--json]")
		}
		fs := flag.NewFlagSet("backups", flag.ContinueOnError)
		dry := fs.Bool("dry-run", false, "preview cleanup")
		if e = fs.Parse(args[2:]); e != nil {
			return e
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected backups argument")
		}
		report, e := p.Backups(args[1] == "prune", *dry)
		if e != nil {
			return e
		}
		if hasJSON {
			printJSON(report)
		} else {
			fmt.Printf("%d backups, %d bytes; retained after cleanup: %d bytes\n", len(report.Items), report.Bytes, report.RemainingBytes)
			for _, b := range report.Items {
				fmt.Printf("%s  %s  %d bytes  cleanup=%t  %s\n", b.ID, b.Phase, b.Bytes, b.Remove, b.Reason)
			}
			for _, w := range report.Warnings {
				fmt.Println(w)
			}
		}
		return nil
	case "version":
		fmt.Println(omai.Version)
		return nil
	case "inventory":
		printJSON(p.Inventory())
		return nil
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		yes := fs.Bool("yes", false, "unattended setup")
		remote := fs.String("remote", "", "existing remote URL")
		branch := fs.String("branch", "main", "sync branch")
		machine := fs.String("machine", "This computer", "local display label")
		seed := fs.String("seed", "", "initial provider")
		providers := fs.String("providers", "", "comma-separated providers; default detect")
		noService := fs.Bool("no-service", false, "do not install or start systemd (tests/headless)")
		if e = fs.Parse(args[1:]); e != nil {
			return e
		}
		if !*yes {
			if fs.NFlag() > 0 {
				return errors.New("use --yes with setup flags, or run omai setup without flags for guided setup")
			}
			return p.GuidedSetup(os.Stdin, os.Stdout)
		}
		ps := []string{}
		if *providers != "" {
			ps = strings.Split(*providers, ",")
		}
		printJSON(p.Inventory())
		if e = p.Setup(omai.SetupOptions{Remote: *remote, Branch: *branch, Machine: *machine, Seed: *seed, Providers: ps, Service: !*noService}); e != nil {
			return e
		}
		fmt.Println("omai configured. Source:", p.Source)
		return nil
	case "status":
		s := p.Status()
		if hasJSON {
			printJSON(s)
		} else {
			fmt.Printf("omai: %s | daemon: %t | generation: %d\n", s.Health, s.Daemon, s.Generation)
			fmt.Printf("Recovery history: %d backups, %.1f MiB\n", s.BackupCount, float64(s.BackupBytes)/(1<<20))
			if s.LastSync != "" {
				fmt.Println("Last sync:", s.LastSync)
			}
			for _, pr := range s.Providers {
				fmt.Printf("  %s: detected=%t managed=%d\n", pr.Name, pr.Detected, pr.Managed)
			}
			if s.Error != "" {
				fmt.Println(s.Error)
			}
		}
		return nil
	case "doctor":
		s, issues := p.Doctor()
		if hasJSON {
			printJSON(map[string]any{"status": s, "issues": issues, "ok": len(issues) == 0})
		} else {
			fmt.Printf("omai %s: %s\n", s.Version, s.Health)
			fmt.Printf("Recovery history: %d backups, %.1f MiB\n", s.BackupCount, float64(s.BackupBytes)/(1<<20))
			if len(issues) == 0 {
				fmt.Println("All checks passed.")
			}
			for _, i := range issues {
				fmt.Println("-", i)
			}
		}
		if len(issues) > 0 {
			return errors.New("doctor found issues")
		}
		return nil
	case "sync":
		fs := flag.NewFlagSet("sync", flag.ContinueOnError)
		local := fs.Bool("local", false, "skip network")
		if e = fs.Parse(args[1:]); e != nil {
			return e
		}
		s, e := p.Sync(omai.SyncOptions{LocalOnly: *local})
		if hasJSON {
			printJSON(s)
		} else {
			fmt.Printf("omai: %s\n", s.Health)
		}
		return e
	case "conflicts":
		fs := flag.NewFlagSet("conflicts", flag.ContinueOnError)
		show := fs.String("show", "", "show a conflict's saved variants")
		if e = fs.Parse(args[1:]); e != nil {
			return e
		}
		if *show != "" {
			c, e := p.ConflictDetail(*show)
			if e != nil {
				return e
			}
			if hasJSON {
				printJSON(c)
				return nil
			}
			fmt.Println(c.Key + ": " + c.Reason)
			for name, v := range c.Choices {
				fmt.Printf("\n--- %s ---\n", name)
				if string(v) == "null" || len(v) == 0 {
					fmt.Println("(deleted)")
					continue
				}
				var b omai.Blob
				if json.Unmarshal(v, &b) == nil && b.Data != nil {
					fmt.Printf("%s\n", b.Data)
				} else {
					fmt.Printf("%s\n", v)
				}
			}
			fmt.Printf("\nResolve with: omai resolve %q --take CHOICE\n", c.Key)
			return nil
		}
		cs, e := p.Conflicts()
		if e != nil {
			return e
		}
		if hasJSON {
			printJSON(cs)
		} else {
			if len(cs) == 0 {
				fmt.Println("No conflicts.")
			}
			for _, c := range cs {
				fmt.Printf("%s: %s\n  choices: %s\n", c.Key, c.Reason, strings.Join(c.Choices, ", "))
			}
		}
		return nil
	case "resolve":
		if len(args) < 2 {
			return errors.New("usage: omai resolve KEY --take CHOICE")
		}
		key := args[1]
		fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
		take := fs.String("take", "", "explicit choice")
		if e = fs.Parse(args[2:]); e != nil {
			return e
		}
		if *take == "" {
			return errors.New("--take is required")
		}
		cs, e := p.Conflicts()
		if e != nil {
			return e
		}
		found := false
		for _, c := range cs {
			if c.Key == key {
				for _, v := range c.Choices {
					if v == *take {
						found = true
					}
				}
			}
		}
		if !found {
			return errors.New("no matching conflict choice")
		}
		o := omai.SyncOptions{}
		if strings.HasPrefix(key, "git/") {
			o.GitChoices = map[string]string{key: *take}
		} else {
			o.Choices = map[string]string{key: *take}
		}
		_, e = p.Sync(o)
		return e
	case "rollback":
		id := ""
		if len(args) > 1 {
			id = args[1]
		}
		if e = p.Rollback(id); e != nil {
			return e
		}
		fmt.Println("Rolled back. Automatic sync is paused; inspect the source, then omai daemon resume.")
		return nil
	case "daemon":
		if len(args) != 2 {
			return errors.New("usage: omai daemon run|install|start|stop|restart|pause|resume")
		}
		if args[1] == "run" {
			return p.Daemon(context.Background())
		}
		return p.Control(args[1])
	case "personal":
		if len(args) < 3 || args[1] != "add" {
			return errors.New("usage: omai personal add NAME --source personal/FILE --path '~/destination'")
		}
		fs := flag.NewFlagSet("personal add", flag.ContinueOnError)
		source := fs.String("source", "", "canonical path")
		path := fs.String("path", "", "home-relative destination")
		if e = fs.Parse(args[3:]); e != nil {
			return e
		}
		return p.Enroll(args[2], *source, *path)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
