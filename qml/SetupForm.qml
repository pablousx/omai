pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import qs.Commons

Column {
  id: form
  required property color foreground
  required property color secondary
  property alias firstField: remoteInput
  property bool working: false
  property bool attempted: false
  readonly property string remoteError: {
    var value = remoteInput.text.trim()
    if (!value) return ""
    if (/\s/.test(value)) return "Use a repository URL without spaces."
    if (/^https?:\/\/[^/]*@/.test(value)) return "Remove credentials from the URL. omai uses your saved Git sign-in."
    if (!/^(https?:\/\/|ssh:\/\/|git:\/\/|file:\/\/|\/|[^/:\s]+@[^/:\s]+:)/.test(value)) return "Use an HTTPS or SSH Git URL, or leave this blank for local sync."
    return ""
  }
  function submit() {
    if (working) return
    attempted = true
    if (remoteError) { remoteInput.forceActiveFocus(); return }
    submitted(remoteInput.text.trim(), machineInput.text.trim(), ["", "codex", "claude", "opencode"][seedInput.currentIndex])
  }
  signal submitted(string remote, string machine, string seed)
  signal cancelled()
  signal focusMoved(var item)
  spacing: Style.space(9)
  function focusFirst() { remoteInput.forceActiveFocus() }
  function clearDraft() { remoteInput.text = ""; machineInput.text = ""; seedInput.currentIndex = 0; attempted = false }
  Keys.onEscapePressed: cancelled()

  component Label: Text {
    width: form.width
    color: form.secondary
    font.family: Style.font.family
    font.pixelSize: Style.space(12)
    wrapMode: Text.WordWrap
  }
  component Field: Controls.TextField {
    width: form.width
    enabled: !form.working
    implicitHeight: Style.space(38)
    color: form.foreground
    placeholderTextColor: form.secondary
    selectByMouse: true
    font.family: Style.font.family
    font.pixelSize: Style.space(12)
    onActiveFocusChanged: if (activeFocus) form.focusMoved(this)
    background: Rectangle {
      color: Util.alpha(form.foreground, 0.05)
      radius: Style.space(5)
      border.color: parent.activeFocus ? Color.accent : Util.alpha(form.foreground, 0.2)
    }
  }
  component Button: ActionButton {
    foreground: form.foreground
    onActiveFocusChanged: if (activeFocus) form.focusMoved(this)
  }

  Label { text: "Connect your setup"; color: form.foreground; font.pixelSize: Style.space(16) }
  Label { text: "Git remote · optional" }
  Field {
    id: remoteInput
    objectName: "omai-setup-remote"
    Accessible.name: "Git remote, optional"
    placeholderText: "git@github.com:you/ai-config.git"
    onAccepted: machineInput.forceActiveFocus()
  }
  Label { visible: form.attempted && form.remoteError !== ""; text: form.remoteError; color: Color.urgent }
  Label { text: "Use an existing Git repository to sync across computers. Leave this blank to keep changes local."; font.pixelSize: Style.space(11) }
  Label { text: "Computer name · optional" }
  Field { id: machineInput; objectName: "omai-setup-machine"; Accessible.name: "Computer name, optional"; placeholderText: "This computer"; onAccepted: form.submit() }
  Label { text: "Import existing configuration" }
  Controls.ComboBox {
    id: seedInput
    objectName: "omai-setup-seed"
    Accessible.name: "Import existing configuration"
    enabled: !form.working
    width: form.width
    implicitHeight: Style.space(36)
    model: ["All detected providers (recommended)", "Start with Codex", "Start with Claude Code", "Start with OpenCode"]
    font.family: Style.font.family
    font.pixelSize: Style.space(12)
    contentItem: Text {
      text: seedInput.displayText
      color: form.foreground
      font: seedInput.font
      verticalAlignment: Text.AlignVCenter
      elide: Text.ElideRight
      rightPadding: Style.space(18)
    }
    indicator: Text {
      text: "▾"
      color: form.foreground
      anchors.right: parent.right
      anchors.rightMargin: Style.space(9)
      anchors.verticalCenter: parent.verticalCenter
    }
    background: Rectangle {
      color: Util.alpha(form.foreground, 0.05)
      radius: Style.space(5)
      border.color: seedInput.activeFocus ? Color.accent : Util.alpha(form.foreground, 0.2)
    }
    delegate: Controls.ItemDelegate {
      required property int index
      required property string modelData
      width: seedInput.width
      highlighted: seedInput.highlightedIndex === index
      contentItem: Text { text: parent.modelData; color: form.foreground; font: seedInput.font }
      background: Rectangle { color: parent.highlighted ? Util.alpha(Color.accent, 0.2) : "transparent" }
    }
    popup: Controls.Popup {
      y: seedInput.height
      width: seedInput.width
      padding: Style.space(4)
      contentItem: ListView {
        implicitHeight: contentHeight
        model: seedInput.popup.visible ? seedInput.delegateModel : null
        currentIndex: seedInput.highlightedIndex
        clip: true
      }
      background: Rectangle {
        color: Color.popups.background
        border.color: Util.alpha(form.foreground, 0.2)
        radius: Style.space(5)
      }
    }
    onActiveFocusChanged: if (activeFocus) form.focusMoved(this)
  }
  Label {
    text: "Each provider keeps its own instructions, settings, skills, and MCPs. Credentials stay local. If versions differ, you choose what to keep."
    font.pixelSize: Style.space(11)
  }
  Row {
    spacing: Style.space(8)
    Button {
      objectName: "omai-setup-submit"
      text: form.working ? "Setting up…" : remoteInput.text.trim() ? "Start syncing" : "Start local sync"
      primary: true
      enabled: !form.working
      onClicked: form.submit()
    }
    Button { objectName: "omai-setup-cancel"; text: form.working ? "Close" : "Cancel"; onClicked: form.cancelled() }
  }
}
