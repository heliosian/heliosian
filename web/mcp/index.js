import {copyGlyph} from '/datagrid.js';

const address = location.origin + '/';
const command = 'claude mcp add --transport http helios ' + address;
for (const [id, text] of [['address', address], ['command', command]]) {
  const code = document.getElementById(id);
  code.textContent = text;
  code.after(copyGlyph(text));
}
