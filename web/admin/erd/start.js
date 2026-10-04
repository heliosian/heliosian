import {el} from '/elements.js';
import {chrome, tables} from '/chrome.js';

chrome('erd');

const sheetColors = {
  datapeople: '#f4a261',
  datagroups: '#8ecae6',
  datadocuments: '#90be6d',
  datamail: '#c77dff',
  dataconfig: '#ffd166',
  generated: '#3fd0b4',
  '*': '#ef476f',
};

const sheetNames = {'*': 'every sheet'};

function diagram(list) {
  const lines = ['classDiagram'];
  const links = [];
  const styles = [];
  for (const t of [...list].sort((a, b) => a.name.localeCompare(b.name))) {
    lines.push(`  class ${t.name} {`);
    for (const c of t.columns) {
      if (c.relation) {
        links.push(`  ${t.name} --> ${c.relation} : ${c.name}`);
        continue;
      }
      lines.push(`    ${c.kind} ${c.name}`);
    }
    lines.push('  }');
    styles.push(`  style ${t.name} stroke:${sheetColors[t.sheet]},stroke-width:2px`);
  }
  return [...lines, ...links, ...styles].join('\n');
}

function clickable(root, names) {
  for (const node of root.querySelectorAll('g[id*="classId-"]')) {
    const name = names.find(n => new RegExp(`classId-${n}-\\d+$`).test(node.id));
    if (!name) {
      continue;
    }
    node.classList.add('table');
    node.addEventListener('click', () => {
      location.href = `/resources#${name}`;
    });
  }
}

const root = document.getElementById('erd');
const legend = document.getElementById('legend');
for (const [sheet, color] of Object.entries(sheetColors)) {
  const item = el('span', 'item', sheetNames[sheet] ?? sheet.replace(/^data/, ''));
  item.style.setProperty('--k', color);
  legend.append(item);
}
try {
  const list = await tables();
  window.mermaid.initialize({
    startOnLoad: false,
    maxTextSize: 1000000,
    theme: 'base',
    themeVariables: {
      darkMode: true,
      background: '#0e1719',
      mainBkg: '#142124',
      primaryColor: '#142124',
      primaryBorderColor: '#24383c',
      primaryTextColor: '#d6e4e1',
      secondaryColor: '#1a2b2f',
      tertiaryColor: '#1a2b2f',
      lineColor: '#4f6763',
      textColor: '#d6e4e1',
      fontFamily: '"JetBrains Mono", "SF Mono", ui-monospace, Menlo, Consolas, monospace',
      fontSize: '12px',
    },
  });
  const {svg} = await window.mermaid.render('erd-svg', diagram(list));
  root.innerHTML = svg;
  clickable(root, list.map(t => t.name));
} catch (err) {
  root.replaceChildren(el('p', 'error', err.message));
}
