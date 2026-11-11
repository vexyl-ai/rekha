// Both pages carry the tags search engines and link previews read, and link to each other.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const pages = {
  landing: { file: path.join(__dirname, 'index.html'), url: 'https://vexyl-ai.github.io/rekha/' },
  demo: { file: path.join(__dirname, 'demo', 'index.html'), url: 'https://vexyl-ai.github.io/rekha/demo/' },
};
const count = (s, re) => (s.match(re) || []).length;
const attr = (s, name, key) => {
  const m = s.match(new RegExp(`<(?:meta|link)[^>]*${key}="${name}"[^>]*>`));
  return m ? m[0] : '';
};

for (const [name, page] of Object.entries(pages)) {
  const html = fs.readFileSync(page.file, 'utf8');
  test(`${name}: one h1 and a distinct title`, () => {
    assert.equal(count(html, /<h1[\s>]/g), 1);
    const title = html.match(/<title>([^<]+)<\/title>/)[1];
    assert.ok(title.length > 20 && title.length <= 70, title);
  });
  test(`${name}: description, canonical and link-preview tags`, () => {
    assert.match(attr(html, 'description', 'name'), /content="[^"]{50,160}"/);
    assert.match(attr(html, 'canonical', 'rel'), new RegExp(`href="${page.url}"`));
    for (const p of ['og:title', 'og:description', 'og:image', 'og:url']) {
      assert.match(attr(html, p, 'property'), /content="[^"]+"/, p);
    }
    assert.match(attr(html, 'og:url', 'property'), new RegExp(`content="${page.url}"`));
    assert.match(attr(html, 'twitter:card', 'name'), /summary_large_image/);
  });
}
test('landing links to the playground and the demo links home', () => {
  const landing = fs.readFileSync(pages.landing.file, 'utf8');
  const demo = fs.readFileSync(pages.demo.file, 'utf8');
  assert.match(landing, /href="demo\/"/);
  assert.match(demo, /<a class="brand" href="\.\.\/"/);
});
test('landing titles differ from the demo title', () => {
  const t = (f) => fs.readFileSync(f, 'utf8').match(/<title>([^<]+)<\/title>/)[1];
  assert.notEqual(t(pages.landing.file), t(pages.demo.file));
});
test('every local asset the pages reference exists', () => {
  for (const [name, page] of Object.entries(pages)) {
    const html = fs.readFileSync(page.file, 'utf8');
    for (const m of html.matchAll(/(?:src|href)="([^"#:]+)"/g)) {
      const ref = m[1].replace(/\/$/, '/index.html');
      if (ref.endsWith('.wasm') || ref === 'wasm_exec.js') continue;
      assert.ok(fs.existsSync(path.join(path.dirname(page.file), ref)), `${name}: ${m[1]}`);
    }
  }
});
