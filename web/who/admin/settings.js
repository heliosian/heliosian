import {load} from './app.js';
import {setColor} from './images.js';

export function initSettings() {
  document.querySelector('#staff-color').addEventListener('change', e => {
    setColor('staff', '', e.target, document.querySelector('#staff-color-status'));
  });

  document.querySelector('#save-years').addEventListener('click', async () => {
    const status = document.querySelector('#years-status');
    const button = document.querySelector('#save-years');
    const years = {
      photo: Number(document.querySelector('#years-photo').value),
      facts: Number(document.querySelector('#years-facts').value),
      familyPhoto: Number(document.querySelector('#years-family-photo').value),
    };
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Saving…';
    const res = await fetch('/api/config/stale-years', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(years),
    });
    button.disabled = false;
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.classList.add('ok');
    status.textContent = 'Saved.';
    await load();
  });

  document.querySelector('#save-privacy-links').addEventListener('click', async () => {
    const status = document.querySelector('#privacy-links-status');
    const button = document.querySelector('#save-privacy-links');
    const links = {
      veracrossPreferences: document.querySelector('#privacy-veracross-url').value.trim(),
      heliosWhoOptIn: document.querySelector('#privacy-optin-url').value.trim(),
    };
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Saving…';
    const res = await fetch('/api/config/privacy-links', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(links),
    });
    button.disabled = false;
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.classList.add('ok');
    status.textContent = 'Saved.';
    await load();
  });
}
