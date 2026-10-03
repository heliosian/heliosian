import {chrome} from '/chrome.js';

chrome('api');
window.SwaggerUIBundle({url: '/api/openapi.json', dom_id: '#swagger-ui', deepLinking: true, validatorUrl: null});
