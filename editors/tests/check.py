#!/usr/bin/env python3
"""Check generated formats and real Vim highlighting without third-party modules."""
import json
from pathlib import Path
import plistlib
import shutil
import subprocess
import tempfile
import zipfile
import xml.etree.ElementTree as ET

root = Path(__file__).resolve().parents[1]
subprocess.run(["python3", str(root / "tools/build.py"), "--check"], check=True)
grammar = json.loads((root / "vscode/syntaxes/sid64-starlark.tmLanguage.json").read_text())
for path in [root / "sublime/SID64-Starlark.tmLanguage", root / "textmate/SID64-Starlark.tmbundle/Syntaxes/SID64-Starlark.tmLanguage"]:
    assert plistlib.loads(path.read_bytes()) == grammar
manifest = json.loads((root / "vscode/package.json").read_text())
for group, key in [("grammars", "path"), ("snippets", "path"), ("languages", "configuration")]:
    for contribution in manifest["contributes"][group]:
        assert (root / "vscode" / contribution[key]).is_file()
with tempfile.TemporaryDirectory(prefix="sid64-editors-") as tmp:
    tmp = Path(tmp)
    subprocess.run(["python3", str(root / "tools/build.py"), "--check", "--package", "--output", str(tmp)], check=True)
    archives = sorted(tmp.glob("*.zip")) + sorted(tmp.glob("*.vsix")) + sorted(tmp.glob("*.sublime-package"))
    assert len(archives) == 5
    for archive in archives:
        with zipfile.ZipFile(archive) as z:
            assert z.testzip() is None
            assert all(not name.startswith("/") and ".." not in Path(name).parts for name in z.namelist())
            if archive.suffix == ".vsix":
                ET.fromstring(z.read("extension.vsixmanifest"))
                ET.fromstring(z.read("[Content_Types].xml"))
                assert json.loads(z.read("extension/package.json")) == manifest
    vim = shutil.which("vim")
    if vim:
        sample = tmp / "fixture.star"
        sample.write_text('# tell("comment")\nroom("tell in string")\ndef on_enter(event):\n    tell("hello")\ntext = """begin\nmonster inside string\nend"""\nmonster("outside")\n')
        script = tmp / "test.vim"
        # fnameescape avoids spaces and metacharacters in checkout locations.
        quote = lambda s: "'" + str(s).replace("'", "''") + "'"
        script.write_text(f"set nocompatible\nexecute 'set runtimepath^=' . fnameescape({quote(root / 'vim')})\nfiletype plugin on\nsyntax enable\nexecute 'edit ' . fnameescape({quote(sample)})\n" + '''call assert_equal('sid64starlark', &filetype)
call assert_equal('sid64Comment', synIDattr(synID(1, 3, 1), 'name'))
call assert_equal('sid64API', synIDattr(synID(2, 1, 1), 'name'))
call assert_equal('sid64String', synIDattr(synID(2, 7, 1), 'name'))
call assert_equal('sid64Hook', synIDattr(synID(3, 5, 1), 'name'))
call assert_equal('sid64String', synIDattr(synID(6, 1, 1), 'name'))
call assert_equal('sid64API', synIDattr(synID(8, 1, 1), 'name'))
call assert_equal(4, &shiftwidth)
call assert_equal('# %s', &commentstring)
if !empty(v:errors)
''' + f"call writefile(v:errors, {quote(tmp / 'errors')})\ncquit\nendif\nqa!\n")
        result = subprocess.run([vim, "-Nu", "NONE", "-n", "-es", "-S", str(script)], capture_output=True, text=True)
        assert result.returncode == 0, (tmp / "errors").read_text() if (tmp / "errors").exists() else result.stderr
        print("Native Vim file detection, syntax, comments, and indentation passed.")
    else:
        print("Vim not installed; native Vim checks skipped.")
print("Generated sources, matching grammars, and all five archives passed.")
