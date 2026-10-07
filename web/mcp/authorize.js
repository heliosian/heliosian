import {api} from '/api.js';

const query = location.search;
const $ = id => document.getElementById(id);

function fail(err) {
  $('ask').hidden = true;
  $('error').textContent = err.message;
  $('error').hidden = false;
}

async function go(path) {
  try {
    const {redirect} = await api('POST', path, {query});
    location.href = redirect;
  } catch (err) {
    fail(err);
  }
}

async function start() {
  try {
    const about = await api('POST', '/api/mcp/request', {query});
    $('client').textContent = about.client;
    $('email').textContent = about.email;
    $('redirect').textContent = about.redirect;
    $('ask').hidden = false;
  } catch (err) {
    fail(err);
  }
}

$('allow').addEventListener('click', () => go('/api/mcp/approve'));
$('deny').addEventListener('click', () => go('/api/mcp/deny'));
start();
