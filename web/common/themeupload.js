import {el, svg} from '/elements.js';
import {openModal} from '/modal.js';
import {api} from '/api.js';
import {refreshThemes} from '/themes.js';

export async function postTheme(files) {
  const form = new FormData();
  for (const f of files) {
    form.append('file', f);
  }
  const {result} = await api('POST', '/api/do/theme', form);
  await refreshThemes([result[0]]);
  return result[0];
}

async function entryFiles(entry) {
  if (entry.isFile) {
    return [await new Promise((ok, fail) => entry.file(ok, fail))];
  }
  const reader = entry.createReader();
  const out = [];
  for (;;) {
    const batch = await new Promise((ok, fail) => reader.readEntries(ok, fail));
    if (!batch.length) {
      return out;
    }
    for (const e of batch) {
      out.push(...await entryFiles(e));
    }
  }
}

function filePicker(label, folder, add) {
  const input = el('input');
  input.type = 'file';
  input.multiple = true;
  input.webkitdirectory = folder;
  input.hidden = true;
  input.addEventListener('change', () => {
    add([...input.files]);
    input.value = '';
  });
  const pick = el('button', 'link-button', label);
  pick.type = 'button';
  pick.addEventListener('click', () => input.click());
  return [pick, input];
}

function themeGroups(files) {
  const groups = new Map();
  for (const f of files) {
    const folder = f.themeFolder || (f.webkitRelativePath ? f.webkitRelativePath.split('/')[0] : '');
    const key = /\.zip$/i.test(f.name) ? `zip:${f.name}:${f.size}` : folder ? `folder:${folder}` : 'loose';
    if (!groups.has(key)) {
      groups.set(key, {name: /\.zip$/i.test(f.name) ? f.name : folder || 'the loose files', files: []});
    }
    groups.get(key).files.push(f);
  }
  return [...groups.values()];
}

export function themeUpload(upload, onUploaded) {
  let files = [];
  const zone = el('div', 'theme-drop');
  const words = el('p', 'theme-drop-words', 'Drop your themes here: their folders, zips of them, or theme.json with its pictures. Each zip and each folder is a theme of its own.');
  const list = el('ul', 'theme-drop-list');
  const add = more => {
    files = [...files, ...more.filter(f => !f.name.startsWith('.') && !files.some(x => x.name === f.name && x.size === f.size))];
    list.replaceChildren(...files.map(f => el('li', '', f.name)));
    zone.classList.toggle('has-files', files.length > 0);
  };
  const clear = el('button', 'link-button', 'Start over');
  clear.type = 'button';
  clear.addEventListener('click', () => {
    files = [];
    add([]);
  });
  const picks = el('p', 'theme-drop-picks');
  picks.append('Or choose ', ...filePicker('files', false, add), ' or ', ...filePicker('a folder', true, add), '. ', clear);
  zone.append(svg('upload'), words, picks, list);
  zone.addEventListener('dragover', ev => {
    ev.preventDefault();
    zone.classList.add('is-over');
  });
  zone.addEventListener('dragleave', () => zone.classList.remove('is-over'));
  zone.addEventListener('drop', async ev => {
    ev.preventDefault();
    zone.classList.remove('is-over');
    const entries = [...ev.dataTransfer.items].map(i => i.webkitGetAsEntry()).filter(Boolean);
    const found = [];
    for (const entry of entries) {
      const inside = await entryFiles(entry);
      if (entry.isDirectory) {
        for (const f of inside) {
          f.themeFolder = entry.fullPath;
        }
      }
      found.push(...inside);
    }
    add(found);
  });
  const kit = el('p', 'theme-kit-note');
  const link = el('a', '', 'Download the theme kit');
  link.href = '/open/theme-kit.zip';
  kit.append(link, ': instructions, examples and a preview page that checks a theme before you upload it. Hand it to an AI to design one. Uploading a theme again with the same key replaces it.');
  openModal('Upload a theme', [kit, zone], {
    saveLabel: 'Upload',
    submit: async () => {
      if (!files.length) {
        throw new Error('Drop your theme in first.');
      }
      const done = [];
      const failed = [];
      for (const group of themeGroups(files)) {
        try {
          done.push(await upload(group.files));
          files = files.filter(f => !group.files.includes(f));
        } catch (err) {
          failed.push(`${group.name}: ${err.message}`);
        }
      }
      add([]);
      if (!failed.length) {
        return done[0];
      }
      if (done.length) {
        onUploaded(done[0]);
      }
      throw new Error(`${done.length ? `Uploaded ${done.length}; ` : ''}${failed.length === 1 ? 'this one didn’t upload' : 'these didn’t upload'} - ${failed.join(' · ')}`);
    },
    afterSave: onUploaded,
  });
}
