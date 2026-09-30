#!/usr/bin/env python3
"""Build the allowlisted source-free clinic partner kit; never copies a clinic solution."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil

REPO = Path(__file__).resolve().parents[2]
TEMPLATES = Path(__file__).resolve().parent / 'kit'


def build(destination, deny, guidance=None):
    destination = destination.resolve()
    if destination.exists():
        raise ValueError('destination must not exist (never overwrite a delivery)')
    denied = [REPO, Path.home() / '.codex', *deny]
    if any(destination == p.resolve() or p.resolve() in destination.parents for p in denied):
        raise ValueError('kit must be outside denied trees')
    destination.mkdir(parents=True)
    for folder in ('bin', 'api', 'docs/guides', 'authored', 'evidence',
                   '.claude/skills/tessera-author'):
        (destination / folder).mkdir(parents=True, exist_ok=True)
    files = {
        REPO / 'bin/tessera': 'bin/tessera-cli',
        TEMPLATES / 'launcher.py': 'bin/tessera',
        TEMPLATES / 'BRIEF.md': 'BRIEF.md',
        REPO / 'bench/market/toolserver/openapi.json': 'api/clinic.openapi.json',
        REPO / '.claude/skills/tessera-author/SKILL.md': '.claude/skills/tessera-author/SKILL.md',
    }
    for name in ('authoring', 'testing', 'validation', 'expressions', 'partner-testing'):
        files[REPO / f'docs/guides/{name}.md'] = f'docs/guides/{name}.md'
    for name in ('traveler', 'rede', 'sdr'):
        for source in (REPO / 'examples' / name).rglob('*'):
            if source.is_file() and source.suffix in ('.yaml', '.yml', '.star', '.md'):
                files[source] = str(source.relative_to(REPO))
    for source, target in files.items():
        target = destination / target
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    (destination / 'bin/tessera').chmod(0o755)
    instructions = (TEMPLATES / 'AGENTS.md').read_text().replace('@KIT@', str(destination))
    if guidance:
        shutil.copy2(guidance, destination / 'docs/guides/conversation-delivery.md')
        instructions += '\nRead docs/guides/conversation-delivery.md as part of the delivery checklist.\n'
    (destination / 'AGENTS.md').write_text(instructions)
    profile = '(version 1)\n(allow default)\n'
    for path in denied:
        for operation in ('file-read*', 'file-write*'):
            profile += f'(deny {operation} (subpath {json.dumps(str(path.resolve()))}))\n'
    (destination / 'partner.sb').write_text(profile)
    exclusions = []
    for doc in (destination / 'docs').rglob('*.md'):
        def link(match):
            target = match[2].split('#')[0]
            if target and not re.match(r'[a-z]+:', target) and not (doc.parent / target).exists():
                exclusions.append({'document': str(doc.relative_to(destination)), 'target': target})
                return match[1] + ' (background reference unavailable in this distribution)'
            return match[0]
        doc.write_text(re.sub(r'\[([^\]]+)\]\(([^)]+)\)', link, doc.read_text()))
    (destination / 'excluded-citations.json').write_text(json.dumps(exclusions, indent=2) + '\n')
    manifest = {str(p.relative_to(destination)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in sorted(destination.rglob('*')) if p.is_file()}
    (destination / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    return manifest


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('destination', type=Path)
    parser.add_argument('--deny', type=Path, action='append', default=[],
                        help='additional previous/parallel partner kit to deny (repeatable)')
    parser.add_argument('--guidance', type=Path, help='optional experimental conversation guidance')
    args = parser.parse_args()
    manifest = build(args.destination, args.deny, args.guidance)
    print(f'{len(manifest)} files packaged; binary hash {manifest["bin/tessera-cli"]}')
