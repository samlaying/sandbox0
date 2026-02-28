#!/usr/bin/env python3
"""
Extract code snippets from MDX documentation files.

Usage:
    python3 extract-code.py [doc-name]

Examples:
    python3 extract-code.py              # Extract from all docs
    python3 extract-code.py get-started  # Extract from specific doc
"""

from __future__ import annotations

import os
import re
import shutil
import sys
from pathlib import Path
from typing import Dict, List, Optional, Tuple, Any


# Paths
DOCS_DIR = Path(__file__).parent
OUTPUT_DIR = Path(__file__).parent / "docs-codes"

# Language to file extension mapping
LANG_MAP = {
    "go": ("go", "main.go"),
    "golang": ("go", "main.go"),
    "python": ("py", "main.py"),
    "py": ("py", "main.py"),
    "typescript": ("ts", "main.ts"),
    "ts": ("ts", "main.ts"),
    "bash": ("bash", "main.sh"),
    "shell": ("bash", "main.sh"),
    "cli": ("bash", "main.sh"),
}

# Comment prefixes for different languages
COMMENT_PREFIX = {
    "go": "//",
    "py": "#",
    "ts": "//",
    "bash": "#",
}

# Patterns to identify install/package commands (not real code)
INSTALL_PATTERNS = [
    r'^go get ',
    r'^go install ',
    r'^pip install ',
    r'^pip3 install ',
    r'^npm install ',
    r'^yarn add ',
    r'^pnpm add ',
    r'^bun install ',
    r'^bun add ',
]


def is_install_command(code: str) -> bool:
    """Check if code snippet is just an install command."""
    code_stripped = code.strip()
    for pattern in INSTALL_PATTERNS:
        if re.match(pattern, code_stripped, re.MULTILINE):
            # Check if it's ONLY install commands
            lines = [l.strip() for l in code_stripped.split('\n') if l.strip() and not l.strip().startswith('#')]
            non_install_lines = [l for l in lines if not any(re.match(p, l) for p in INSTALL_PATTERNS)]
            if not non_install_lines:
                return True
    return False


def extract_tabs_code(content: str) -> List[Dict[str, Any]]:
    """Extract code from Tabs components in MDX content."""
    results = []

    # Pattern to match Tabs component with tabs prop
    # Handle multi-line tabs array
    pattern = r'<Tabs\s+tabs=\{\[\s*([\s\S]*?)\]\}\s*/?>'

    for match in re.finditer(pattern, content):
        tabs_content = match.group(1)

        # Extract individual tab objects
        tab_pattern = r'\{\s*label:\s*["\']([^"\']+)["\']\s*,\s*language:\s*["\']([^"\']+)["\']\s*,\s*code:\s`([^`]*)`\s*\}'

        for tab_match in re.finditer(tab_pattern, tabs_content, re.DOTALL):
            label = tab_match.group(1)
            language = tab_match.group(2).lower()
            code = tab_match.group(3)

            if language in LANG_MAP:
                # Skip install-only commands
                if is_install_command(code):
                    continue

                results.append({
                    "label": label,
                    "language": language,
                    "code": code,
                    "ext_dir": LANG_MAP[language][0],
                    "filename": LANG_MAP[language][1],
                })

    return results


def get_doc_files(doc_name: Optional[str] = None) -> List[Path]:
    """Get list of MDX files to process."""
    if doc_name:
        # Find specific doc file
        doc_path = DOCS_DIR / doc_name
        if doc_path.is_file() and doc_path.suffix == ".mdx":
            return [doc_path]
        elif doc_path.is_dir():
            return list(doc_path.rglob("*.mdx"))
        else:
            # Try adding .mdx extension
            doc_path_mdx = DOCS_DIR / f"{doc_name}.mdx"
            if doc_path_mdx.is_file():
                return [doc_path_mdx]
            # Try as page.mdx in subdirectory
            doc_page = DOCS_DIR / doc_name / "page.mdx"
            if doc_page.is_file():
                return [doc_page]
            print(f"Warning: Document '{doc_name}' not found")
            return []
    else:
        return list(DOCS_DIR.rglob("*.mdx"))


def get_output_subpath(doc_file: Path) -> Path:
    """Get the output subdirectory path for a doc file."""
    rel_path = doc_file.relative_to(DOCS_DIR)
    # Remove 'page.mdx' suffix, use parent directory name or file stem
    if rel_path.name == "page.mdx":
        parts = rel_path.parts[:-1]  # Remove 'page.mdx'
        if not parts:
            return Path("index")
        return Path(*parts)
    else:
        return Path(rel_path.stem)


def process_doc(doc_file: Path) -> Dict[str, List[Dict[str, Any]]]:
    """Process a single doc file and return extracted code by language."""
    content = doc_file.read_text()
    tabs = extract_tabs_code(content)

    # Group by language
    by_lang: Dict[str, List[Dict[str, Any]]] = {}
    for tab in tabs:
        lang_key = tab["ext_dir"]
        if lang_key not in by_lang:
            by_lang[lang_key] = []
        by_lang[lang_key].append(tab)

    return by_lang


def wrap_go_code(code: str) -> str:
    """Wrap Go code with package declaration if needed."""
    # Check if code starts with a package declaration
    code_stripped = code.lstrip()
    if code_stripped.startswith('package '):
        return code
    # Wrap in a dummy package for syntax checking
    return f"package main\n\n{code}"


def write_code_files(doc_subpath: Path, code_by_lang: Dict[str, List[Dict[str, Any]]]) -> List[Path]:
    """Write extracted code to output files."""
    written_files = []

    for lang_key, tabs in code_by_lang.items():
        # Clean the output directory first
        lang_dir = OUTPUT_DIR / doc_subpath / lang_key
        if lang_dir.exists():
            shutil.rmtree(lang_dir)
        lang_dir.mkdir(parents=True, exist_ok=True)

        # Get comment prefix for this language
        comment = COMMENT_PREFIX.get(lang_key, "//")

        # Combine code from multiple tabs (if any)
        combined_code = ""
        for i, tab in enumerate(tabs):
            if i > 0:
                combined_code += f"\n\n{comment} --- Additional snippet: {tab['label']} ---\n\n"
            combined_code += tab["code"]

        # Wrap Go code with package declaration if needed
        if lang_key == "go":
            combined_code = wrap_go_code(combined_code)

        output_file = lang_dir / tabs[0]["filename"]
        output_file.write_text(combined_code)
        written_files.append(output_file)
        print(f"  Written: {output_file.relative_to(DOCS_DIR)}")

    return written_files


def main():
    doc_name = sys.argv[1] if len(sys.argv) > 1 else None

    print(f"Docs directory: {DOCS_DIR}")
    print(f"Output directory: {OUTPUT_DIR}")
    print()

    doc_files = get_doc_files(doc_name)
    if not doc_files:
        print("No documentation files found")
        sys.exit(1)

    print(f"Found {len(doc_files)} documentation file(s)")
    print()

    all_written = []
    for doc_file in doc_files:
        print(f"Processing: {doc_file.relative_to(DOCS_DIR)}")

        doc_subpath = get_output_subpath(doc_file)
        code_by_lang = process_doc(doc_file)

        if code_by_lang:
            written = write_code_files(doc_subpath, code_by_lang)
            all_written.extend(written)
        else:
            print("  No code snippets found")
        print()

    print(f"Total files written: {len(all_written)}")


if __name__ == "__main__":
    main()
