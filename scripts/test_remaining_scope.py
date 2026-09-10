import json
from pathlib import Path
import tempfile
import unittest
from check_remaining_scope import artifact, digest, event_audit, merge_profiles


class RemainingScopeTests(unittest.TestCase):
    def test_artifact_tampering_and_escape_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);p=root/'result';p.write_text('actual evidence')
            records={'result':{'sha256':digest(p)}}
            self.assertEqual(artifact(root,records,'result'),p.resolve())
            p.write_text('replacement')
            with self.assertRaisesRegex(ValueError,'changed'):artifact(root,records,'result')
            with self.assertRaisesRegex(ValueError,'unsafe'):artifact(root,{'../escape':{'sha256':'a'*64}},'../escape')

    def test_profile_merge_preserves_uncovered_blocks_and_rejects_conflicts(self):
        a='mode: atomic\nx.go:1.1,2.1 2 0\nx.go:3.1,4.1 1 1\n'
        b='mode: atomic\nx.go:1.1,2.1 2 1\ny.go:1.1,2.1 3 0\n'
        merged=merge_profiles([a,b])
        self.assertIn('x.go:1.1,2.1 2 1',merged);self.assertIn('y.go:1.1,2.1 3 0',merged)
        with self.assertRaisesRegex(ValueError,'inconsistent'):merge_profiles([a,b.replace('2 1','4 1')])
        with self.assertRaisesRegex(ValueError,'atomic'):merge_profiles(['mode: set\n'])

    def test_missing_inventory_and_skipped_tests_cannot_qualify(self):
        # This exercises inventory reconciliation independently of the separately
        # tested strict Go stream parser. A reported pass alone is insufficient.
        events=[dict(Action='start',Package='p'),dict(Action='run',Package='p',Test='TestOne'),
                dict(Action='pass',Package='p',Test='TestOne'),dict(Action='pass',Package='p')]
        raw='\n'.join(map(json.dumps,events))
        inventory=json.dumps(dict(Package='p',Output='TestOne\nTestTwo\n'))
        strict=lambda _:dict(status='passed',errors=[])
        with self.assertRaisesRegex(ValueError,'inventory'):event_audit(raw,inventory,'p',strict)
        inventory=json.dumps(dict(Package='p',Output='TestOne\n'))
        self.assertEqual(event_audit(raw,inventory,'p',strict)[0],{'TestOne'})
        events[2]['Action']='skip'
        with self.assertRaisesRegex(ValueError,'inventory'):event_audit('\n'.join(map(json.dumps,events)),inventory,'p',strict)
        with self.assertRaisesRegex(ValueError,'package'):event_audit(raw,inventory,'p\nq',strict)


if __name__=='__main__':unittest.main()
