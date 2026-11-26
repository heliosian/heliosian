import {api} from '/api.js';
import {copyGlyph} from '/datagrid.js';

const address = location.origin + '/';
const command = 'claude mcp add --transport http helios ' + address;
for (const [id, text] of [['address', address], ['command', command]]) {
  const code = document.getElementById(id);
  code.textContent = text;
  code.after(copyGlyph(text));
}

const create = document.getElementById('create');
create.addEventListener('click', async () => {
  create.disabled = true;
  try {
    const {token, email} = await api('POST', '/api/mcp/durable');
    document.getElementById('who').textContent = email;
    const code = document.getElementById('token');
    code.textContent = token;
    code.after(copyGlyph(token));
    document.getElementById('made').hidden = false;
    create.parentElement.hidden = true;
  } catch (err) {
    const error = document.getElementById('error');
    error.textContent = err.message;
    error.hidden = false;
    create.disabled = false;
  }
});
