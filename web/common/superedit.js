import {domain} from '/mode.js';

const cookie = 'heliosian-super-edit';

export function superEditOn() {
  return new RegExp('(?:^|; )' + cookie + '=1(?:;|$)').test(document.cookie);
}

export function setSuperEdit(on) {
  const d = domain();
  document.cookie = `${cookie}=${on ? '1' : ''}; path=/; max-age=${on ? 34560000 : 0}; SameSite=Lax${d ? '; domain=' + d : ''}${location.protocol === 'https:' ? '; Secure' : ''}`;
}
