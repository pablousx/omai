import contextlib
import copy
import io
import unittest
from unittest.mock import patch

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import setup_repository as setup


class FakeGitHub:
    def __init__(self, private=False, ready=True):
        self.calls = []
        self.private = private
        self.ready = ready
        self.rules = []
        self.labels = [{'name': 'unrelated-label'}, {'name': 'Bug'}]
        self.rules_available = True
        self.security_fails = False
        self.owner_type = 'User'

    def __call__(self, method, endpoint, data=None, paginate=False):
        self.calls.append((method, endpoint, copy.deepcopy(data)))
        prefix = 'repos/example/project'
        if method == 'GET':
            if endpoint == prefix:
                return {'owner': {'type': self.owner_type, 'login': 'example'},
                        'private': self.private, 'visibility': 'private' if self.private else 'public',
                        'default_branch': 'trunk', 'permissions': {'admin': True}, 'archived': False}
            if endpoint == 'users/example':
                return {'id': 123, 'type': 'User'}
            if endpoint.endswith('/collaborators/example/permission'):
                return {'permission': 'admin'}
            if '/labels?' in endpoint:
                return copy.deepcopy(self.labels)
            if '/rulesets?' in endpoint:
                if not self.rules_available:
                    raise setup.APIError('Upgrade to GitHub Pro to enable this feature. (HTTP 403)')
                return copy.deepcopy(self.rules)
            if endpoint.endswith('/branches/trunk'):
                return {'commit': {'sha': 'abc123'}}
            if '/check-runs?' in endpoint:
                return {'check_runs': [{'name': entry['context'], 'app': {'id': 15368},
                                        'status': 'completed', 'conclusion': 'success' if self.ready else 'failure'}
                                       for entry in setup.rulesets(123, 0)[1]['rules'][0]['parameters']['required_status_checks']]}
            raise AssertionError(f'Unexpected GET {endpoint}')
        if self.security_fails and endpoint.endswith('/vulnerability-alerts'):
            raise setup.APIError('Resource not accessible by integration (HTTP 403)')
        if endpoint.endswith('/rulesets'):
            self.rules.append({'id': len(self.rules) + 1, 'name': data['name'], 'source_type': 'Repository'})
        return None


class SetupTest(unittest.TestCase):
    def run_setup(self, fake, **kwargs):
        with patch.object(setup, 'api', fake), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return setup.configure('example/project', **kwargs)

    def test_preview_never_writes(self):
        fake = FakeGitHub()
        self.assertEqual(self.run_setup(fake), 0)
        self.assertTrue(all(method == 'GET' for method, _, _ in fake.calls))

    def test_rerun_updates_rules_and_preserves_unrelated_settings(self):
        fake = FakeGitHub()
        self.assertEqual(self.run_setup(fake, apply=True), 0)
        self.assertEqual(self.run_setup(fake, apply=True), 0)
        self.assertEqual(len(fake.rules), 2)
        rule_updates = [(method, data) for method, endpoint, data in fake.calls if data and 'bypass_actors' in data]
        self.assertEqual([method for method, _ in rule_updates], ['POST', 'POST', 'PUT', 'PUT'])
        for _, data in rule_updates:
            self.assertEqual(data['bypass_actors'], [{'actor_id': 123, 'actor_type': 'User', 'bypass_mode': 'always'}])
        self.assertFalse(any(method == 'DELETE' for method, _, _ in fake.calls))
        self.assertFalse(any(data and ('visibility' in data or 'private' in data or 'is_template' in data)
                             for _, _, data in fake.calls))
        self.assertTrue(any(method == 'PATCH' and endpoint.endswith('/labels/Bug') for method, endpoint, _ in fake.calls))

    def test_failing_ci_does_not_enable_required_checks(self):
        fake = FakeGitHub(ready=False)
        self.assertEqual(self.run_setup(fake, apply=True), 1)
        self.assertEqual([rule['name'] for rule in fake.rules], ['Open source baseline'])

    def test_private_plan_limit_is_reported_without_visibility_change(self):
        fake = FakeGitHub(private=True)
        fake.rules_available = False
        self.assertEqual(self.run_setup(fake, apply=True), 0)
        self.assertFalse(any('private-vulnerability-reporting' in endpoint for _, endpoint, _ in fake.calls))
        self.assertFalse(any(data and 'security_and_analysis' in data for _, _, data in fake.calls))
        self.assertTrue(any(endpoint.endswith('/automated-security-fixes') for _, endpoint, _ in fake.calls))

    def test_permissions_failure_is_not_treated_as_plan_limit(self):
        fake = FakeGitHub()
        fake.security_fails = True
        self.assertEqual(self.run_setup(fake, apply=True), 1)
        self.assertFalse(setup.plan_unavailable(setup.APIError('Resource not accessible by integration (HTTP 403)')))

    def test_organization_requires_explicit_maintainer_before_writes(self):
        fake = FakeGitHub()
        fake.owner_type = 'Organization'
        with self.assertRaisesRegex(ValueError, 'require --maintainer'):
            self.run_setup(fake, apply=True)
        self.assertTrue(all(method == 'GET' for method, _, _ in fake.calls))

    def test_duplicate_managed_rules_are_rejected(self):
        with self.assertRaises(setup.APIError):
            setup.matching_rule([{'id': 1, 'name': 'same', 'source_type': 'Repository'},
                                 {'id': 2, 'name': 'same', 'source_type': 'Repository'}], 'same')

    def test_rules_are_resolved_for_selected_maintainer_and_reviews(self):
        baseline, ci = setup.rulesets(999, 1)
        review = next(rule for rule in baseline['rules'] if rule['type'] == 'pull_request')
        self.assertEqual(review['parameters']['required_approving_review_count'], 1)
        self.assertEqual(ci['bypass_actors'][0]['actor_id'], 999)

    def test_invalid_repo_rejected_before_api(self):
        with patch.object(setup, 'api') as api, self.assertRaises(ValueError):
            setup.configure('../wrong/repository', apply=True)
        api.assert_not_called()
