#!/usr/bin/env python3
"""Validate the starter repository's community files and GitHub configuration."""
import json
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

import yaml

ROOT = Path(__file__).resolve().parents[2]


class UniqueLoader(yaml.BaseLoader):
    def construct_mapping(self, node, deep=False):
        result = {}
        for key_node, value_node in node.value:
            key = self.construct_object(key_node, deep=deep)
            if key in result:
                raise ValueError(f'Duplicate YAML key: {key}')
            result[key] = self.construct_object(value_node, deep=deep)
        return result


def validate(root):
    errors = []
    required = ('README.md', 'LICENSE', 'CONTRIBUTING.md', 'CODE_OF_CONDUCT.md',
                'SECURITY.md', 'SUPPORT.md', '.github/CODEOWNERS',
                '.github/pull_request_template.md', '.github/dependabot.yml',
                '.github/release.yml', '.github/labels.json',
                '.github/ISSUE_TEMPLATE/bug_report.yml',
                '.github/ISSUE_TEMPLATE/feature_request.yml',
                '.github/rulesets/open-source.json', '.github/rulesets/ci.json',
                '.github/workflows/repository.yml')
    for name in required:
        if not (root / name).is_file() or not (root / name).read_text().strip():
            errors.append(f'Missing or empty required file: {name}')
    configs = {}
    for path in sorted((root / '.github').rglob('*')):
        if path.suffix not in ('.json', '.yaml', '.yml'):
            continue
        try:
            data = json.loads(path.read_text()) if path.suffix == '.json' else yaml.load(path.read_text(), Loader=UniqueLoader)
            configs[str(path.relative_to(root))] = data
        except (ValueError, yaml.YAMLError) as error:
            errors.append(f'{path.relative_to(root)}: {error}')
    labels = configs.get('.github/labels.json', [])
    names = [label['name'] for label in labels]
    if len(set(name.casefold() for name in names)) != len(names):
        errors.append('Duplicate label names')
    for label in labels:
        if not re.fullmatch('[0-9a-fA-F]{6}', label['color']):
            errors.append(f'Invalid label color: {label["name"]}')
    for name, data in configs.items():
        if name.startswith('.github/ISSUE_TEMPLATE/') and 'body' in data:
            ids = [field['id'] for field in data['body'] if 'id' in field]
            if len(ids) != len(set(ids)):
                errors.append(f'{name}: duplicate issue form field IDs')
            for label in data.get('labels', []):
                if label not in names:
                    errors.append(f'{name}: undefined label {label}')
        if name.startswith('.github/workflows/'):
            if data.get('permissions', {}).get('contents') != 'read':
                errors.append(f'{name}: declare contents: read at workflow level; scope writes to specific jobs')
            for job in data.get('jobs', {}).values():
                for item in [job] + job.get('steps', []):
                    uses = item.get('uses', '')
                    if uses and not uses.startswith('./') and not re.fullmatch(r'[^@]+@[0-9a-f]{40}', uses):
                        errors.append(f'{name}: pin external actions/workflows to a full commit SHA: {uses}')
    ci = configs.get('.github/workflows/repository.yml', {})
    events = ci.get('on', {})
    if not all(event in events for event in ('push', 'pull_request')):
        errors.append('CI must run on pushes and pull requests')
    if any((events.get(event) or {}).get(key) for event in ('push', 'pull_request')
           for key in ('paths', 'paths-ignore')):
        errors.append('Required repository checks must not use workflow path filters')
    if not any(job.get('name') == 'Repository checks' for job in ci.get('jobs', {}).values()):
        errors.append('CI must expose the stable Repository checks job name')
    for category in configs.get('.github/release.yml', {}).get('changelog', {}).get('categories', []):
        for label in category.get('labels', []):
            if label != '*' and label not in names:
                errors.append(f'Release notes reference undefined label: {label}')
    for path in list(root.glob('*.md')) + list((root / 'docs').rglob('*.md')) + list((root / '.github').rglob('*.md')):
        for link in re.findall(r'\[[^\]]*\]\(([^)]+)\)', path.read_text()):
            url = urlsplit(link)
            if url.scheme or url.netloc or not url.path:
                continue
            target = (path.parent / unquote(url.path)).resolve()
            if not target.is_relative_to(root.resolve()) or not target.exists():
                errors.append(f'{path.relative_to(root)}: invalid local link {link}')
    return errors


if __name__ == '__main__':
    failures = validate(ROOT)
    if failures:
        print('\n'.join(failures), file=sys.stderr)
        sys.exit(1)
    print('Repository checks passed: community files, YAML/JSON, labels, CI conventions, action pins, and local documentation links.')
