import {el, svg} from '/elements.js';
import {pageSize, fillPager} from '/members.js';

export function countList(items) {
  const list = el('div', 'count-list');
  for (const item of items) {
    const row = el('button', 'count-row' + (item.tone ? ' is-' + item.tone : ''));
    row.type = 'button';
    const mark = el('span', 'count-row-mark');
    mark.append(svg(item.icon));
    row.append(mark, el('span', 'count-row-label', item.label));
    if (item.count !== undefined) {
      row.append(el('strong', 'count-row-count', String(item.count)));
    }
    row.append(svg('chevron-right'));
    list.append(row);
    if (!item.expand) {
      row.addEventListener('click', item.onClick);
      continue;
    }
    const panel = el('div', 'count-panel');
    panel.hidden = true;
    row.setAttribute('aria-expanded', 'false');
    const rows = el('div', 'count-panel-rows');
    const pager = el('div', 'member-pager count-panel-pager');
    let people = [];
    const draw = page => {
      rows.replaceChildren(...(people.length ? people.slice(page * pageSize, (page + 1) * pageSize) : [el('p', 'count-panel-empty', 'Nobody yet.')]));
      fillPager(pager, page, people.length, draw);
    };
    panel.append(rows, pager);
    row.addEventListener('click', () => {
      const open = panel.hidden;
      people = open ? item.expand() : [];
      draw(0);
      panel.hidden = !open;
      row.classList.toggle('is-open', open);
      row.setAttribute('aria-expanded', String(open));
    });
    list.append(panel);
  }
  return list;
}
