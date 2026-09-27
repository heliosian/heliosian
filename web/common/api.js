export function signedIn(res) {
  if (res.status === 401) {
    location.reload();
    return new Promise(() => {});
  }
  return res;
}

export async function api(method, url, body, signal) {
  const init = {method, signal};
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.headers = {'Content-Type': 'application/json'};
    init.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await signedIn(await fetch(url, init));
  } catch (err) {
    if (err.name === 'AbortError') {
      throw err;
    }
    throw new Error('Couldn’t reach the server; check your connection and try again.');
  }
  if (!res.ok) {
    const text = await res.text();
    const err = new Error(text);
    if (res.headers.get('Content-Type') === 'application/json') {
      err.conflict = JSON.parse(text);
      err.message = err.conflict.error || text;
    }
    throw err;
  }
  if (res.status === 204) {
    return null;
  }
  return res.json();
}
