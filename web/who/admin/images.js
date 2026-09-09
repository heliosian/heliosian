import {state} from './state.js';
import {load} from './app.js';

export function renderImages(selector, items, kind, colors) {
  const wrap = document.querySelector(selector);
  wrap.replaceChildren();
  for (const item of items) {
    const row = document.createElement('div');
    row.className = 'image-row';

    if (item.imageUrl) {
      const img = document.createElement('img');
      img.src = item.imageUrl;
      img.alt = '';
      row.append(img);
    } else {
      row.append(document.createElement('div')).className = 'placeholder';
    }

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = item.name;
    row.append(name);

    const status = document.createElement('span');
    status.className = 'status';
    row.append(status);

    const colorInput = document.createElement('input');
    colorInput.type = 'color';
    colorInput.className = 'color-swatch';
    colorInput.title = `Hover color for ${item.name}`;
    colorInput.value = colors[item.name] || '#8a939b';
    colorInput.addEventListener('change', () => setColor(kind, item.name, colorInput, status));
    row.append(colorInput);

    const label = document.createElement('label');
    label.className = 'upload-button';
    label.textContent = 'Replace';
    if (!state.hasStore) {
      label.style.opacity = '0.5';
      label.style.cursor = 'default';
    }
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.disabled = !state.hasStore;
    input.addEventListener('change', () => uploadImage(kind, item.name, input, status));
    label.append(input);
    row.append(label);

    wrap.append(row);
  }
}

async function uploadImage(kind, name, input, status) {
  if (!input.files.length) {
    return;
  }
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  const form = new FormData();
  form.append('kind', kind);
  form.append('name', name);
  form.append('file', input.files[0]);
  const res = await fetch('/api/admin/images', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return;
  }
  status.textContent = '';
  await load();
}

export async function setColor(kind, name, input, status) {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  const res = await fetch('/api/config/color', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({kind, name, color: input.value}),
  });
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return;
  }
  status.textContent = '';
}
