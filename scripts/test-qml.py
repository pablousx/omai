#!/usr/bin/env python3
"""Exercise native Omarchy UI against fixtures, never the user's AI state."""
import argparse
import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument("--state", help="run one scenario")
parser.add_argument("--screenshots", type=Path, help="save fixture panel PNGs to this directory")
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
wayland = os.environ.get("WAYLAND_DISPLAY", "")
if not wayland:
    raise SystemExit("A Wayland session is required for native Omarchy UI validation.")
if not wayland.startswith("/"):
    wayland = str(Path(os.environ["XDG_RUNTIME_DIR"]) / wayland)
if args.screenshots:
    args.screenshots = args.screenshots.resolve()
    args.screenshots.mkdir(parents=True, exist_ok=True)
base = json.loads((root / "tests/fixtures/status.json").read_text())
conflict = {"key": "git/instructions.md", "kind": "git", "reason": "Both computers changed this file", "choices": ["local", "remote"]}
states = ["setup", "setup-form", "setup-submit", "setup-failure", "update", "clear-settings", "clear-failure", "healthy", "local", "offline", "pending", "paused", "error", "conflict", "mismatch", "malformed", "bad-shape", "failed", "actions", "stopped", "diagnostics", "diagnostics-error", "history", "empty-history", "review", "readout-failure", "rollback", "setup-validation", "keyboard", "details", "readout-race", "review-long"]
if args.state:
    assert args.state in states
    states = [args.state]
with tempfile.TemporaryDirectory(prefix="omai-qml-") as tmp:
    stage = Path(tmp)
    home, runtime = stage / "home", stage / "runtime"
    home.mkdir()
    theme = home / ".local/state/omarchy/current/theme"
    theme.mkdir(parents=True)
    (theme / "colors.toml").write_text('foreground = "#cdd6f4"\nbackground = "#1e1e2e"\naccent = "#89b4fa"\nred = "#f38ba8"\n')
    (theme / "shell.toml").write_text('[font]\nfamily = "DejaVu Sans"\n')
    runtime.mkdir(mode=0o700)
    for module in ("Ui", "Commons"):
        (stage / module).symlink_to(Path("/usr/share/omarchy/shell") / module)
    shutil.copytree(root / "qml", stage / "plugin/qml")
    shutil.copy2(root / "manifest.json", stage / "plugin/manifest.json")
    shutil.copytree(root / "scripts", stage / "plugin/scripts")
    fixture = stage / "fixture.json"
    record = stage / "commands.jsonl"
    fake_binary = home / ".local/share/omai/bin/omai"
    fake_binary.parent.mkdir(parents=True)
    fake = f'''#!{sys.executable}
import json, os, pathlib, sys, time
if sys.argv[1]=="status":
 print(pathlib.Path({str(fixture)!r}).read_text())
 sys.exit(1 if os.environ["OMAI_QML_STATE"]=="failed" else 0)
with pathlib.Path({str(record)!r}).open("a") as out: out.write(json.dumps(sys.argv[1:])+"\\n")
time.sleep(0.15)
state=os.environ["OMAI_QML_STATE"]
if sys.argv[1]=="doctor":
 if state=="readout-failure":print("invalid JSON");sys.exit(1)
 print(json.dumps(dict(ok=state!="diagnostics-error",issues=["Remote is unavailable"] if state=="diagnostics-error" else [])))
 sys.exit(1 if state=="diagnostics-error" else 0)
if sys.argv[1]=="backups":
 print(json.dumps(dict(bytes=4096,items=[] if state=="empty-history" else [dict(id="1788922830110201551",bytes=4096,protected=True,phase="complete",reason="Latest rollback generation")])))
 sys.exit(0)
if sys.argv[1]=="conflicts":
 import base64
 print(json.dumps(dict(key="providers/codex/instructions.md",reason="Both computers changed this file",choices=dict(local=dict(data=base64.b64encode(("Long saved text ✦ "*20000 if state=="review-long" else "Hello café ✦").encode()).decode()),remote=None))))
 sys.exit(0)
if sys.argv[1]=="rollback":sys.exit(0)
if sys.argv[1:]==["settings","clear","--yes"]:
 if os.environ["OMAI_QML_STATE"]=="clear-failure":
  print("Could not stop the daemon.",file=sys.stderr);sys.exit(1)
 p=pathlib.Path({str(fixture)!r});report=json.loads(p.read_text())
 report.update(configured=False,daemon=False,health="setup");p.write_text(json.dumps(report));sys.exit(0)
sys.exit(2)
'''
    bridge = stage / "plugin/scripts/plugin-setup"
    bridge.write_text(f'''#!{sys.executable}
import json, os, pathlib, sys, time
with pathlib.Path({str(record)!r}).open("a") as out: out.write(json.dumps(["plugin-setup"]+sys.argv[1:])+"\\n")
time.sleep(0.15)
if os.environ["OMAI_QML_STATE"]=="setup-failure" and "--source" not in sys.argv:
 print("Release unavailable. Your choices are saved in the form.",file=sys.stderr)
 sys.exit(1)
p=pathlib.Path({str(fixture)!r})
report=json.loads(p.read_text());report.update(configured=True,health="healthy",daemon=True,version="1.0.0")
p.write_text(json.dumps(report))
''')
    bridge.chmod(0o700)
    env = dict(os.environ)
    for key in list(env):
        if key.startswith(("XDG_", "CODEX_", "CLAUDE_", "OPENCODE_", "GIT_")) or key in ("DISPLAY", "DBUS_SESSION_BUS_ADDRESS"):
            env.pop(key, None)
    env.update(HOME=str(home), XDG_RUNTIME_DIR=str(runtime),
               XDG_CONFIG_HOME=str(home / ".config"), XDG_DATA_HOME=str(home / ".local/share"),
               XDG_STATE_HOME=str(home / ".local/state"), XDG_CACHE_HOME=str(stage / "cache"),
               WAYLAND_DISPLAY=wayland, QT_QPA_PLATFORM="wayland", QT_QPA_PLATFORMTHEME="basic",
               QT_QUICK_CONTROLS_STYLE="Basic", TZ="UTC")
    for state in states:
        record.unlink(missing_ok=True)
        report = copy.deepcopy(base)
        if state.startswith("setup-"):
            report.update(health="setup",configured=False,daemon=False)
        if state in ("local", "offline", "pending", "paused", "error", "conflict"):
            report["health"] = state
        report["paused"] = state == "paused"
        report["pending"] = state in ("pending", "offline")
        if state == "stopped":
            report["daemon"] = False
        if state == "error":
            report["error"] = "A managed file needs attention. Run Doctor for details."
        if state in ("conflict", "actions"):
            report["health"] = "conflict"
            report["conflicts"] = [conflict]
        if state in ("mismatch", "update"):
            report["version"] = "0.1.0"
        if state == "bad-shape":
            report["conflicts"] = [{"key": "broken", "choices": None}]
        fixture.write_text("{" if state == "malformed" else json.dumps(report))
        fake_binary.unlink(missing_ok=True)
        if state != "setup":
            fake_binary.write_text(fake)
            fake_binary.chmod(0o700)
        env["OMAI_QML_STATE"] = state
        screenshot = str(args.screenshots / (state + ".png")) if args.screenshots and state in ("setup", "setup-form", "healthy", "conflict", "stopped", "diagnostics", "diagnostics-error", "history", "empty-history", "review", "readout-failure", "rollback", "details", "paused", "offline", "mismatch") else ""
        # Use actual panel keyboard signals, including conflict buttons. A
        # synthetic screenshot grabs only our fixture content, never the desktop.
        script = '''import QtQuick
import Quickshell
import "plugin/qml" as Omai
ShellRoot {
  id: testRoot
  property string state: STATE
  property string capturePath: CAPTURE
  property int phase: 0
  function fail(message) { console.error("OMAI_QML_FAILED", state, message); Qt.exit(1) }
  function findObject(object, name, seen) {
    if (!object || seen.indexOf(object)>=0) return null
    seen.push(object)
    if (object.objectName === name) return object
    var children = object.data || object.children || []
    for (var i=0;i<children.length;i++) {
      var found = findObject(children[i],name,seen)
      if (found) return found
    }
    if (object.contentItem) {
      var content = object.contentItem
      if (Array.isArray(content) || content.length !== undefined) {
        for (var j=0;j<content.length;j++) { var item=findObject(content[j],name,seen); if(item)return item }
      } else return findObject(content,name,seen)
    }
    return null
  }
  function done() { console.log("OMAI_QML_PASSED",state);Qt.quit() }
  PanelWindow {
    anchors { top: true; left: true }
    implicitWidth: 32; implicitHeight: 32
    exclusionMode: ExclusionMode.Ignore
    color: "transparent"
    Omai.Omai { id: widget }
  }
  Timer {
    interval: 1200; running: true; repeat: true
    onTriggered: {
      if(testRoot.phase===6){
        if(widget.readoutError && testRoot.state!=="readout-failure"){testRoot.fail(widget.readoutError);return}
        if(testRoot.state==="readout-failure") {
          if(!widget.readoutError){testRoot.fail("readout failure missing");return}
        } else if(!widget.readoutSummary) {testRoot.fail("readout result missing");return}
        if(testRoot.state==="diagnostics-error" && widget.readoutEntries.length!==1){testRoot.fail("diagnostic exit 1 treated as empty result");return}
        if(testRoot.state==="readout-race" && (widget.readoutKind!=="backups" || widget.readoutSummary.indexOf("1 recovery point")<0)){testRoot.fail("stale readout appeared after switching views");return}
        if(testRoot.state==="review-long" && (!widget.previewTruncated || widget.viewActions.length!==3)){testRoot.fail("long preview has no full comparison action");return}
        if(testRoot.state==="empty-history" && widget.readoutSummary.indexOf("No recovery points")<0){testRoot.fail("empty history missing");return}
        if(testRoot.state==="review" && (widget.readoutEntries[0].body!=="Hello café ✦" || widget.readoutEntries[1].body.indexOf("deletes")<0)){testRoot.fail("saved text preview incorrect");return}
        if(testRoot.capturePath){widget.open();testRoot.phase=2;return}
        widget.escapeView()
        if(widget.readoutOpen){testRoot.fail("escape did not return to sync");return}
        testRoot.done();return
      }
      if(testRoot.phase===7){
        if(widget.busy || widget.actionMessage.indexOf("undone")<0){testRoot.fail("rollback result");return}
        testRoot.done();return
      }
      if (testRoot.phase === 1) { if (widget.busy || widget.actionMessage.indexOf("conflict")<0) {testRoot.fail("action result");return} testRoot.done();return }
      if (testRoot.phase === 3) {
        if(widget.busy){testRoot.fail("setup still running");return}
        if(testRoot.state === "setup-failure" && widget.setupFailed) {
          if(!widget.setupOpen || widget.actionError.indexOf("Release unavailable")<0){testRoot.fail("setup error not displayed in form");return}
          testRoot.findObject(widget,"omai-setup-source",[]).clicked()
          testRoot.phase=4;return
        }
        if(widget.setupOpen || !widget.report.configured || widget.updateNeeded || widget.actionError){testRoot.fail("setup/update did not finish");return}
        testRoot.done();return
      }
      if(testRoot.phase===4){
        if(widget.busy || widget.setupOpen || widget.actionError || !widget.report.configured){testRoot.fail("source retry failed");return}
        testRoot.done();return
      }
      if(testRoot.phase===5){
        if(widget.busy){testRoot.fail("reset still running");return}
        if(testRoot.state==="clear-failure"){
          if(!widget.report.configured || widget.setupOpen || widget.actionError.indexOf("Could not stop")<0){testRoot.fail("reset failure lost settings view");return}
        }else if(widget.report.configured || !widget.setupOpen || widget.actionError){testRoot.fail("reset did not return to setup");return}
        testRoot.done();return
      }
      if (testRoot.phase === 2) {
        // An outside click can dismiss a fixture on an interactive desktop.
        if (!widget.opened) {widget.open();return}
        widget.actionError="";widget.actionMessage=""
        widget.now=new Date("2026-09-08T12:00:30Z").getTime()
        var scroll=testRoot.findObject(widget,"omai-scroll",[])
        if (!scroll || scroll.contentY !== 0) {testRoot.fail("panel did not open at the top");return}
        var content=testRoot.findObject(widget,"omai-panel-content",[])
        if (!content) {testRoot.fail("cannot find preview content");return}
        var capture = content
        while (capture.parent && !("borderSpec" in capture)) capture = capture.parent
        if (!capture.QsWindow.window) return
        if (!capture.grabToImage(function(result){if(!result.saveToFile(testRoot.capturePath))testRoot.fail("save screenshot");else testRoot.done()})) testRoot.fail("grab screenshot")
        stop();return
      }
      if (["malformed","bad-shape","failed"].indexOf(testRoot.state)>=0) {
        if (!widget.statusError || widget.enabledAction({args:["sync"]})) {testRoot.fail("invalid status accepted");return}
        testRoot.done();return
      }
      if (!widget.statusLoaded || widget.statusError || widget.pluginVersion !== "1.0.0") {testRoot.fail("status/version loading: "+widget.statusError);return}
      if(testRoot.state.indexOf("clear-")===0){
        widget.runAction(widget.baseActions.find(x=>x.args[0]==="advanced"))
        widget.runAction(widget.advancedActions.find(x=>x.args[0]==="confirm-clear"))
        if(!widget.clearConfirm || widget.busy){testRoot.fail("reset skipped confirmation");return}
        widget.runAction(widget.viewActions[0])
        if(widget.clearConfirm || widget.busy){testRoot.fail("reset cancellation failed");return}
        widget.runAction(widget.advancedActions.find(x=>x.args[0]==="confirm-clear"))
        var confirm=testRoot.findObject(widget,"omai-clear-confirm",[])
        if(!confirm){testRoot.fail("missing reset confirmation");return}
        confirm.clicked();confirm.clicked()
        if(!widget.busy){testRoot.fail("confirmed reset did not launch");return}
        testRoot.phase=5;return
      }
      if(testRoot.state.indexOf("setup-")===0){
        widget.runAction(widget.baseActions[0])
        if(!widget.setupOpen || widget.busy){testRoot.fail("setup did not open inline");return}
        var remote=testRoot.findObject(widget,"omai-setup-remote",[])
        var machine=testRoot.findObject(widget,"omai-setup-machine",[])
        var seed=testRoot.findObject(widget,"omai-setup-seed",[])
        var cancel=testRoot.findObject(widget,"omai-setup-cancel",[])
        var submit=testRoot.findObject(widget,"omai-setup-submit",[])
        if(!remote||!machine||!seed||!cancel||!submit){testRoot.fail("setup fields missing");return}
        if(!testRoot.findObject(widget,"omai-keyboard",[]).blocked){testRoot.fail("panel shortcuts intercept form typing");return}
        remote.text="git@example.com:personal/ai.git";machine.text="My laptop";seed.currentIndex=1
        cancel.clicked()
        if(widget.setupOpen||widget.busy){testRoot.fail("cancel launched setup");return}
        widget.runAction(widget.baseActions[0])
        if(remote.text!=="git@example.com:personal/ai.git"){testRoot.fail("cancel lost draft");return}
        if(testRoot.state==="setup-form"){
          if(testRoot.capturePath){widget.open();testRoot.phase=2;return}
          testRoot.done();return
        }
        if(testRoot.state==="setup-validation"){
          remote.text="not a git remote";submit.clicked()
          if(widget.busy){testRoot.fail("invalid remote launched setup");return}
          remote.text="git@example.com:personal/ai.git"
          machine.accepted()
        }else {submit.clicked();submit.clicked()}
        if(!widget.busy){testRoot.fail("setup did not start");return}
        testRoot.phase=3;return
      }
      if(testRoot.state==="update"){
        widget.runAction(widget.baseActions.find(x=>x.args[0]==="plugin-update"))
        if(!widget.busy){testRoot.fail("update did not start inline");return}
        testRoot.phase=3;return
      }
      if (testRoot.state === "mismatch") {
        if (!widget.updateNeeded || widget.enabledAction({args:["sync"]})) {testRoot.fail("version mismatch not handled");return}
        testRoot.done();return
      }
      if(["diagnostics","diagnostics-error","history","empty-history","review","readout-failure","readout-race","review-long"].indexOf(testRoot.state)>=0){
        var command=testRoot.state==="history"||testRoot.state==="empty-history"?["backups","list"]:(testRoot.state==="review"||testRoot.state==="review-long")?["conflicts","--show","providers/codex/instructions.md"]:["doctor"]
        widget.runAction({maintenance:true,args:command})
        if(testRoot.state==="readout-race"){
          widget.closeReadout()
          widget.runAction({maintenance:true,args:["backups","list"]})
        }
        if(!widget.readoutOpen||widget.busy){testRoot.fail("details did not open inline");return}
        testRoot.phase=6;return
      }
      if(testRoot.state==="rollback"){
        widget.runAction({args:["rollback"]})
        if(!widget.pendingConfirmation||widget.busy||widget.selectedAction!==0){testRoot.fail("rollback missing safe confirmation");return}
        widget.escapeView()
        if(widget.pendingConfirmation||widget.busy){testRoot.fail("rollback escape failed");return}
        widget.runAction({args:["rollback"]})
        widget.runAction(widget.viewActions[1]);widget.runAction(widget.viewActions[1])
        if(!widget.busy){testRoot.fail("rollback did not launch");return}
        testRoot.phase=7;return
      }
      if(testRoot.state==="keyboard"){
        widget.selectedAction=0;widget.actionInFlight=true
        widget.moveSelection(1)
        if(widget.actions[widget.selectedAction].args[0]!=="details"){testRoot.fail("keyboard landed on disabled action");return}
        widget.actionInFlight=false
        testRoot.done();return
      }
      if(testRoot.state==="details"){
        widget.runAction(widget.baseActions.find(x=>x.args[0]==="details"))
        if(!widget.detailsOpen){testRoot.fail("details did not expand");return}
        if(testRoot.capturePath){widget.open();testRoot.phase=2;return}
        testRoot.done();return
      }
      if(testRoot.state==="stopped"){
        if(widget.primaryAction.args.join(" ")!=="daemon start"){testRoot.fail("stopped next action incorrect");return}
        if(testRoot.capturePath){widget.open();testRoot.phase=2;return}
        testRoot.done();return
      }
      var expected=testRoot.state==="actions"?"conflict":testRoot.state
      if(widget.report.health!==expected){testRoot.fail("unexpected health "+widget.report.health);return}
      if(testRoot.state==="actions"){
        var keyboard=testRoot.findObject(widget,"omai-keyboard",[])
        if (!keyboard){testRoot.fail("missing keyboard catcher");return}
        widget.selectedAction=0
        keyboard.moveRequested(0,1)
        if(widget.selectedAction!==1||widget.actions[1].args[0]!=="resolve"){testRoot.fail("conflict keyboard selection");return}
        keyboard.tabRequested(-1)
        if(widget.selectedAction!==0){testRoot.fail("backtab");return}
        keyboard.moveRequested(0,1)
        keyboard.activateRequested()
        if(!widget.pendingConfirmation || widget.busy || widget.selectedAction!==0){testRoot.fail("conflict choice skipped confirmation");return}
        keyboard.moveRequested(0,1)
        keyboard.activateRequested()
        keyboard.activateRequested()
        if(!widget.busy){testRoot.fail("action did not start");return}
        testRoot.phase=1;return
      }
      // Exercise result messages without launching real tools.
      widget.finishAction(3,"","")
      if(widget.actionError || widget.actionMessage.indexOf("delayed")<0){testRoot.fail("offline result");return}
      widget.finishAction(1,"","Fixture failure")
      if(widget.actionError!=="Fixture failure"){testRoot.fail("error result");return}
      widget.actionError="";widget.actionMessage=""
      if(testRoot.capturePath){widget.open();testRoot.phase=2;return}
      testRoot.done()
    }
  }
}
'''.replace("STATE", json.dumps(state)).replace("CAPTURE", json.dumps(screenshot))
        (stage / "shell.qml").write_text(script)
        result = subprocess.run(["quickshell", "--no-color", "-p", str(stage / "shell.qml")],
                                env=env, text=True, capture_output=True, timeout=15)
        output = result.stdout + result.stderr
        if result.returncode or "OMAI_QML_PASSED " + state not in output or "ERROR" in output or "WARN scene" in output:
            raise SystemExit(output)
        if state == "actions":
            commands = [json.loads(line) for line in record.read_text().splitlines()]
            assert commands == [["resolve", "git/instructions.md", "--take", "local"]], commands
        if state == "rollback":
            commands = [json.loads(line) for line in record.read_text().splitlines()]
            assert commands == [["rollback"]], commands
        if state.startswith("clear-"):
            commands = [json.loads(line) for line in record.read_text().splitlines()]
            assert commands == [["settings", "clear", "--yes"]], commands
        if state in ("setup-submit", "setup-failure", "update", "setup-form", "setup-validation"):
            commands = [json.loads(line) for line in record.read_text().splitlines()] if record.exists() else []
            expected = [] if state == "setup-form" else [["plugin-setup", "update"]] if state == "update" else [["plugin-setup", "setup", "--remote", "git@example.com:personal/ai.git", "--machine", "My laptop", "--seed", "codex"]]
            if state == "setup-failure":
                expected.append(expected[0][:2] + ["--source"] + expected[0][2:])
            assert commands == expected, commands
        print("PASS: native QML " + state, flush=True)
