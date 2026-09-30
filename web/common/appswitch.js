import {query} from '/data.js';
import {noteError, openFeedback} from '/feedback.js';

function tierLabels() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels.slice(1) : labels;
}

export function currentApp() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels[0] : 'home';
}

export function appOrigin(key) {
  const tier = tierLabels();
  const host = key === 'home' ? tier : [key, ...tier];
  return location.protocol + '//' + host.join('.') + (location.port ? ':' + location.port : '');
}

export function whoLink(email) {
  return appOrigin('who') + '/people/' + encodeURIComponent((email || '').split('@')[0]);
}

async function switchList() {
  const read = await query('/api/apps');
  return read.result.map(read.get);
}

const hoverGrace = 150;

export function hoverMenu(button, menu, open, close) {
  let timer;
  const enter = e => {
    if (e.pointerType !== 'mouse') {
      return;
    }
    clearTimeout(timer);
    open();
  };
  const leave = e => {
    if (e.pointerType !== 'mouse') {
      return;
    }
    clearTimeout(timer);
    timer = setTimeout(close, hoverGrace);
  };
  for (const node of [button, menu]) {
    node.addEventListener('pointerenter', enter);
    node.addEventListener('pointerleave', leave);
  }
}

export function hoverClick(e) {
  if (matchMedia('(hover: none)').matches) {
    return false;
  }
  return !e.pointerType || e.pointerType === 'mouse';
}

export function closeBarMenus() {
  for (const menu of document.querySelectorAll('.user-menu, .app-switch-menu')) {
    menu.hidden = true;
  }
}

function closeAppSwitches() {
  for (const menu of document.querySelectorAll('.app-switch-menu')) {
    menu.hidden = true;
  }
}

export function initAppSwitch() {
  const current = currentApp();
  const wraps = document.querySelectorAll('.app-switch');
  switchList().then(apps => {
    for (const wrap of wraps) {
      const menu = wrap.querySelector('.app-switch-menu');
      for (const app of apps) {
        if (app.me.listed === false && app.key !== current) {
          continue;
        }
        menu.append(appRow(app, app.key === current));
      }
      menu.append(menuFoot());
    }
  }).catch(err => noteError('/api/apps: ' + err.message));
  for (const wrap of wraps) {
    const button = wrap.querySelector('.app-switch-button');
    const menu = wrap.querySelector('.app-switch-menu');
    button.href = appOrigin('home');
    const open = () => {
      closeBarMenus();
      menu.hidden = false;
    };
    hoverMenu(button, menu, open, closeAppSwitches);
    button.addEventListener('click', e => {
      if (hoverClick(e)) {
        return;
      }
      e.preventDefault();
      const opening = menu.hidden;
      closeAppSwitches();
      menu.hidden = !opening;
    });
  }
  document.addEventListener('click', e => {
    if (!e.target.closest('.app-switch')) {
      closeAppSwitches();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeAppSwitches();
      for (const menu of document.querySelectorAll('.topbar-alert-menu')) {
        menu.hidden = true;
      }
    }
  });
}

function appRow(app, isCurrent) {
  const row = document.createElement('a');
  row.href = appOrigin(app.key);
  row.className = isCurrent ? 'is-current' : '';
  const icon = document.createElement('img');
  icon.src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  icon.alt = '';
  const text = document.createElement('span');
  const name = document.createElement('span');
  name.className = 'app-switch-name';
  name.textContent = app.name;
  const tagline = document.createElement('span');
  tagline.className = 'app-switch-tagline';
  tagline.textContent = app.tagline;
  text.append(name, tagline);
  row.append(icon, text);
  return row;
}

function menuFoot() {
  const foot = document.createElement('div');
  foot.className = 'app-switch-foot';
  foot.append(feedbackLine(), repoLine());
  return foot;
}

function feedbackLine() {
  const line = document.createElement('button');
  line.type = 'button';
  line.className = 'app-switch-feedback';
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  mark.setAttribute('viewBox', '0 0 24 24');
  mark.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z');
  mark.append(path);
  line.append(mark, document.createTextNode('Report a problem or idea'));
  line.addEventListener('click', () => {
    closeAppSwitches();
    openFeedback();
  });
  return line;
}

function repoLine() {
  const line = document.createElement('a');
  line.className = 'app-switch-repo';
  line.href = 'https://github.com/heliosian/heliosian';
  line.target = '_blank';
  line.rel = 'noopener';
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  mark.setAttribute('viewBox', '0 0 24 24');
  mark.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', 'M12 .5C5.37.5 0 5.87 0 12.5c0 5.3 3.44 9.8 8.21 11.39.6.11.82-.26.82-.58 0-.29-.01-1.05-.02-2.06-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.09-.75.08-.73.08-.73 1.21.09 1.84 1.24 1.84 1.24 1.07 1.84 2.81 1.31 3.5 1 .11-.78.42-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.12-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.29-1.55 3.3-1.23 3.3-1.23.66 1.66.24 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.81 5.62-5.49 5.92.43.37.81 1.1.81 2.22 0 1.6-.01 2.9-.01 3.29 0 .32.22.7.83.58A12.01 12.01 0 0 0 24 12.5C24 5.87 18.63.5 12 .5z');
  mark.append(path);
  line.append(mark, document.createTextNode('Built for the community, by the community'));
  return line;
}
