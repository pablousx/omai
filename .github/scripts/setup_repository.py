#!/usr/bin/env python3
"""Preview or apply the template's GitHub settings using an authenticated gh CLI."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[2]
SETTINGS = {
    'has_issues': True, 'has_wiki': False, 'has_projects': False,
    'allow_squash_merge': True, 'allow_merge_commit': False,
    'allow_rebase_merge': False, 'delete_branch_on_merge': True,
    'squash_merge_commit_title': 'PR_TITLE',
    'squash_merge_commit_message': 'PR_BODY',
}
WORKFLOW_PERMISSIONS = {
    'default_workflow_permissions': 'read',
    'can_approve_pull_request_reviews': False,
}


class APIError(RuntimeError):
    pass


def api(method, endpoint, data=None, paginate=False):
    command = ['gh', 'api', '--hostname', 'github.com', '--method', method, endpoint]
    if data is not None:
        command += ['--input', '-']
    if paginate:
        command += ['--paginate', '--slurp']
    result = subprocess.run(command, input=json.dumps(data) if data is not None else None,
                            text=True, capture_output=True)
    if result.returncode:
        raise APIError(result.stderr.strip() or result.stdout.strip())
    body = json.loads(result.stdout) if result.stdout.strip() else None
    return [item for page in body for item in page] if paginate else body


def rulesets(maintainer_id, approvals):
    result = []
    for name in ('open-source', 'ci'):
        rule = json.loads((ROOT / '.github/rulesets' / (name + '.json')).read_text())
        rule['bypass_actors'] = [{'actor_id': maintainer_id, 'actor_type': 'User', 'bypass_mode': 'always'}]
        for entry in rule['rules']:
            if entry['type'] == 'pull_request':
                entry['parameters']['required_approving_review_count'] = approvals
        result.append(rule)
    return result


def matching_rule(existing, name):
    matches = [r for r in existing if r['name'] == name and r.get('source_type') == 'Repository']
    if len(matches) > 1:
        raise APIError(f'Multiple repository rulesets named {name!r}; reconcile duplicates before retrying.')
    return matches[0]['id'] if matches else None


def plan_unavailable(error):
    text = str(error).lower()
    return any(message in text for message in (
        'upgrade to github pro', 'upgrade to github team',
        'not available for this repository', 'not supported on this plan',
    ))


def check_ready(prefix, branch, rule):
    sha = api('GET', f'{prefix}/branches/{quote(branch, safe="")}')['commit']['sha']
    pages = api('GET', f'{prefix}/commits/{sha}/check-runs?per_page=100')
    checks = pages['check_runs']
    # The API defaults to the latest run of each name. Require every configured project and repository check.
    if pages.get('total_count', len(checks)) > len(checks):
        raise APIError('More than 100 check runs on the default branch; verify required checks manually.')
    required = rule['rules'][0]['parameters']['required_status_checks']
    return all(any(c['name'] == r['context'] and c['app']['id'] == r['integration_id']
                   and c['status'] == 'completed' and c['conclusion'] == 'success'
                   for c in checks) for r in required)


def configure(repo, maintainer=None, approvals=0, apply=False):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo):
        raise ValueError('Repository must be OWNER/REPO on github.com.')
    prefix = f'repos/{repo}'
    info = api('GET', prefix)
    if info.get('archived') or not info.get('permissions', {}).get('admin'):
        raise APIError('An unarchived repository and repository administration permission are required.')
    if maintainer is None:
        if info['owner']['type'] != 'User':
            raise ValueError('Organization repositories require --maintainer LOGIN.')
        maintainer = info['owner']['login']
    if not re.fullmatch(r'[A-Za-z0-9-]+', maintainer):
        raise ValueError('Maintainer must be a GitHub user login.')
    user = api('GET', f'users/{maintainer}')
    if user['type'] != 'User':
        raise ValueError('The bypass maintainer must be an individual user.')
    # A public user ID alone is not enough: confirm they can administer this repository.
    permission = api('GET', f'{prefix}/collaborators/{maintainer}/permission')
    if permission['permission'] != 'admin':
        raise ValueError('The bypass maintainer must have repository admin permission.')
    baseline, ci = rulesets(user['id'], approvals)
    labels = json.loads((ROOT / '.github/labels.json').read_text())
    print(json.dumps({'repository': repo, 'visibility': info['visibility'],
                      'settings': SETTINGS, 'workflow_permissions': WORKFLOW_PERMISSIONS,
                      'labels': [x['name'] for x in labels], 'rulesets': [baseline, ci],
                      'dependabot_alerts_and_security_updates': True,
                      'public_security_features': not info['private']}, indent=2))
    if not apply:
        print('PREVIEW ONLY: no changes made. Use --apply to configure this repository.')
        return 0
    errors = []

    def attempt(label, operation):
        try:
            operation()
            print(f'APPLIED: {label}')
        except APIError as error:
            errors.append(label)
            print(f'ERROR: {label}: {error}', file=sys.stderr)

    attempt('repository settings', lambda: api('PATCH', prefix, SETTINGS))
    attempt('workflow permissions', lambda: api('PUT', prefix + '/actions/permissions/workflow', WORKFLOW_PERMISSIONS))
    attempt('Dependabot alerts', lambda: api('PUT', prefix + '/vulnerability-alerts'))
    attempt('Dependabot security updates', lambda: api('PUT', prefix + '/automated-security-fixes'))

    def sync_labels():
        existing = {item['name'].casefold(): item['name'] for item in api('GET', prefix + '/labels?per_page=100', paginate=True)}
        for label in labels:
            old = existing.get(label['name'].casefold())
            endpoint = prefix + '/labels' + ('/' + quote(old, safe='') if old else '')
            payload = dict(label)
            if old:
                payload['new_name'] = payload.pop('name')
            api('PATCH' if old else 'POST', endpoint, payload)
    attempt('managed labels', sync_labels)

    if info['private']:
        print('SKIPPED: public vulnerability reporting, secret scanning, and push protection on a private repository.')
    else:
        attempt('private vulnerability reporting', lambda: api('PUT', prefix + '/private-vulnerability-reporting'))
        for feature in ('secret_scanning', 'secret_scanning_push_protection'):
            attempt(feature, lambda feature=feature: api('PATCH', prefix, {
                'security_and_analysis': {feature: {'status': 'enabled'}}}))

    try:
        existing = api('GET', prefix + '/rulesets?per_page=100', paginate=True)
        for rule in (baseline, ci):
            if rule is ci and not check_ready(prefix, info['default_branch'], ci):
                errors.append('required CI pending')
                print('PENDING: Required checks must pass on the current default-branch commit. Rerun setup afterward.', file=sys.stderr)
                continue
            rule_id = matching_rule(existing, rule['name'])
            api('PUT' if rule_id else 'POST', prefix + '/rulesets' + (f'/{rule_id}' if rule_id else ''), rule)
            print(f'APPLIED: {rule["name"]}; Always allow bypass for {maintainer}')
    except APIError as error:
        if info['private'] and plan_unavailable(error):
            print(f'UNAVAILABLE: private-repository rulesets require a qualifying GitHub plan. {error}')
        else:
            errors.append('rulesets')
            print(f'ERROR: rulesets: {error}', file=sys.stderr)
    print('Finished with errors/pending work: ' + ', '.join(errors) if errors else 'Finished. Review any SKIPPED or UNAVAILABLE features above.')
    return 1 if errors else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', required=True, help='Exact OWNER/REPO on github.com')
    parser.add_argument('--maintainer', help='Admin user granted Always allow bypass; defaults to personal repository owner')
    parser.add_argument('--approvals', type=int, choices=range(0, 7), default=0)
    parser.add_argument('--apply', action='store_true', help='Apply the printed configuration; default is read-only preview')
    args = parser.parse_args()
    try:
        return configure(args.repo, args.maintainer, args.approvals, args.apply)
    except (APIError, ValueError, FileNotFoundError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
