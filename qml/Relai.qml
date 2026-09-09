pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import QtQuick.Layouts
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "pablousx.relai"
  ipcTarget: "pablousx.relai"
  manageIpc: false
  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  property var report: ({health: "setup", configured: false, daemon: false, providers: [], machines: [], conflicts: [], warnings: []})
  property bool statusLoaded: false
  property bool statusPending: false
  property string statusError: ""
  property string pluginVersion: ""
  property var actionItems: ({})
  property var activeAction: null
  property var pendingConfirmation: null
  property bool detailsOpen: false
  property bool readoutOpen: false
  property string readoutKind: ""
  property string readoutTitle: ""
  property string readoutSummary: ""
  property var readoutEntries: []
  property var readoutArgs: []
  property string readoutError: ""
  property bool readoutTimedOut: false
  property bool readoutPending: false
  property bool readoutQueued: false
  property bool previewTruncated: false
  property double now: Date.now()
  readonly property bool dashboard: !setupOpen && !readoutOpen && !pendingConfirmation
  property bool actionInFlight: false
  readonly property bool busy: actionInFlight || actionProc.running || setupProc.running || terminalProc.running
  property bool setupOpen: false
  property bool advancedOpen: false
  readonly property bool clearConfirm: !!pendingConfirmation && pendingConfirmation.args[0] === "settings"
  property bool clearingSettings: false
  property bool setupFailed: false
  property string setupMode: ""
  property var setupArguments: []
  readonly property bool updateNeeded: statusLoaded && pluginVersion !== "" && report.version !== pluginVersion
  property string actionMessage: ""
  property string actionError: ""
  property int selectedAction: 0
  property string selectionKey: ""
  property bool keyboardNavigation: false
  property string returnSelectionKey: ""
  onOpenedChanged: {
    if (opened) {
      keyboardNavigation = false
      scroll.contentY = 0
    }
  }
  readonly property string launcher: decodeURIComponent(Qt.resolvedUrl("../scripts/relai").toString().replace(/^file:\/\//, ""))
  readonly property string setupLauncher: decodeURIComponent(Qt.resolvedUrl("../scripts/plugin-setup").toString().replace(/^file:\/\//, ""))
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property color secondary: Util.alpha(foreground, 0.62)
  readonly property color healthColor: statusError ? Color.urgent : report.health === "conflict" || report.health === "error" ? Color.urgent
    : report.health === "offline" || report.health === "paused" || (report.configured && !report.daemon) ? Color.accent : foreground
  readonly property string healthLabel: statusError ? "Status unavailable" : !statusLoaded ? "Checking your setup…" : updateNeeded ? "Update available"
    : !report.configured ? "Bring your AI setup along"
    : report.paused ? "Sync is paused"
    : report.health === "conflict" ? "Choose which changes to keep"
    : report.health === "offline" ? "Waiting for a connection"
    : report.health === "error" ? "Sync needs attention"
    : !report.daemon ? "Automatic sync is off"
    : report.pending ? "Changes ready to sync"
    : report.health === "local" ? "Your local setup is in sync" : "Your setup is up to date"
  readonly property string healthDescription: statusError ? statusError : !statusLoaded ? "This usually takes a moment."
    : updateNeeded ? "Install Relai " + pluginVersion + " to continue syncing with this plugin."
    : !report.configured ? "Keep your instructions, settings, skills, and MCPs ready on every computer."
    : report.paused ? "Your files are kept as they are. Resume when you want changes to sync again."
    : report.health === "conflict" ? "Both versions are saved. Review them below before choosing one."
    : report.health === "offline" ? "Local changes are saved. Relai will retry automatically when connected."
    : report.health === "error" ? "Your saved files are available. Check what needs attention before retrying."
    : !report.daemon ? "Start automatic sync to watch for changes in the background."
    : report.pending ? "Your changes are saved locally and waiting to reach the remote."
    : !report.remote_configured ? "Changes stay on this computer. No Git remote is connected."
    : "Changes sync automatically in the background. You can keep working."
  readonly property var primaryAction: !statusLoaded || statusError ? {label: "Check again", maintenance: true, args: ["refresh"]}
    : updateNeeded ? {label: "Update Relai", maintenance: true, args: ["plugin-update"]}
    : !report.configured ? {label: "Set up Relai", maintenance: true, args: ["plugin-setup"]}
    : !report.daemon ? {label: "Start automatic sync", args: ["daemon", "start"]}
    : report.paused ? {label: "Resume syncing", args: ["daemon", "resume"]}
    : report.health === "error" ? {label: "Check sync issues", maintenance: true, args: ["doctor"]}
    : {label: report.health === "offline" ? "Try syncing again" : "Sync now", args: ["sync"]}
  readonly property var baseActions: {
    var result = [primaryAction]
    if (report.configured && statusLoaded && !statusError && !updateNeeded && report.daemon && !report.paused)
      result.push({label: "Pause syncing", args: ["daemon", "pause"]})
    if (setupFailed && !setupOpen) result.push({label: "Build local copy and retry", maintenance: true, args: ["plugin-source"]})
    if (report.configured) result.push({label: detailsOpen ? "Hide details" : "Sync details", maintenance: true, args: ["details"]})
    result.push({label: advancedOpen ? "Hide advanced" : "Advanced", maintenance: true, args: ["advanced"]})
    return result
  }
  readonly property var advancedActions: !advancedOpen ? [] : [
    {label: "Check sync issues", maintenance: true, args: ["doctor"]},
    {label: "Recovery history", maintenance: true, args: ["backups", "list"]},
    {label: "Undo last apply…", args: ["rollback"], destructive: true, disabled: !report.backup_count},
    {label: "Update Relai", maintenance: true, args: ["plugin-update"]},
    {label: report.daemon ? "Stop automatic sync" : "Start automatic sync", args: ["daemon", report.daemon ? "stop" : "start"]},
    {label: "Clear settings (keeps configs)…", maintenance: true, args: ["confirm-clear"], destructive: true}
  ].filter(x => actionKey(x) !== actionKey(primaryAction) && (report.configured || ["doctor", "plugin-update"].indexOf(x.args[0]) >= 0))
  readonly property var viewActions: pendingConfirmation ? [
    {label: "Cancel", maintenance: true, args: ["cancel-confirm"]},
    {label: pendingConfirmation.confirmLabel, maintenance: true, destructive: true, args: ["accept-confirm"]}
  ] : readoutOpen ? [
    {label: "Back to sync", maintenance: true, args: ["close-readout"]},
    {label: "Check again", maintenance: true, args: ["reload-readout"]}
  ].concat(previewTruncated ? [{label: "Full comparison ↗", maintenance: true, hint: "Opens the complete saved versions in a terminal.", args: ["full-conflict"]}] : []) : []
  readonly property var actions: {
    if (viewActions.length) return viewActions
    var result = []
    for (var c of (report.conflicts || [])) {
      result.push({label: "Review versions", maintenance: true, args: ["conflicts", "--show", c.key]})
      for (var choice of c.choices) result.push({label: choiceLabel(choice), args: ["resolve", c.key, "--take", choice]})
    }
    return result.concat(baseActions, advancedActions)
  }
  readonly property int conflictActionCount: (report.conflicts || []).reduce((n, c) => n + 1 + c.choices.length, 0)
  function conflictOffset(index) {
    var offset = 0
    for (var i = 0; i < index; i++) offset += 1 + report.conflicts[i].choices.length
    return offset
  }
  function choiceLabel(choice) {
    return choice === "local" ? "Keep this computer’s version" : choice === "remote" ? "Keep remote version"
      : choice === "canonical" ? "Keep synced source" : "Keep " + (choice === "claude" ? "Claude Code" : choice === "codex" ? "Codex" : choice === "opencode" ? "OpenCode" : choice) + " version"
  }
  function actionKey(action) { return action ? JSON.stringify(action.args) : "" }
  onActionsChanged: {
    var index = actions.findIndex(action => JSON.stringify(action.args) === selectionKey)
    selectedAction = index >= 0 ? index : 0
    selectionKey = actions.length ? JSON.stringify(actions[selectedAction].args) : ""
    Qt.callLater(ensureSelectedVisible)
  }
  onSelectedActionChanged: {
    if (actions[selectedAction]) selectionKey = JSON.stringify(actions[selectedAction].args)
    Qt.callLater(ensureSelectedVisible)
  }
  function ensureSelectedVisible() {
    if (!keyboardNavigation) return
    var item = actionItems[actionKey(actions[selectedAction])]
    revealItem(item)
  }
  function revealItem(item) {
    if (!item) return
    var y = item.mapToItem(content, 0, 0).y
    if (y < scroll.contentY) scroll.contentY = y
    else if (y + item.height > scroll.contentY + scroll.height) scroll.contentY = Math.max(0, y + item.height - scroll.height)
  }
  function moveSelection(direction) {
    keyboardNavigation = true
    keyCatcher.forceActiveFocus()
    for (var i = 0; i < actions.length; i++) {
      selectedAction = (selectedAction + direction + actions.length) % actions.length
      if (enabledAction(actions[selectedAction])) break
    }
    Qt.callLater(ensureSelectedVisible)
  }
  function enabledAction(action) {
    if (!action || action.disabled) return false
    var name = action.args[0]
    if (["close-readout", "cancel-confirm", "details", "advanced"].indexOf(name) >= 0) return true
    if (name === "reload-readout") return !readoutPending
    if (name === "refresh") return !statusPending
    if (busy) return false
    if (name === "sync" && report.paused) return false
    if (name === "accept-confirm") return statusLoaded && !statusError && !updateNeeded
    return action.maintenance || (statusLoaded && report.configured && !statusError && !updateNeeded)
  }
  function escapeView() {
    if (pendingConfirmation) { cancelConfirmation(); return }
    if (readoutOpen) { closeReadout(); return }
    if (setupOpen && !busy) { setupOpen = false; keyCatcher.forceActiveFocus(); return }
    root.close()
  }
  function cancelConfirmation() {
    pendingConfirmation = null
    restoreSelection()
  }
  function restoreSelection() {
    var key = returnSelectionKey
    Qt.callLater(function() {
      var index = root.actions.findIndex(x => root.actionKey(x) === key)
      if (index >= 0) root.selectedAction = index
      keyCatcher.forceActiveFocus()
    })
  }
  function refresh() {
    if (setupProc.running) return
    if (!statusProc.running && !statusPending) { statusPending = true; statusDeadline.restart(); statusProc.running = true }
  }
  function acceptStatus(text) {
    try {
      var next = JSON.parse(text)
      if (!next || typeof next.health !== "string" || typeof next.version !== "string" || typeof next.configured !== "boolean" || typeof next.daemon !== "boolean"
          || !Array.isArray(next.providers) || !Array.isArray(next.machines) || !Array.isArray(next.conflicts) || !Array.isArray(next.warnings)) throw new Error("Invalid status shape")
      if (["setup", "healthy", "local", "offline", "pending", "paused", "error", "conflict"].indexOf(next.health) < 0) throw new Error("Unknown health state")
      for (var provider of next.providers) if (!provider || typeof provider.name !== "string" || typeof provider.detected !== "boolean" || typeof provider.managed !== "number") throw new Error("Invalid provider")
      for (var machine of next.machines) if (!machine || typeof machine.name !== "string" || typeof machine.local !== "boolean") throw new Error("Invalid machine")
      if (next.warnings.some(x => typeof x !== "string")) throw new Error("Invalid warning")
      for (var c of next.conflicts) {
        if (typeof c.key !== "string" || typeof c.reason !== "string" || !Array.isArray(c.choices) || c.choices.some(x => typeof x !== "string")) throw new Error("Invalid conflict")
      }
      if (!statusLoaded) selectionKey = ""
      report = next
      statusLoaded = true
      statusError = ""
    } catch (e) { statusError = "Could not read Relai status. Try checking again, or open Advanced → Check sync issues." }
  }
  function finishAction(code, stdout, stderr) {
    actionInFlight = false
    actionStartDeadline.stop()
    actionError = ""
    if (code === 2) actionMessage = "A conflict needs a decision. Both versions are saved below."
    else if (code === 3) actionMessage = "Remote sync is delayed. Local changes are saved and will retry."
    else if (code !== 0) { actionMessage = ""; actionError = String(stderr).trim().slice(-1000) || "That didn’t finish. Try again or check sync issues in Advanced." }
    else {
      var args = activeAction ? activeAction.args : []
      actionMessage = args[0] === "sync" ? "Sync complete." : args[0] === "resolve" ? "Your choice was applied."
        : args[0] === "rollback" ? "Last apply undone. Sync is paused so you can review your files."
        : args[1] === "pause" ? "Sync paused." : args[1] === "resume" ? "Sync resumed."
        : args[1] === "stop" ? "Automatic sync stopped." : args[1] === "start" ? "Automatic sync started." : "Done."
      feedbackDeadline.restart()
    }
    if (clearingSettings) {
      clearingSettings = false
      if (code === 0) {
        advancedOpen = false; setupFailed = false
        setupForm.clearDraft()
        setupOpen = true
        actionMessage = "Relai settings cleared. Your configuration files are kept."
        Qt.callLater(function() { setupForm.focusFirst() })
      }
    }
    refresh()
  }
  function requestConfirmation(action) {
    returnSelectionKey = actionKey(action)
    pendingConfirmation = action
    selectedAction = 0
    keyboardNavigation = true
    scroll.contentY = 0
    Qt.callLater(function() { keyCatcher.forceActiveFocus() })
  }
  function runAction(action) {
    if (!enabledAction(action)) return
    var name = action.args[0]
    if (name === "refresh") { refresh(); return }
    if (name === "details") { detailsOpen = !detailsOpen; Qt.callLater(function() { root.revealItem(syncDetails) }); return }
    if (name === "advanced") { advancedOpen = !advancedOpen; Qt.callLater(function() { root.revealItem(advancedSection) }); return }
    if (name === "cancel-confirm") { cancelConfirmation(); return }
    if (name === "close-readout") { closeReadout(); return }
    if (name === "reload-readout") { loadReadout(); return }
    if (name === "full-conflict") {
      actionMessage = "The full comparison is open in a terminal."; actionError = ""
      actionInFlight = true
      actionStartDeadline.restart()
      var terminal = decodeURIComponent(Qt.resolvedUrl("../scripts/terminal").toString().replace(/^file:\/\//, ""))
      terminalProc.command = ["omarchy-launch-terminal", terminal].concat(readoutArgs.filter(x => x !== "--json"))
      terminalProc.running = true
      return
    }
    if (name === "accept-confirm") {
      if (!pendingConfirmation) return
      var confirmed = pendingConfirmation
      pendingConfirmation = null
      executeAction(confirmed)
      return
    }
    if (name === "confirm-clear") {
      if (updateNeeded || !report.configured) return
      requestConfirmation({args: ["settings", "clear", "--yes"], confirmLabel: "Clear settings", title: "Start setup again?",
        description: "This stops automatic sync and clears Relai’s connection settings and preferences. Your synced source, provider files, Git history, and recovery history are kept."})
      return
    }
    if (name === "rollback") {
      requestConfirmation({args: action.args, confirmLabel: "Undo last apply", title: "Undo the last applied change?",
        description: "Restore the previous configuration from this computer’s latest recovery point. Sync will pause so you can review the restored files before sharing them."})
      return
    }
    if (name === "resolve") {
      requestConfirmation({args: action.args, confirmLabel: choiceLabel(action.args[3]), title: "Apply this version?",
        description: choiceLabel(action.args[3]) + " for “" + action.args[1] + "”. This replaces the other version for this item and resumes reconciliation."})
      return
    }
    if (["doctor", "backups", "conflicts"].indexOf(name) >= 0) { openReadout(action); return }
    if (name === "plugin-setup") {
      setupOpen = true
      actionError = ""; actionMessage = ""; setupFailed = false
      Qt.callLater(function() { setupForm.focusFirst() })
      return
    }
    if (name === "plugin-update") { beginSetup("update", [], false); return }
    if (name === "plugin-source") { beginSetup(setupMode, setupArguments, true); return }
    executeAction(action)
  }
  function executeAction(action) {
    if (busy) return
    activeAction = action
    clearingSettings = action.args[0] === "settings"
    actionInFlight = true
    feedbackDeadline.stop()
    actionStartDeadline.restart()
    actionMessage = action.args[0] === "sync" ? "Syncing your changes…" : action.args[0] === "rollback" ? "Restoring the previous configuration…"
      : action.args[0] === "resolve" ? "Applying your choice…" : clearingSettings ? "Clearing Relai settings…" : "Updating automatic sync…"
    actionError = ""
    actionProc.command = [root.launcher].concat(action.args)
    actionProc.running = true
  }
  function openReadout(action) {
    returnSelectionKey = actionKey(action)
    readoutKind = action.args[0]
    readoutTitle = readoutKind === "doctor" ? "Sync check" : readoutKind === "backups" ? "Recovery history" : "Review saved versions"
    readoutArgs = action.args.concat(["--json"])
    readoutOpen = true
    scroll.contentY = 0
    loadReadout()
  }
  function closeReadout() {
    readoutOpen = false
    readoutPending = false
    readoutQueued = false
    readoutDeadline.stop()
    if (readoutProc.running) readoutProc.signal(15)
    restoreSelection()
  }
  function loadReadout() {
    if (!readoutOpen) return
    readoutError = ""; readoutEntries = []; readoutSummary = ""; readoutTimedOut = false; previewTruncated = false
    readoutPending = true
    if (readoutProc.running) {
      readoutQueued = true
      readoutProc.signal(15)
      return
    }
    readoutQueued = false
    readoutProc.command = [root.launcher].concat(readoutArgs)
    readoutProc.running = true
    readoutDeadline.restart()
  }
  function acceptReadout(code, text, error) {
    if (!readoutOpen || readoutTimedOut) return
    try {
      var data = JSON.parse(text)
      var entries = []
      if (readoutKind === "doctor") {
        if (!data || !Array.isArray(data.issues) || typeof data.ok !== "boolean") throw new Error("Invalid check result")
        readoutSummary = data.ok ? "All checks passed. Relai is ready to sync." : "Here’s what needs attention. Your saved configuration is still available."
        entries = data.issues.map(x => ({title: "Check this", body: String(x)}))
      } else if (readoutKind === "backups") {
        if (code !== 0 || !data || !Array.isArray(data.items)) throw new Error("Invalid history result")
        readoutSummary = data.items.length ? countLabel(data.items.length, "recovery point") + " · " + formatBytes(data.bytes) + ". These undo local changes; your synced configuration lives in Git." + (data.items.length > 10 ? " Showing the latest 10." : "")
          : "No recovery points yet. Relai creates one before applying a configuration change."
        entries = data.items.slice(-10).reverse().map(x => ({title: /^\d{19}$/.test(x.id) ? prettyTime(new Date(Number(x.id) / 1000000).toISOString()) : "Recovery point " + x.id, body: formatBytes(x.bytes) + " · " + (x.protected ? "Kept for recovery" : "Eligible for automatic cleanup") + (x.reason === "Latest rollback generation" ? "\nMost recent applied change" : x.phase !== "complete" && x.phase !== "recovered" ? "\nNeeds a recovery check" : "")}))
      } else {
        if (code !== 0 || !data || !data.choices || typeof data.choices !== "object") throw new Error("Invalid saved versions")
        readoutSummary = data.key + " — " + data.reason
        for (var key of Object.keys(data.choices)) {
          var value = data.choices[key]
          var body = value === null ? "(This version deletes the item.)" : value && value.data !== undefined ? decodeBlob(value.data) : JSON.stringify(value, null, 2)
          if (body.length > 12000) previewTruncated = true
          entries.push({title: choiceLabel(key), body: body.length > 12000 ? body.slice(0, 12000) + "\n… Preview shortened. Use Full comparison to read the rest before choosing." : body})
        }
      }
      readoutEntries = entries
    } catch (e) { readoutError = String(error).trim().slice(-1000) || "Couldn’t load these details. Check again to retry." }
  }
  function decodeBlob(value) {
    try {
      var alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
      var encoded = String(value).replace(/=+$/, "")
      var buffer = 0, bits = 0, escaped = ""
      for (var i = 0; i < encoded.length; i++) {
        var digit = alphabet.indexOf(encoded[i])
        if (digit < 0) throw new Error("Invalid content encoding")
        buffer = (buffer << 6) | digit
        bits += 6
        if (bits >= 8) {
          bits -= 8
          escaped += "%" + ("0" + ((buffer >> bits) & 255).toString(16)).slice(-2)
        }
      }
      return decodeURIComponent(escaped)
    } catch (e) { return "Couldn’t preview this content." }
  }
  function beginSetup(mode, args, source) {
    if (busy) return
    setupMode = mode
    setupArguments = args
    setupFailed = false
    actionError = ""
    actionMessage = mode === "setup" ? "Preparing Relai and starting sync…" : "Updating Relai…"
    actionInFlight = true
    feedbackDeadline.stop()
    actionStartDeadline.restart()
    setupProc.command = [root.setupLauncher, mode].concat(source ? ["--source"] : []).concat(args)
    setupProc.running = true
  }
  function open() { controller.show(); refresh() }
  function prettyTime(value) {
    var date = new Date(value)
    return value && !isNaN(date.getTime()) ? date.toLocaleString(Qt.locale(), "MMM d, HH:mm") : "Not yet"
  }
  function relativeTime(value) {
    var elapsed = now - new Date(value).getTime()
    if (!value || isNaN(elapsed)) return "Not yet"
    if (elapsed < 60000) return "Just now"
    if (elapsed < 3600000) return Math.floor(elapsed / 60000) + " min ago"
    if (elapsed < 86400000) return Math.floor(elapsed / 3600000) + " hr ago"
    return prettyTime(value)
  }
  function formatBytes(bytes) {
    return bytes < 1024 ? (bytes || 0) + " B" : bytes < 1048576 ? Math.round(bytes / 1024) + " KiB" : (bytes / 1048576).toFixed(1) + " MiB"
  }
  function countLabel(count, singular) { return (count || 0) + " " + singular + (count === 1 ? "" : "s") }
  Timer { interval: 30000; running: root.opened; repeat: true; onTriggered: root.now = Date.now() }
  Timer { id: feedbackDeadline; interval: 6000; onTriggered: if (!root.busy && !root.actionError) root.actionMessage = "" }
  Timer {
    id: readoutDeadline; interval: 15000
    onTriggered: {
      root.readoutTimedOut = true
      root.readoutPending = false
      root.readoutError = "This is taking longer than expected. Check again to retry."
      if (readoutProc.running) readoutProc.signal(9)
    }
  }
  Process {
    id: readoutProc
    stdout: StdioCollector { id: readoutOutput }
    stderr: StdioCollector { id: readoutErrors }
    onExited: function(code) {
      readoutDeadline.stop()
      if (root.readoutQueued) { root.readoutQueued = false; Qt.callLater(root.loadReadout); return }
      root.readoutPending = false
      root.acceptReadout(code, readoutOutput.text, readoutErrors.text)
    }
  }

  Timer {
    interval: Math.min(60, Math.max(2, Number(root.setting("refreshSeconds", 3)) || 3)) * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }
  Timer {
    id: actionStartDeadline
    interval: 5000
    onTriggered: if (root.actionInFlight && !actionProc.running && !setupProc.running && !terminalProc.running) root.finishAction(1, "", "Could not launch the action. Check the Relai installation.")
  }
  Timer {
    id: statusDeadline
    interval: 15000
    onTriggered: {
      if (root.statusPending) {
        root.statusPending = false
        root.statusError = "Status command did not respond. Try checking again, or open Advanced → Check sync issues."
        if (statusProc.running) statusProc.signal(9)
      }
    }
  }
  FileView {
    id: versionFile
    onLoadFailed: root.statusError = "Could not read plugin manifest. Reinstall the plugin."
    path: decodeURIComponent(Qt.resolvedUrl("../manifest.json").toString().replace(/^file:\/\//, ""))
    onLoaded: {
      try { root.pluginVersion = JSON.parse(text()).version }
      catch (e) { root.statusError = "Could not read plugin version. Reinstall the plugin." }
    }
  }
  Process {
    id: statusProc
    command: [root.launcher, "status", "--json"]
    stdout: StdioCollector { id: statusOutput }
    stderr: StdioCollector {}
    onExited: function(code) {
      root.statusPending = false
      statusDeadline.stop()
      if (code === 0) root.acceptStatus(statusOutput.text)
      else root.statusError = "Couldn’t check sync status. Check again to retry."
    }
  }
  Process {
    id: actionProc
    onStarted: actionStartDeadline.stop()
    stdout: StdioCollector { id: actionOutput }
    stderr: StdioCollector { id: actionErrors }
    onExited: function(code) { root.finishAction(code, actionOutput.text, actionErrors.text) }
  }
  Process {
    id: setupProc
    onStarted: actionStartDeadline.stop()
    stdout: SplitParser {
      onRead: function(line) {
        if (/^(Preparing Relai|Downloading Relai|Building Relai|Installed Relai|Saving configuration)/.test(line)) root.actionMessage = line
      }
    }
    stderr: StdioCollector { id: setupErrors }
    onExited: function(code) {
      root.actionInFlight = false
      actionStartDeadline.stop()
      root.setupFailed = code !== 0
      root.actionError = code === 0 ? "" : (setupErrors.text.trim().slice(-3000) || "Setup could not finish. Try again.")
      root.actionMessage = code === 0 ? (root.setupMode === "setup" ? "Setup complete. Sync is running." : "Relai is up to date.") : ""
      if (code === 0) { root.setupOpen = false; feedbackDeadline.restart(); keyCatcher.forceActiveFocus() }
      else Qt.callLater(function() { root.revealItem(actionFeedback) })
      root.refresh()
    }
  }

  Process {
    id: terminalProc
    onStarted: actionStartDeadline.stop()
    stderr: StdioCollector { id: terminalErrors }
    onExited: function(code) {
      root.actionInFlight = false
      actionStartDeadline.stop()
      root.actionMessage = ""
      root.actionError = code === 0 ? "" : terminalErrors.text.trim().slice(-1000) || "Couldn’t open the full comparison. Try again."
    }
  }

  component RelaiButton: ActionButton {
    foreground: root.foreground
    property var entry: null
    property int actionIndex: -1
    text: entry ? entry.label : ""
    hint: entry && entry.hint ? entry.hint : ""
    enabled: entry ? root.enabledAction(entry) : true
    primary: actionIndex === root.conflictActionCount && root.dashboard
    destructive: !!entry && !!entry.destructive
    highlighted: root.keyboardNavigation && actionIndex === root.selectedAction
    onClicked: if (entry) { root.keyboardNavigation = false; root.selectedAction = actionIndex; root.runAction(entry) }
    Component.onCompleted: if (entry) root.actionItems[root.actionKey(entry)] = this
    onEntryChanged: if (entry) root.actionItems[root.actionKey(entry)] = this
  }
  component Label: Text {
    textFormat: Text.PlainText
    color: root.secondary
    font.family: Style.font.family
    font.pixelSize: Style.space(12)
    wrapMode: Text.WordWrap
  }
  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    tooltipText: "Relai · " + root.healthLabel
    iconComponent: Component {
      Item {
        Text {
          anchors.centerIn: parent
          text: "✦"
          font.family: Style.font.family
          font.pixelSize: Style.space(17)
          color: root.healthColor
        }
        Rectangle {
          width: Style.space(4); height: width; radius: width / 2
          anchors.right: parent.right; anchors.bottom: parent.bottom
          color: root.healthColor
          visible: root.report.health !== "healthy" || !root.report.daemon
        }
      }
    }
    onPressed: function(code) {
      if (code === Qt.MiddleButton) root.runAction({args: ["sync"]})
      else root.toggle()
    }
  }

  KeyboardPanel {
    id: popup
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: root.setupOpen ? setupForm.firstField : keyCatcher
    contentWidth: popup.fittedContentWidth(Style.space(440))
    contentHeight: popup.fittedContentHeight(content.implicitHeight, Style.space(650))

    PanelKeyCatcher {
      id: keyCatcher
      objectName: "relai-keyboard"
      blocked: root.setupOpen
      anchors.fill: parent
      onCloseRequested: root.escapeView()
      onMoveRequested: function(dx, dy) { root.moveSelection(dy || dx) }
      onActivateRequested: root.runAction(root.actions[root.selectedAction])
      onTabRequested: function(direction) { root.moveSelection(direction) }
      onTextKey: function(key) {
        if (key === "r") { if (root.readoutOpen) root.loadReadout(); else root.refresh() }
        if (key === "s" && root.dashboard && root.report.configured && !root.report.paused) root.runAction({args: ["sync"]})
      }

      Flickable {
        id: scroll
        objectName: "relai-scroll"
        anchors.fill: parent
        contentWidth: width
        contentHeight: content.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        Controls.ScrollBar.vertical: Controls.ScrollBar {}

        Column {
          id: content
          objectName: "relai-panel-content"
          width: scroll.width
          spacing: Style.space(14)

          RowLayout {
            width: parent.width
            Label { text: "✦  Relai"; color: root.foreground; font.pixelSize: Style.space(21); Layout.fillWidth: true }
            Label { text: "AI CONFIG SYNC"; font.pixelSize: Style.space(9); font.letterSpacing: 1 }
          }

          Rectangle {
            width: parent.width
            implicitHeight: healthContent.implicitHeight + Style.space(26)
            radius: Style.space(8)
            color: Util.alpha(root.healthColor, 0.07)
            border.color: Util.alpha(root.healthColor, 0.16)
            Column {
              id: healthContent
              anchors { left: parent.left; right: parent.right; top: parent.top; margins: Style.space(13) }
              spacing: Style.space(6)
              Label { width: parent.width; text: root.healthLabel; color: root.healthColor; font.pixelSize: Style.space(16) }
              Label { width: parent.width; text: root.healthDescription; font.pixelSize: Style.space(11) }
              Label {
                visible: root.report.configured && root.statusLoaded && !root.statusError
                width: parent.width
                text: (root.report.remote_configured ? "Last synced · " + root.relativeTime(root.report.last_sync) : "Last applied · " + root.relativeTime(root.report.last_apply))
                font.pixelSize: Style.space(10)
              }
            }
          }

          Rectangle {
            id: actionFeedback
            objectName: "relai-feedback"
            width: parent.width
            visible: root.actionMessage !== "" || root.actionError !== ""
            implicitHeight: feedbackRow.implicitHeight + Style.space(18)
            radius: Style.space(6)
            color: Util.alpha(root.actionError ? Color.urgent : Color.accent, 0.07)
            RowLayout {
              id: feedbackRow
              anchors { top: parent.top; left: parent.left; right: parent.right; margins: Style.space(9) }
              spacing: Style.space(9)
              Controls.BusyIndicator { running: root.busy; visible: running; implicitWidth: Style.space(20); implicitHeight: width }
              Label { Layout.fillWidth: true; text: root.actionError || root.actionMessage; color: root.actionError ? Color.urgent : root.foreground; font.pixelSize: Style.space(11); Accessible.role: Accessible.StaticText; Accessible.name: text }
            }
          }

          SetupForm {
            id: setupForm
            width: parent.width
            visible: root.setupOpen
            working: root.busy
            foreground: root.foreground
            secondary: root.secondary
            onFocusMoved: function(item) { Qt.callLater(function() { root.revealItem(item) }) }
            onSubmitted: function(remote, machine, seed) { root.beginSetup("setup", ["--remote", remote, "--machine", machine, "--seed", seed], false) }
            onCancelled: { root.setupOpen = false; keyCatcher.forceActiveFocus() }
          }

          Column {
            width: parent.width
            visible: !!root.pendingConfirmation
            spacing: Style.space(12)
            Label { width: parent.width; text: root.pendingConfirmation ? root.pendingConfirmation.title : ""; color: root.foreground; font.pixelSize: Style.space(16) }
            Label { width: parent.width; text: root.pendingConfirmation ? root.pendingConfirmation.description : "" }
          }

          Column {
            width: parent.width
            visible: root.readoutOpen
            spacing: Style.space(10)
            Label { width: parent.width; text: root.readoutTitle; color: root.foreground; font.pixelSize: Style.space(16) }
            Label { width: parent.width; visible: root.readoutPending; text: "Loading details…" }
            Label { width: parent.width; visible: root.readoutError !== ""; text: root.readoutError; color: Color.urgent }
            Label { width: parent.width; visible: root.readoutSummary !== ""; text: root.readoutSummary }
            Repeater {
              model: root.readoutEntries
              delegate: Column {
                required property var modelData
                width: content.width
                spacing: Style.space(5)
                Label { width: parent.width; text: modelData.title; color: root.foreground; font.pixelSize: Style.space(12) }
                Controls.TextArea {
                  width: parent.width
                  text: modelData.body
                  textFormat: TextEdit.PlainText
                  readOnly: true
                  selectByMouse: true
                  wrapMode: TextEdit.WrapAnywhere
                  color: root.secondary
                  font.family: Style.font.family
                  font.pixelSize: Style.space(11)
                  padding: Style.space(8)
                  background: Rectangle { color: Util.alpha(root.foreground, 0.035); radius: Style.space(5) }
                }
              }
            }
          }

          RowLayout {
            width: parent.width
            visible: root.viewActions.length > 0
            Repeater {
              model: root.viewActions
              delegate: RelaiButton {
                required property var modelData
                required property int index
                objectName: index === 1 && root.clearConfirm ? "relai-clear-confirm" : "relai-view-" + index
                entry: modelData
                actionIndex: index
                primary: index === 0
                Layout.fillWidth: true
              }
            }
          }

          Column {
            width: parent.width
            spacing: Style.space(12)
            visible: root.dashboard && (root.report.conflicts || []).length > 0
            Repeater {
              model: root.report.conflicts || []
              delegate: Column {
                id: conflictItem
                required property var modelData
                required property int index
                readonly property int actionOffset: root.conflictOffset(index)
                width: content.width
                spacing: Style.space(7)
                Label { width: parent.width; text: modelData.key; wrapMode: Text.WrapAnywhere; color: root.foreground }
                Label { width: parent.width; text: modelData.reason; font.pixelSize: Style.space(11) }
                RelaiButton { entry: root.actions[conflictItem.actionOffset]; actionIndex: conflictItem.actionOffset }
                Flow {
                  width: parent.width
                  spacing: Style.space(6)
                  Repeater {
                    model: conflictItem.modelData.choices
                    delegate: RelaiButton {
                      required property string modelData
                      required property int index
                      actionIndex: conflictItem.actionOffset + 1 + index
                      entry: root.actions[actionIndex]
                    }
                  }
                }
              }
            }
          }

          GridLayout {
            visible: root.dashboard
            width: parent.width
            columns: 2
            columnSpacing: Style.space(8)
            rowSpacing: Style.space(8)
            Repeater {
              model: root.baseActions
              delegate: RelaiButton {
                required property var modelData
                required property int index
                actionIndex: root.conflictActionCount + index
                entry: modelData
                objectName: "relai-action-" + modelData.args[0]
                text: index === 0 && root.busy ? "Working…" : modelData.label
                Layout.fillWidth: true
              }
            }
          }

          Column {
            width: parent.width
            spacing: Style.space(11)
            visible: root.report.configured && root.statusLoaded && root.dashboard
            Label { text: "YOUR PROVIDERS"; font.pixelSize: Style.space(10); font.letterSpacing: 1 }
            Repeater {
              model: root.report.providers || []
              delegate: Column {
                required property var modelData
                width: content.width
                spacing: Style.space(4)
                RowLayout {
                  width: parent.width
                  Label { text: modelData.name === "claude" ? "Claude Code" : modelData.name === "codex" ? "Codex" : "OpenCode"; color: root.foreground; font.pixelSize: Style.space(13); Layout.fillWidth: true }
                  Label { text: !modelData.detected ? "Not found" : !modelData.managed ? "Nothing to sync yet" : "Included"; font.pixelSize: Style.space(11) }
                }
                Label {
                  width: parent.width
                  text: root.countLabel(modelData.instructions, "instruction") + " · " + root.countLabel(modelData.settings, "setting") + " · " + root.countLabel(modelData.skills, "skill") + " · " + root.countLabel(modelData.mcp, "MCP")
                  visible: modelData.detected
                  font.pixelSize: Style.space(10)
                }
              }
            }
          }

          Column {
            id: syncDetails
            width: parent.width
            visible: root.detailsOpen && root.dashboard
            spacing: Style.space(8)
            Label { text: "SYNC DETAILS"; font.pixelSize: Style.space(10); font.letterSpacing: 1 }
            Label { width: parent.width; text: "Each provider keeps its own configuration. Counts show directly managed items; hooks and rules are included too. A zero means no files or declarations were found in that category."; font.pixelSize: Style.space(11) }
            Repeater {
              model: root.report.machines || []
              delegate: Label {
                required property var modelData
                width: content.width
                text: modelData.name + (modelData.local ? " · this computer" : " · saved label, not live presence")
              }
            }
            Label { width: parent.width; text: "Last remote sync · " + (root.report.remote_configured ? root.prettyTime(root.report.last_sync) : "No remote connected"); font.pixelSize: Style.space(11) }
            Label { width: parent.width; text: "Last local apply · " + root.prettyTime(root.report.last_apply); font.pixelSize: Style.space(11) }
            Label { width: parent.width; text: root.countLabel(root.report.backup_count, "recovery point") + " · " + root.formatBytes(root.report.backup_bytes); font.pixelSize: Style.space(11) }
            Label { width: parent.width; visible: (root.report.warnings || []).length > 0; text: "Kept local"; color: root.foreground }
            Repeater {
              model: root.report.warnings || []
              delegate: Label { required property string modelData; width: content.width; text: modelData; font.pixelSize: Style.space(11) }
            }
          }

          Column {
            id: advancedSection
            width: parent.width
            visible: root.advancedOpen && root.dashboard
            spacing: Style.space(8)
            Label { text: "ADVANCED"; font.pixelSize: Style.space(10); font.letterSpacing: 1 }
            Label { width: parent.width; text: "Checks and recovery stay in this panel. Undo and reset ask before making changes."; font.pixelSize: Style.space(11) }
            GridLayout {
              width: parent.width
              columns: 2
              columnSpacing: Style.space(8)
              rowSpacing: Style.space(8)
              Repeater {
                model: root.advancedActions
                delegate: RelaiButton {
                  required property var modelData
                  required property int index
                  actionIndex: root.conflictActionCount + root.baseActions.length + index
                  entry: modelData
                  objectName: "relai-" + modelData.args[0]
                  Layout.fillWidth: true
                }
              }
            }
          }

          Column {
            width: parent.width
            visible: root.setupFailed
            spacing: Style.space(8)
            Label { width: parent.width; text: "Your choices are saved. Try again, or build this local copy if a release is unavailable."; font.pixelSize: Style.space(11) }
            ActionButton {
              objectName: "relai-setup-source"
              visible: root.setupOpen
              onActiveFocusChanged: if (activeFocus) root.revealItem(this)
              text: "Build local copy and retry"
              foreground: root.foreground
              hint: "Requires mise and the pinned Go compiler."
              enabled: !root.busy
              onClicked: root.beginSetup(root.setupMode, root.setupArguments, true)
            }
          }
          Label {
            width: parent.width
            visible: root.busy
            text: "You can close this panel. The current operation will continue."
            font.pixelSize: Style.space(10)
          }
        }
      }
    }
  }
}
