#!/usr/bin/env python3
"""Check the research inventory against literal catalogue definitions, without a database.

This validates audit data only: a passing check does not mean the 3D feature is complete.
Usage: python3 docs/3d/tools/audit_3d_assets.py [path/to/assets-manifest.json]
"""

import collections
import json
from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[3]
PART_DEFINITION = re.compile(r'^\s*\{"((?:mpl|vlh|com)\.[^"\s]+)",\s*"[^"\n]+",\s*"[\d.]+', re.M)


def audit(manifest, root):
    """Return diagnostics; do not infer runtime IDs or geometry from part names."""
    errors = []
    entries = manifest['catalogue_types']
    keys = [entry['part_key_source'] for entry in entries]
    duplicates = sorted(key for key, count in collections.Counter(keys).items() if count > 1)
    if duplicates:
        errors.append(f'duplicate catalogue keys: {duplicates}')
    expected = {}
    for group, filename in [('multipla', 'multipla'), ('valhalla', 'valhalla'), ('commun', 'common')]:
        definitions = (root / 'internal/sampledata' / (filename + '.go')).read_text(encoding='utf-8')
        expected.update((key, group) for key in PART_DEFINITION.findall(definitions))
    missing = sorted(expected.keys() - set(keys))
    unknown = sorted(set(keys) - expected.keys())
    if missing:
        errors.append(f'seed keys missing from inventory: {missing}')
    if unknown:
        errors.append(f'inventory keys absent from seed: {unknown}')
    for entry in entries:
        key = entry['part_key_source']
        if key in expected and entry['vehicle_or_group'] != expected[key]:
            errors.append(f'{key}: wrong vehicle/group')
        status = entry['geometry_status']
        if status not in {'verified_exact', 'approximation', 'schematic', 'unavailable'}:
            errors.append(f'{key}: unknown geometry status {status!r}')
        if status == 'unavailable' and entry['asset_path'] is not None:
            errors.append(f'{key}: unavailable geometry must not advertise an asset')
        if status != 'unavailable':
            path = entry['asset_path']
            if not path or not (root / path).is_file():
                errors.append(f'{key}: geometry file missing')
            if not entry['source_url'] or not entry['license']:
                errors.append(f'{key}: asset provenance/license missing')
        if status == 'verified_exact' and not entry['verified_geometry']:
            errors.append(f'{key}: exact status without verified geometry')
        if entry['verified_geometry'] and not entry['geometry_evidence_url']:
            errors.append(f'{key}: geometry verification has no evidence')
        if entry['verified_position'] and not entry['position_evidence_url']:
            errors.append(f'{key}: position verification has no evidence')
    return errors


def main():
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / 'docs/3d/assets-manifest.json'
    try:
        manifest = json.loads(path.read_text(encoding='utf-8'))
        errors = audit(manifest, ROOT)
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f'Invalid audit inventory: {error}', file=sys.stderr)
        return 1
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    entries = manifest['catalogue_types']
    print(f'Inventory integrity: OK ({len(entries)} unique catalogue types aligned with seed definitions).')
    print('Groups:', dict(sorted(collections.Counter(e['vehicle_or_group'] for e in entries).items())))
    print('Geometry:', dict(sorted(collections.Counter(e['geometry_status'] for e in entries).items())))
    print('No runtime BOM occurrence coverage or visual acceptance is implied by this audit.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
