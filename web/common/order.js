const keyDigits = '0123456789abcdefghijklmnopqrstuvwxyz';

function digitAt(key, i) {
  return i < key.length ? key[i] : '0';
}

function tail(key, i) {
  return i < key.length ? key.slice(i) : '';
}

function between(lo, hi) {
  if (hi !== '') {
    let n = 0;
    while (n < hi.length && digitAt(lo, n) === hi[n]) {
      n++;
    }
    if (n > 0) {
      return hi.slice(0, n) + between(tail(lo, n), hi.slice(n));
    }
  }
  let a = 0;
  let b = keyDigits.length;
  if (lo !== '') {
    a = keyDigits.indexOf(lo[0]);
  }
  if (hi !== '') {
    b = keyDigits.indexOf(hi[0]);
  }
  if (b - a > 1) {
    return keyDigits[Math.floor((a + b) / 2)];
  }
  if (hi.length > 1) {
    return hi.slice(0, 1);
  }
  return keyDigits[a] + between(tail(lo, 1), '');
}

export function keyBetween(before, after) {
  return between(before || '', after || '');
}

export function movedKey(keys, from, to) {
  const rest = keys.filter((_, i) => i !== from);
  return keyBetween(rest[to - 1], rest[to]);
}
