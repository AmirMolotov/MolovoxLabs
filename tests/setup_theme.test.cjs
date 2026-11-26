const assert = require('node:assert/strict');
const fs = require('node:fs');
const {test} = require('node:test');

test('the desktop shortcut checkbox leaves the installer buttons clickable', () => {
  const theme = fs.readFileSync('scripts/setup-theme.xml', 'utf8');
  const windowWidth = Number(theme.match(/<Window Width="(\d+)"/)[1]);
  const installPage = theme.match(/<Page Name="Install">([\s\S]*?)<\/Page>/)[1];

  function horizontalBounds(name) {
    const control = installPage.match(new RegExp(`<[^>]+Name="${name}"[^>]*>`))[0];
    const x = Number(control.match(/ X="(-?\d+)"/)[1]);
    const width = Number(control.match(/ Width="(-?\d+)"/)[1]);
    // WiX uses negative placement values to inset from the right edge.
    const left = x < 0 ? windowWidth + x - width : x;
    return [left, width < 0 ? windowWidth + width : left + width];
  }

  const [, shortcutRight] = horizontalBounds('DesktopShortcut');
  const [installLeft] = horizontalBounds('InstallButton');
  const [cancelLeft] = horizontalBounds('WelcomeCancelButton');
  assert.ok(shortcutRight < Math.min(installLeft, cancelLeft), 'the checkbox must end before the action buttons');
});
