const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const tm = require('vscode-textmate');
const onig = require('vscode-oniguruma');
const root = path.resolve(__dirname, '..');
(async () => {
  const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm'));
  await onig.loadWASM(wasm.buffer.slice(wasm.byteOffset, wasm.byteOffset + wasm.byteLength));
  const registry = new tm.Registry({
    onigLib: Promise.resolve({createOnigScanner: p => new onig.OnigScanner(p), createOnigString: s => new onig.OnigString(s)}),
    loadGrammar: async scope => scope === 'source.sid64-starlark'
      ? JSON.parse(fs.readFileSync(path.join(root, 'vscode/syntaxes/sid64-starlark.tmLanguage.json'), 'utf8')) : null,
  });
  const grammar = await registry.loadGrammar('source.sid64-starlark');
  function scopes(line, word, state = tm.INITIAL) {
    const index = line.indexOf(word);
    assert.ok(index >= 0);
    return grammar.tokenizeLine(line, state).tokens.find(t => t.startIndex <= index && t.endIndex > index).scopes;
  }
  function has(line, word, prefix, state) {
    assert.ok(scopes(line, word, state).some(s => s.startsWith(prefix)), `${line}: ${word} needs ${prefix}`);
  }
  const spec = JSON.parse(fs.readFileSync(path.join(root, 'language.json')));
  for (const name of [...spec.world, ...spec.game]) {
    has(`${name}("hello")`, name, 'support.function.sid64');
    has(`# ${name}("hello")`, name, 'comment.');
    has(`"${name}(hello)"`, name, 'string.');
  }
  for (const hook of spec.hooks) has(`def ${hook}(event):`, hook, 'entity.name.function');
  has('if event.command == "answer":', 'if', 'keyword.');
  has('if event.command == "answer":', 'command', 'variable.other.property.');
  has('health = 0xFF + 1.5e2 + .25', '0xFF', 'constant.numeric');
  has('health = 0xFF + 1.5e2 + .25', '.25', 'constant.numeric');
  has('tell("escaped \\" + "text")', 'escaped', 'string.');
  let state = grammar.tokenizeLine('text = """first line', tm.INITIAL).ruleStack;
  has('monster("still a string") # still a string', 'monster', 'string.', state);
  state = grammar.tokenizeLine('"""', state).ruleStack;
  has('monster("outside")', 'monster', 'support.function.sid64', state);
  has('r"room(\\path)"', 'room', 'string.');
  has('event.room', 'room', 'variable.other.property.');
  for (const file of fs.readdirSync(path.join(root, '../internal/game/content')).filter(f => f.endsWith('.star'))) {
    state = tm.INITIAL;
    for (const line of fs.readFileSync(path.join(root, '../internal/game/content', file), 'utf8').split('\n')) {
      const result = grammar.tokenizeLine(line, state);
      assert.ok(!result.stoppedEarly, file);
      state = result.ruleStack;
    }
    assert.equal(state.depth, 1, `${file}: unterminated string state`);
  }
  registry.dispose();
  console.log('TextMate tokenization passed for APIs, hooks, literals, and all bundled scripts.');
})().catch(error => {console.error(error); process.exitCode = 1;});
