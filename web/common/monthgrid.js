import {el, link} from '/elements.js';

export function monthGrid(month, items, className) {
  const cal = el('div', 'calendar' + (className ? ' ' + className : ''));
  for (const d of ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']) {
    cal.append(el('div', 'dow', d));
  }
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const start = new Date(first);
  start.setDate(1 - first.getDay());
  const today = new Date().toDateString();
  for (let i = 0; i < 42; i++) {
    const day = new Date(start);
    day.setDate(start.getDate() + i);
    const cell = el('div', 'day' + (day.getMonth() !== month.getMonth() ? ' other' : '') + (day.toDateString() === today ? ' today' : ''));
    cell.append(el('div', 'num', String(day.getDate())));
    for (const item of items.filter(e => e.key === day.toDateString())) {
      const words = item.label || item.title;
      const chipClass = 'chip' + (item.className ? ' ' + item.className : '');
      let chip;
      if (item.onClick) {
        chip = el('button', chipClass, words);
        chip.type = 'button';
        chip.addEventListener('click', item.onClick);
      } else {
        chip = link(item.href, chipClass, words);
      }
      if (item.card) {
        const wrap = el('span', 'chip-wrap');
        const pop = el('span', 'chip-pop');
        const head = el('span', 'chip-pop-head');
        head.append(el('span', 'chip-pop-title', item.card.title), el('span', 'chip-pop-when', item.card.when));
        pop.append(head);
        if (!item.card.rows.length) {
          pop.append(el('span', 'chip-pop-empty', item.card.empty || 'Nothing here'));
        }
        for (const row of item.card.rows) {
          const line = el('span', 'chip-pop-row');
          const who = el('span', 'chip-pop-who');
          who.append(el('span', 'chip-pop-name', row.name), el('span', 'chip-pop-sub', row.sub));
          line.append(who, el('span', 'chip-pop-note' + (row.warn ? ' is-warn' : ''), row.note));
          pop.append(line);
        }
        wrap.append(chip, pop);
        cell.append(wrap);
        continue;
      }
      chip.title = item.title;
      cell.append(chip);
    }
    cal.append(cell);
  }
  return cal;
}
