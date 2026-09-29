const indent = '⠀⠀';

function key(name) {
  return name.replace(/\W/g, '_');
}

function inner(property) {
  if (property.items) {
    return inner(property.items);
  }
  if (property.additionalProperties) {
    return inner(property.additionalProperties);
  }
  return property;
}

function typeOf(property) {
  const types = [property.type ?? 'any'].flat().filter(t => t !== 'null');
  const name = types[0] ?? 'any';
  if (name === 'array' && property.items) {
    return typeOf(property.items) + '[]';
  }
  if (name === 'object' && property.additionalProperties) {
    return `map~${typeOf(property.additionalProperties)}~`;
  }
  return name;
}

function fields(properties, depth, lines) {
  for (const field of Object.keys(properties).sort()) {
    const property = properties[field];
    lines.push(`    ${indent.repeat(depth)}${typeOf(property)} ${field}`);
    const nested = inner(property).properties;
    if (nested) {
      fields(nested, depth + 1, lines);
    }
  }
}

function diagram(spec) {
  const lines = ['classDiagram'];
  const links = [];
  const schemas = spec.components.schemas;
  for (const name of Object.keys(schemas).sort()) {
    if (name === 'resources') {
      continue;
    }
    const own = {};
    for (const [field, property] of Object.entries(schemas[name].properties)) {
      if (property['x-relation']) {
        const arrow = property.type === 'array' ? '--*' : '-->';
        links.push(`  ${key(name)} ${arrow} ${key(property['x-relation'])} : ${field}`);
        continue;
      }
      if (field !== 'can') {
        own[field] = property;
      }
    }
    lines.push(`  class ${key(name)}["${name}"] {`);
    fields(own, 0, lines);
    lines.push('  }');
  }
  return [...lines, ...links].join('\n');
}

function doubleArrowheads(root) {
  for (const [suffix, refX] of [['-compositionEnd', 13], ['-compositionEnd-margin', 16]]) {
    for (const marker of root.querySelectorAll(`marker[id$="${suffix}"]`)) {
      marker.setAttribute('refX', refX);
      marker.querySelector('path').setAttribute('d', 'M 18,7 L9,13 L14,7 L9,1 Z M 11,7 L2,13 L7,7 L2,1 Z');
    }
  }
}

const response = await fetch('/api/openapi.json');
if (!response.ok) {
  throw new Error(`openapi.json: ${response.status}`);
}
window.mermaid.initialize({startOnLoad: false, maxTextSize: 1000000});
const {svg} = await window.mermaid.render('erd-svg', diagram(await response.json()));
const root = document.getElementById('erd');
root.innerHTML = svg;
doubleArrowheads(root);
