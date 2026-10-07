const address = location.origin + '/';
document.getElementById('address').textContent = address;
document.getElementById('command').textContent = 'claude mcp add --transport http helios ' + address;
