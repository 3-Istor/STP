"""Guard against falsely claiming catalogue or verified geometry coverage in the audit."""

import copy
import json
from pathlib import Path
import unittest

from audit_3d_assets import audit


ROOT = Path(__file__).resolve().parents[3]


class AuditInventoryTest(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads((ROOT / 'docs/3d/assets-manifest.json').read_text(encoding='utf-8'))

    def test_seed_and_inventory_agree(self):
        self.assertEqual(audit(self.manifest, ROOT), [])

    def test_rejects_false_coverage_and_identity_claims(self):
        cases = [
            ('duplicate key', lambda m: m['catalogue_types'].append(copy.deepcopy(m['catalogue_types'][0])), 'duplicate catalogue keys'),
            ('missing seed type', lambda m: m['catalogue_types'].pop(), 'seed keys missing'),
            ('unknown seed key', lambda m: m['catalogue_types'][0].update(part_key_source='com.not-a-real-part'), 'absent from seed'),
            ('wrong vehicle', lambda m: m['catalogue_types'][0].update(vehicle_or_group='valhalla'), 'wrong vehicle/group'),
            ('invented status', lambda m: m['catalogue_types'][0].update(geometry_status='photorealistic'), 'unknown geometry status'),
            ('unavailable with asset', lambda m: m['catalogue_types'][0].update(asset_path='fake.glb'), 'must not advertise an asset'),
            ('asset missing', lambda m: m['catalogue_types'][0].update(geometry_status='approximation', asset_path='not-present.glb'), 'geometry file missing'),
            ('asset unlicensed', lambda m: m['catalogue_types'][0].update(geometry_status='schematic', asset_path='docs/3d/tools/audit_3d_assets.py'), 'provenance/license missing'),
            ('unverified exactness', lambda m: m['catalogue_types'][0].update(geometry_status='verified_exact'), 'exact status without verified geometry'),
            ('geometry without evidence', lambda m: m['catalogue_types'][0].update(verified_geometry=True), 'geometry verification has no evidence'),
            ('position without evidence', lambda m: m['catalogue_types'][0].update(verified_position=True), 'position verification has no evidence'),
        ]
        for name, corrupt, expected in cases:
            with self.subTest(name=name):
                manifest = copy.deepcopy(self.manifest)
                corrupt(manifest)
                errors = audit(manifest, ROOT)
                self.assertTrue(any(expected in error for error in errors), errors)


if __name__ == '__main__':
    unittest.main()
