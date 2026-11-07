#!/usr/bin/env python3
"""Check complete app catalogs and prevent new untranslated source literals."""

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CATALOGS = ROOT / "internal" / "locales"
GO_TOKENS = re.compile(r'//[^\n]*|/\*[\s\S]*?\*/|"(?:\\.|[^"\\])*"|`[^`]*`')
GO_FUNCTION = re.compile(r"^func\s+(?:\([^\n]*?\)\s+)?(\w+)\s*\(", re.MULTILINE)
HAN = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff]")
PRINTF = re.compile(r"%(?:\[\d+\])?[-+#0 ]*(?:\d+|\*)?(?:\.(?:\d+|\*))?(?:\[\d+\])?[a-zA-Z%]")

# These literals are bilingual search terms or existing user-editable sample
# catalog data. They are never translated at a general text-rendering boundary.
LITERAL_DATA = {
    "internal/desktop/catalog.go": {"demoGroups", "groupDeleteDestination"},
    "internal/desktop/settings.go": {"settingsCategoryHasMatch", "terminalColorSettings"},
    "internal/desktop/credentials_ui.go": {"credentialGroup"},
    "internal/desktop/application_security.go": {"securitySettingsMatch"},
}


def load_catalog(path):
    def unique_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"{path.name}: duplicate key {key}")
            result[key] = value
        return result

    return json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique_pairs)


def main():
    source = load_catalog(CATALOGS / "zh-Hans.json")
    english = load_catalog(CATALOGS / "en.json")
    keys = {key for key in source if not key.startswith("@")}
    errors = []
    english_keys = {key for key in english if not key.startswith("@")}
    for key in sorted(keys - english_keys):
        errors.append(f"en.json: missing {key}")
    for key in sorted(english_keys - keys):
        errors.append(f"en.json: unknown {key}")
    for key in sorted(keys & english_keys):
        translated = english[key]
        if not isinstance(translated, str) or not translated.strip():
            errors.append(f"en.json: empty message {key}")
            continue
        if HAN.search(translated):
            errors.append(f"en.json: untranslated Chinese in {key}")
        expected = [item for item in PRINTF.findall(source[key]) if item != "%%"]
        actual = [item for item in PRINTF.findall(translated) if item != "%%"]
        if actual != expected:
            errors.append(f"en.json: printf arguments differ in {key}: {expected} != {actual}")

    for path in sorted((ROOT / "internal").rglob("*.go")):
        if path.name.endswith("_test.go") or path.parent == CATALOGS:
            continue
        relative = path.relative_to(ROOT).as_posix()
        text = path.read_text(encoding="utf-8")
        functions = list(GO_FUNCTION.finditer(text))
        for token in GO_TOKENS.finditer(text):
            literal = token.group()
            if literal.startswith(("//", "/*")) or not HAN.search(literal):
                continue
            function = ""
            for declaration in functions:
                if declaration.start() > token.start():
                    break
                function = declaration.group(1)
            if function in LITERAL_DATA.get(relative, set()):
                continue
            line = text.count("\n", 0, token.start()) + 1
            errors.append(f"{relative}:{line}: untranslated source literal")

    if errors:
        raise SystemExit("\n".join(errors))
    print(f"i18n: {len(keys)} messages; complete zh-Hans/en catalogs; source literals checked")


if __name__ == "__main__":
    main()
