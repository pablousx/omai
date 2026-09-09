import QtQuick
import QtQuick.Controls as Controls
import qs.Commons

Controls.Button {
  id: control
  property color foreground: Color.foreground
  property bool primary: false
  property bool destructive: false
  property string hint: ""
  readonly property color tint: destructive ? Color.urgent : primary ? Color.accent : foreground
  implicitHeight: Math.max(Style.space(38), label.implicitHeight + Style.space(18))
  implicitWidth: Math.max(Style.space(80), label.implicitWidth + Style.space(24))
  hoverEnabled: true
  opacity: enabled ? 1 : 0.45
  Accessible.name: text
  Accessible.description: hint
  Controls.ToolTip.visible: hovered && hint !== ""
  Controls.ToolTip.text: hint
  Controls.ToolTip.delay: 700
  contentItem: Text {
    id: label
    text: control.text
    textFormat: Text.PlainText
    color: control.tint
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
    font.family: Style.font.family
    font.pixelSize: Style.space(12)
    wrapMode: Text.WordWrap
  }
  background: Rectangle {
    radius: Style.space(6)
    color: Util.alpha(control.tint, control.down ? 0.22 : control.hovered ? 0.14 : control.primary ? 0.10 : 0.045)
    border.color: control.highlighted || control.activeFocus ? Color.accent : Util.alpha(control.tint, control.primary ? 0.45 : 0.15)
    Behavior on color { ColorAnimation { duration: 100 } }
  }
  HoverHandler { cursorShape: control.enabled ? Qt.PointingHandCursor : Qt.ArrowCursor }
}
