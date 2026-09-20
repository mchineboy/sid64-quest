# SID64 Starlark editor packages

Syntax highlighting for `.star` files used by SID64 Quest. Includes Starlark keywords, comments, strings (including multiline strings), numbers, the `room`, `link`, and `monster` world API, the six runtime game functions, and all six event hooks. Editor themes choose the colors. Packages contain no game runtime, network client, or language server.

## Build installable packages

From the repository root, with Python 3.9 or newer:

```sh
python3 editors/tools/build.py --package
```

Archives are written to `build/editors/`. Generated editor sources are also checked into this directory, so most editors can use them without a build. The packaging script uses only Python's standard library and does not publish or install anything.

| Editor | Package | Install |
|---|---|---|
| VS Code, Cursor, VSCodium, Windsurf | `sid64-starlark-0.1.0.vsix` | Run **Extensions: Install from VSIX…**, choose the archive, then select **SID64 Starlark** as the language if another extension owns `.star`. |
| IntelliJ IDEA, PyCharm, WebStorm, GoLand, other JetBrains IDEs with TextMate support | `SID64-Starlark.tmbundle.zip` | Extract, enable **TextMate Bundles**, then add the `SID64-Starlark.tmbundle` folder in **Settings → Editor → TextMate Bundles**. |
| Sublime Text | `SID64-Starlark.sublime-package` | Open **Preferences → Browse Packages…**, go up one directory, and copy the archive into **Installed Packages**. Choose **SID64 Starlark** from the syntax menu if needed. |
| TextMate | `SID64-Starlark.tmbundle.zip` | Extract and open the `.tmbundle` folder with TextMate. |
| Vim, Neovim | `sid64-starlark-vim.zip` | Extract into a native `pack/.../start/` plugin directory; see below. |
| Emacs | `sid64-starlark-emacs.zip` | Extract, add the directory to `load-path`, and require the mode; see below. |

All packages associate only `.star`. They do not claim Python files, Bazel BUILD files, or `.bzl` files. Existing file-type plugins can win automatic detection; select the mode explicitly rather than uninstalling unrelated language support.

### VS Code family

The VSIX includes comment toggling, bracket/quote pairing, four-space indentation, and snippets for `on_enter`, `on_look`, `on_say`, `on_command`, `on_talk`, `on_use`, `sid-room`, `sid-link`, and `sid-monster`. It works as a declarative grammar without an extension-host process.

To make the language choice explicit in a project, add to workspace settings:

```json
{"files.associations": {"*.star": "sid64-starlark"}}
```

CLI alternative for VS Code:

```sh
code --install-extension build/editors/sid64-starlark-0.1.0.vsix
```

Other VSIX-compatible editors can use their own **Install from VSIX** action. No marketplace publication or publisher account is required for local installation.

### Vim and Neovim

Extract the ZIP contents (`syntax/`, `ftdetect/`, `ftplugin/`) into one of:

- Vim: `~/.vim/pack/sid64/start/sid64-starlark/`
- Neovim on Linux/macOS: `~/.local/share/nvim/site/pack/sid64/start/sid64-starlark/`
- Neovim on Windows: `%LOCALAPPDATA%/nvim-data/site/pack/sid64/start/sid64-starlark/`

For a source checkout, a plugin manager may point directly at `editors/vim`; it must place that directory on `runtimepath`, rather than the repository root. Enable filetype plugins and syntax in Vim:

```vim
filetype plugin on
syntax enable
```

Use `:setfiletype sid64starlark` if the file has no type, or `:set filetype=sid64starlark` to override an existing association. This package uses native syntax highlighting and requires no Tree-sitter parser. The filetype plugin sets four-space indentation and `#` comments; it does not provide an automatic indenter.

### Emacs

Extract the package, then add this to your init file with your chosen install path:

```elisp
(add-to-list 'load-path "/path/to/sid64-starlark-emacs")
(require 'sid64-starlark-mode)
```

Alternatively, `M-x package-install-file` can install `sid64-starlark-mode.el` directly. The mode derives from built-in `python-mode`, adds SID64 API highlighting, uses four-space indentation, and associates `.star` files. `M-x sid64-starlark-mode` selects it manually. Python editing commands inherited from the base mode are not a Starlark execution environment; do not run these scripts as Python.

## Language scope

These packages provide highlighting, not validation, completion from runtime state, or a debugger. In particular, coloring `room()` does not make it available to event hooks: world declarations run only during content installation, while `tell()`, state, and rewards belong to event hooks. Snippets do not bypass the supported-hook rules. See [scripting](../docs/SCRIPTING.md) and [world/combat authoring](../docs/WORLD-CONTENT.md).

## Maintaining and testing

`language.json` is the API/keyword registry. `tools/build.py` generates the self-contained TextMate grammar, its VS Code JSON and Sublime/TextMate plist variants, Vim syntax files, Emacs mode, and VS Code snippets. Edit the registry or generator, then rebuild; do not hand-edit generated files. Archives use fixed timestamps and sorted entries for reproducible builds.

```sh
python3 editors/tools/build.py --check
python3 editors/tests/check.py
```

The structural check also runs native Vim assertions when Vim is installed. For real TextMate tokenization tests, install the pinned development dependencies and run:

```sh
cd editors
npm install --ignore-scripts
npm test
```

Alternatively, set `NODE_PATH` to an installed VS Code application's `node_modules` directory and run `node editors/tests/grammar.cjs` from the repository root. Tests check API tokens, strings/comments, multiline state, escapes, and the shipped game scripts. Emacs can be checked separately, when installed, with:

```sh
emacs --batch -Q -L editors/emacs -l editors/tests/emacs-test.el
```

GUI editor behavior should still be smoke-tested in each target editor before publishing a marketplace release. The repository packages are local-install artifacts, not marketplace listings.

Format references: [VS Code grammars](https://code.visualstudio.com/api/language-extensions/syntax-highlight-guide), [JetBrains TextMate bundles](https://www.jetbrains.com/help/idea/textmate.html), [Sublime syntax formats](https://www.sublimetext.com/docs/syntax.html), and [TextMate grammars](https://macromates.com/manual/en/language_grammars).
