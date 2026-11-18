"""Refresh assets for the already-verified draft B bindings; never infer new runtime IDs."""
from pathlib import Path
import json

root = Path(__file__).resolve().parents[3] / 'web/public/models/manifests'
source = json.loads((root / 'draft-1.json').read_text())
previous = json.loads((root / 'draft-3.json').read_text())
known = {(b['asset_key'], tuple(b['mesh_path'])): b for b in previous['bindings']}
identities = {}
for binding in source['bindings']:
    match = known.get((binding['asset_key'], tuple(binding['mesh_path'])))
    if match:
        identity = (match['node_id'], match['parent_id'])
        assert identities.get(binding['node_id'], identity) == identity
        identities[binding['node_id']] = identity
bindings = []
for binding in source['bindings']:
    if binding['node_id'] not in identities:
        raise ValueError('No verified draft B identity for source occurrence ' + str(binding['node_id']))
    node, parent = identities[binding['node_id']]
    bindings.append(dict(binding, node_id=node, parent_id=parent))
result = dict(source, scope=dict(kind='draft', id=3), revision=source['revision'] + '-draft-b', bindings=bindings)
(root / 'draft-3.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
print('Draft B:', len(bindings), 'explicit bindings refreshed; runtime reconciliation still required')
