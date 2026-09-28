import {el, svg} from '/elements.js';

function flash(btn) {
  btn.classList.add('copied');
  btn.replaceChildren(svg('check'));
  setTimeout(() => {
    btn.classList.remove('copied');
    btn.replaceChildren(svg('copy'));
  }, 1200);
}

function copier(className, title, text) {
  const btn = el('button', className);
  btn.type = 'button';
  btn.title = title;
  btn.append(svg('copy'));
  btn.addEventListener('click', () => {
    const words = typeof text === 'function' ? text() : text;
    if (!words) {
      return;
    }
    navigator.clipboard.writeText(words);
    flash(btn);
  });
  return btn;
}

export function copyGlyph(text) {
  return copier('copy-glyph', 'Copy', text);
}

export function copyAllButton(title, text) {
  return copier('data-grid-copy', title, text);
}

export function dataGrid({columns, rows, trailing}) {
  const picked = new Set(columns.map((c, i) => i));
  const entries = rows.map(r => ({r, tr: el('tr'), num: el('span')}));
  const shown = () => entries.filter(e => !e.tr.hidden).map(e => e.r);
  const table = el('table', 'data-grid');
  const headRow = el('tr');
  const lead = el('th', 'data-grid-copy-cell');
  lead.append(copyAllButton('Copy the checked columns to the clipboard', () => {
    const cols = columns.filter((c, i) => picked.has(i));
    const list = shown();
    if (!cols.length || !list.length) {
      return '';
    }
    return [cols.map(c => c.label).join('\t'), ...list.map(r => cols.map(c => c.get(r)).join('\t'))].join('\n');
  }));
  headRow.append(lead);
  let sortedBy = null;
  let ascending = true;
  const renumber = () => {
    let n = 0;
    for (const e of entries) {
      if (!e.tr.hidden) {
        e.num.textContent = String(++n);
      }
    }
  };
  const sortBy = (c, th) => {
    ascending = sortedBy === c ? !ascending : true;
    sortedBy = c;
    const key = c.sort || c.get;
    entries.sort((a, b) => {
      const x = key(a.r);
      const y = key(b.r);
      if (!x || !y) {
        return x ? -1 : y ? 1 : 0;
      }
      const order = x.localeCompare(y, undefined, {numeric: true, sensitivity: 'base'});
      return ascending ? order : -order;
    });
    for (const cell of headRow.children) {
      cell.removeAttribute('aria-sort');
    }
    th.setAttribute('aria-sort', ascending ? 'ascending' : 'descending');
    tbody.append(...entries.map(e => e.tr));
    renumber();
  };
  columns.forEach((c, i) => {
    const th = el('th');
    const pick = el('span', 'column-pick');
    const sort = el('button', 'data-grid-sort');
    sort.type = 'button';
    sort.title = `Sort by ${c.label}`;
    sort.append(el('span', '', c.label), svg('chevron-down'));
    sort.addEventListener('click', () => sortBy(c, th));
    const box = el('input');
    box.type = 'checkbox';
    box.checked = true;
    box.title = `Include ${c.label} when copying the checked columns`;
    box.addEventListener('change', () => {
      if (box.checked) {
        picked.add(i);
      } else {
        picked.delete(i);
      }
    });
    pick.append(sort, box);
    th.append(pick, copyGlyph(() => shown().map(c.get).filter(Boolean).join('\n')));
    headRow.append(th);
  });
  if (trailing) {
    headRow.append(el('th'));
  }
  const thead = el('thead');
  thead.append(headRow);
  const tbody = el('tbody');
  entries.forEach((e, n) => {
    const num = el('td', 'data-grid-num');
    e.num.textContent = String(n + 1);
    num.append(e.num, copyGlyph(() => columns.map(c => c.get(e.r)).join('\t')));
    e.tr.append(num);
    columns.forEach((c, i) => {
      const td = el('td', i === 0 ? 'data-grid-name' : '');
      td.append(c.show ? c.show(e.r) : c.get(e.r));
      if (c.get(e.r)) {
        td.append(copyGlyph(c.get(e.r)));
      }
      e.tr.append(td);
    });
    if (trailing) {
      const td = el('td', 'data-grid-trailing');
      td.append(trailing(e.r));
      e.tr.append(td);
    }
    tbody.append(e.tr);
  });
  table.append(thead, tbody);
  const wrap = el('div', 'data-grid-wrap');
  wrap.append(table);
  const show = keep => {
    for (const e of entries) {
      e.tr.hidden = !keep(e.r);
    }
    renumber();
  };
  return {wrap, shown, show};
}
