#!/usr/bin/env python3
"""Generate editor sources and local install archives using only Python's stdlib."""
import argparse
import json
from pathlib import Path
import plistlib
import uuid
import zipfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SPEC = json.loads((ROOT / "language.json").read_text())
SCOPE = "source.sid64-starlark"


def json_bytes(value):
    return (json.dumps(value, indent=2, ensure_ascii=False) + "\n").encode()


def words(names):
    return r"\b(?:" + "|".join(names) + r")\b"


def generated():
    api = SPEC["world"] + SPEC["game"]
    patterns = [{"name": "comment.line.number-sign.starlark", "match": "#.*$"}]
    # Triple-quoted strings precede single-quoted strings. Raw strings do not
    # highlight escapes, but backslash-escaped delimiters still stay inside.
    for raw in (True, False):
        for quote in ('"""', "'''", '"', "'"):
            triple = len(quote) == 3
            patterns.append({
                "name": "string.quoted." + ("triple" if triple else "single") + ".starlark",
                "begin": (r"\b[rR]" if raw else "") + quote,
                "end": quote if triple else quote + "|$",
                "patterns": [{"match": r"\\.", **({} if raw else {"name": "constant.character.escape.starlark"})}],
            })
    patterns += [
        {"name": "support.function.hook.sid64", "match": words(SPEC["hooks"])},
        {"match": r"\b(def)\s+([A-Za-z_]\w*)", "captures": {
            "1": {"name": "storage.type.function.starlark"},
            "2": {"name": "entity.name.function.starlark"}}},
        {"name": "support.function.sid64", "match": r"(?<!\.)" + words(api) + r"(?=\s*\()"},
        {"name": "support.function.builtin.starlark", "match": r"(?<!\.)" + words(SPEC["builtins"]) + r"(?=\s*\()"},
        {"name": "keyword.control.starlark", "match": words(SPEC["keywords"])},
        {"name": "constant.language.starlark", "match": words(["True", "False", "None"])},
        {"name": "variable.other.property.sid64", "match": r"(?<=\.)" + words(SPEC["fields"])},
        {"name": "constant.numeric.starlark", "match": r"\b(?:0[xX][0-9a-fA-F]+|0[oO][0-7]+|0[bB][01]+|[0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?)\b|(?<!\w)\.[0-9]+(?:[eE][+-]?[0-9]+)?\b"},
        {"name": "keyword.operator.starlark", "match": r"[-+*/%=<>!&|^~]+"},
        {"name": "punctuation.section.starlark", "match": r"[()\[\]{}]"},
    ]
    grammar = {"name": SPEC["name"], "scopeName": SCOPE, "fileTypes": SPEC["extensions"], "patterns": patterns}
    plist = plistlib.dumps(grammar, sort_keys=False)
    preferences = plistlib.dumps({"name": "SID64 Starlark", "scope": SCOPE,
        "settings": {"shellVariables": [{"name": "TM_COMMENT_START", "value": "# "}], "tabSize": 4, "softTabs": True}}, sort_keys=False)
    files = {
        "vscode/syntaxes/sid64-starlark.tmLanguage.json": json_bytes(grammar),
        "textmate/SID64-Starlark.tmbundle/Syntaxes/SID64-Starlark.tmLanguage": plist,
        "textmate/SID64-Starlark.tmbundle/Preferences/SID64-Starlark.tmPreferences": preferences,
        "textmate/SID64-Starlark.tmbundle/info.plist": plistlib.dumps({"name": SPEC["name"], "uuid": str(uuid.uuid5(uuid.NAMESPACE_URL, "https://sid64.quest/editors/starlark"))}),
        "sublime/SID64-Starlark.tmLanguage": plist,
        "sublime/SID64-Starlark.tmPreferences": preferences,
        "sublime/SID64-Starlark.sublime-settings": json_bytes({"extensions": ["star"], "tab_size": 4, "translate_tabs_to_spaces": True}),
    }
    manifest = {
        "name": "sid64-starlark", "displayName": SPEC["name"], "version": SPEC["version"],
        "description": "Starlark syntax, SID64 Quest world and combat definitions, and game event hooks.",
        "publisher": "sid64-quest", "license": "UNLICENSED", "engines": {"vscode": "^1.75.0"},
        "categories": ["Programming Languages", "Snippets"],
        "repository": {"type": "git", "url": "https://github.com/tylerhardison/race-condition-kingdom"},
        "capabilities": {"untrustedWorkspaces": {"supported": True}, "virtualWorkspaces": True},
        "contributes": {
            "languages": [{"id": "sid64-starlark", "aliases": [SPEC["name"]], "extensions": [".star"], "configuration": "./language-configuration.json"}],
            "grammars": [{"language": "sid64-starlark", "scopeName": SCOPE, "path": "./syntaxes/sid64-starlark.tmLanguage.json"}],
            "snippets": [{"language": "sid64-starlark", "path": "./snippets/sid64.json"}],
            "configurationDefaults": {"[sid64-starlark]": {"editor.insertSpaces": True, "editor.tabSize": 4}},
        },
    }
    files["vscode/package.json"] = json_bytes(manifest)
    files["vscode/language-configuration.json"] = json_bytes({
        "comments": {"lineComment": "#"}, "brackets": [["{", "}"], ["[", "]"], ["(", ")"]],
        "autoClosingPairs": [{"open": a, "close": b, "notIn": ["string", "comment"]} for a, b in [("{", "}"), ("[", "]"), ("(", ")"), ('"', '"'), ("'", "'")]],
        "indentationRules": {"increaseIndentPattern": r"^\s*(?:def|if|elif|else|for)\b.*:\s*(?:#.*)?$", "decreaseIndentPattern": r"^\s*(?:elif|else)\b.*:"},
    })
    snippets = {}
    for hook in SPEC["hooks"]:
        snippets[hook] = {"prefix": hook, "body": [f"def {hook}(event):", "    ${1:pass}"], "description": "SID64 event hook; check the script kind before attaching."}
    for name, body in {
        "sid-room": ['room("${1:key}", "${2:Room name}", "${3:Description}", kind="${4:normal}")'],
        "sid-link": ['link("${1:from_room}", "${2:north}", "${3:to_room}")'],
        "sid-monster": ['monster("${1:key}", "${2:room_key}", "${3:Name}", "${4:Description}",', '        health=${5:20}, attack=${6:5}, gold=${7:5}, experience=${8:10}, respawn=${9:300})'],
    }.items():
        snippets[name] = {"prefix": name, "body": body, "description": "Startup-only world definition"}
    files["vscode/snippets/sid64.json"] = json_bytes(snippets)
    vim = '''" Generated by editors/tools/build.py; edit language.json or the generator.
if exists('b:current_syntax') | finish | endif
syntax case match
syntax sync fromstart
syntax match sid64Comment "#.*$"
syntax match sid64Number "\\<\\d\\+\\(\\.\\d*\\)\\?\\([eE][+-]\\?\\d\\+\\)\\?\\>"
syntax match sid64Number "\\<0[xX][0-9a-fA-F]\\+\\>"
syntax region sid64String start=+"+ skip=+\\\\.+ end=+"+ oneline
syntax region sid64String start=+'+ skip=+\\\\.+ end=+'+ oneline
syntax region sid64String start=+"""+ skip=+\\\\.+ end=+"""+
syntax region sid64String start=+'''+"'''"+'''+ skip=+\\\\.+ end=+'''+"'''"+'''+
syntax match sid64Function "\\<def\\s\\+\\zs\\h\\w*"
'''
    for group, names in [("Keyword", SPEC["keywords"]), ("Constant", ["True", "False", "None"]), ("API", api), ("Builtin", SPEC["builtins"]), ("Hook", SPEC["hooks"])]:
        vim += "syntax keyword sid64" + group + " " + " ".join(names) + "\n"
    for group, link in {"Comment": "Comment", "Number": "Number", "String": "String", "Function": "Function", "Keyword": "Keyword", "Constant": "Constant", "API": "Function", "Builtin": "Function", "Hook": "Special"}.items():
        vim += f"highlight default link sid64{group} {link}\n"
    vim += "let b:current_syntax = 'sid64starlark'\n"
    files["vim/syntax/sid64starlark.vim"] = vim.encode()
    files["vim/ftdetect/sid64starlark.vim"] = b"augroup sid64starlark\n  autocmd!\n  autocmd BufRead,BufNewFile *.star if empty(&l:filetype) || &l:filetype ==# 'starlark' | setlocal filetype=sid64starlark | endif\naugroup END\n"
    files["vim/ftplugin/sid64starlark.vim"] = b"if exists('b:did_ftplugin') | finish | endif\nlet b:did_ftplugin = 1\nsetlocal commentstring=#\\ %s comments=:# expandtab shiftwidth=4 softtabstop=4\nlet b:undo_ftplugin = 'setlocal commentstring< comments< expandtab< shiftwidth< softtabstop<'\n"
    lisp = ''';;; sid64-starlark-mode.el --- SID64 Quest Starlark highlighting -*- lexical-binding: t; -*-
;; Version: ''' + SPEC["version"] + '''
;; Package-Requires: ((emacs "26.1"))
;;; Commentary:
;; Python-derived editing support with SID64 world APIs and event hooks.
;; Highlighting is not runtime validation; Python-only features are unavailable.
;;; Code:
(require 'python)
(defconst sid64-starlark-font-lock-keywords
  `((,(regexp-opt 'API_LIST 'symbols) . font-lock-builtin-face)
    (,(regexp-opt 'HOOK_LIST 'symbols) . font-lock-function-name-face)))
;;;###autoload
(define-derived-mode sid64-starlark-mode python-mode "SID64 Starlark"
  "Edit SID64 Quest Starlark scripts with game API highlighting."
  (setq-local indent-tabs-mode nil)
  (setq-local python-indent-offset 4)
  (font-lock-add-keywords nil sid64-starlark-font-lock-keywords 'append))
;;;###autoload
(add-to-list 'auto-mode-alist '("\\\\.star\\\\'" . sid64-starlark-mode))
(provide 'sid64-starlark-mode)
;;; sid64-starlark-mode.el ends here
'''
    lisp_list = lambda names: "(" + " ".join('"' + name + '"' for name in names) + ")"
    files["emacs/sid64-starlark-mode.el"] = lisp.replace("API_LIST", lisp_list(api)).replace("HOOK_LIST", lisp_list(SPEC["hooks"])).encode()
    return files


def archive(path, entries):
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        for name, content in sorted(entries.items()):
            info = zipfile.ZipInfo(name, (2020, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            z.writestr(info, content)


def package(files, dest):
    dest.mkdir(parents=True, exist_ok=True)
    readme = (ROOT / "README.md").read_bytes().replace(b"(../docs/", b"(docs/")
    docs = {"docs/" + name: (ROOT.parent / "docs" / name).read_bytes().replace(b"(../editors/README.md)", b"(../README.md)") for name in ["SCRIPTING.md", "WORLD-CONTENT.md"]}
    # A declarative VSIX needs no executable extension host or npm dependencies.
    ns = "http://schemas.microsoft.com/developer/vsx-schema/2011"
    ET.register_namespace("", ns)
    element = lambda name: "{" + ns + "}" + name
    manifest = ET.Element(element("PackageManifest"), {"Version": "2.0.0"})
    metadata = ET.SubElement(manifest, element("Metadata"))
    ET.SubElement(metadata, element("Identity"), {"Language": "en-US", "Id": "sid64-starlark", "Version": SPEC["version"], "Publisher": "sid64-quest"})
    ET.SubElement(metadata, element("DisplayName")).text = SPEC["name"]
    ET.SubElement(metadata, element("Description")).text = "SID64 Quest Starlark highlighting and snippets."
    props = ET.SubElement(metadata, element("Properties"))
    ET.SubElement(props, element("Property"), {"Id": "Microsoft.VisualStudio.Code.Engine", "Value": "^1.75.0"})
    targets = ET.SubElement(manifest, element("Installation"))
    ET.SubElement(targets, element("InstallationTarget"), {"Id": "Microsoft.VisualStudio.Code"})
    ET.SubElement(manifest, element("Dependencies"))
    assets = ET.SubElement(manifest, element("Assets"))
    ET.SubElement(assets, element("Asset"), {"Type": "Microsoft.VisualStudio.Code.Manifest", "Path": "extension/package.json", "Addressable": "true"})
    entries = {"extension/" + key.removeprefix("vscode/"): value for key, value in files.items() if key.startswith("vscode/")}
    entries["extension/README.md"] = readme
    entries.update({"extension/" + name: data for name, data in docs.items()})
    entries["extension.vsixmanifest"] = ET.tostring(manifest, encoding="utf-8", xml_declaration=True)
    entries["[Content_Types].xml"] = b'<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="json" ContentType="application/json"/><Default Extension="md" ContentType="text/markdown"/><Default Extension="vsixmanifest" ContentType="text/xml"/></Types>'
    archive(dest / ("sid64-starlark-" + SPEC["version"] + ".vsix"), entries)
    for directory, filename in [("sublime", "SID64-Starlark.sublime-package"), ("textmate", "SID64-Starlark.tmbundle.zip"), ("vim", "sid64-starlark-vim.zip"), ("emacs", "sid64-starlark-emacs.zip")]:
        entries = {key.removeprefix(directory + "/"): value for key, value in files.items() if key.startswith(directory + "/")}
        entries["README.md"] = readme
        entries.update(docs)
        archive(dest / filename, entries)
    print("Packages written to", dest)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Check generated sources without writing")
    parser.add_argument("--package", action="store_true", help="Also build installable archives")
    parser.add_argument("--output", type=Path, default=ROOT.parent / "build" / "editors")
    args = parser.parse_args()
    files = generated()
    for key, value in files.items():
        path = ROOT / key
        if args.check:
            if not path.exists() or path.read_bytes() != value:
                raise SystemExit("Generated file is stale: " + key)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(value)
    if args.package:
        package(files, args.output)


if __name__ == "__main__":
    main()
