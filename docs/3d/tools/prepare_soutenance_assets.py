#!/usr/bin/env python3
"""Prepare disjoint, explicitly bound copies. Original user GLBs are never overwritten.

Inputs: inspected matrices and the static Valhalla pose from bake_valhalla_pose.mjs.
Runtime picking reads the resulting numeric occurrence manifests, never these source names.
"""
import argparse
import base64
import copy
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import struct

ROOT = Path(__file__).resolve().parents[3]
MODELS = ROOT / 'web/public/models'


def read_glb(path):
    data = path.read_bytes()
    assert struct.unpack_from('<4sII', data) == (b'glTF', 2, len(data))
    length = struct.unpack_from('<I', data, 12)[0]
    document = json.loads(data[20:20 + length])
    offset = 20 + length
    binary_length, kind = struct.unpack_from('<II', data, offset)
    assert kind == 0x004e4942
    return document, bytearray(data[offset + 8:offset + 8 + binary_length])


def write_glb(path, document, binary):
    binary += b'\0' * (-len(binary) % 4)
    document['buffers'][0]['byteLength'] = len(binary)
    encoded = json.dumps(document, separators=(',', ':'), ensure_ascii=True).encode()
    encoded += b' ' * (-len(encoded) % 4)
    data = struct.pack('<4sIIII', b'glTF', 2, 28 + len(encoded) + len(binary), len(encoded), 0x4e4f534a)
    data += encoded + struct.pack('<II', len(binary), 0x004e4942) + binary
    path.write_bytes(data)
    return hashlib.sha256(data).hexdigest()


def append_attribute(document, binary, encoded, dimension):
    data = base64.b64decode(encoded)
    binary += b'\0' * (-len(binary) % 4)
    view = len(document['bufferViews'])
    document['bufferViews'].append(dict(buffer=0, byteOffset=len(binary), byteLength=len(data)))
    binary += data
    accessor = dict(bufferView=view, componentType=5126, count=len(data) // (4 * dimension), type='VEC' + str(dimension))
    if dimension == 3:
        values = struct.unpack('<' + 'f' * (len(data) // 4), data)
        accessor['min'] = [min(values[axis::3]) for axis in range(3)]
        accessor['max'] = [max(values[axis::3]) for axis in range(3)]
    document['accessors'].append(accessor)
    return len(document['accessors']) - 1


def battery_variant(document, binary, nodes, children, variant):
    """An explicitly illustrative casing variant; no capacity-to-dimension business rule."""
    positions = []
    for child in children:
        primitive = document['meshes'][nodes[child]['mesh']]['primitives'][0]
        accessor = document['accessors'][primitive['attributes']['POSITION']]
        view = document['bufferViews'][accessor['bufferView']]
        assert accessor['componentType'] == 5126 and accessor['type'] == 'VEC3'
        stride = view.get('byteStride', 12)
        offset = view.get('byteOffset', 0) + accessor.get('byteOffset', 0)
        positions.append([struct.unpack_from('<3f', binary, offset + i * stride) for i in range(accessor['count'])])
    center = [(min(p[axis] for mesh in positions for p in mesh) + max(p[axis] for mesh in positions for p in mesh)) / 2 for axis in range(3)]
    copies = []
    for child, vertices in zip(children, positions):
        node = copy.deepcopy(nodes[child]); mesh = copy.deepcopy(document['meshes'][node['mesh']])
        assert len(mesh['primitives']) == 1 and 'NORMAL' not in mesh['primitives'][0]['attributes']
        values = [center[axis] + (point[axis] - center[axis]) * variant['illustrative_scale'][axis] for point in vertices for axis in range(3)]
        data = struct.pack('<' + 'f' * len(values), *values)
        mesh['primitives'][0]['attributes']['POSITION'] = append_attribute(document, binary, base64.b64encode(data), 3)
        node['mesh'] = len(document['meshes']); document['meshes'].append(mesh)
        node['name'] += '-illustrative-60ah'; copies.append(len(nodes)); nodes.append(node)
    index = len(nodes)
    nodes.append(dict(name=variant['source_group'], children=copies, extras=dict(accuracy=variant['note'])))
    return index


def prepare(vehicle, layout, inspection, exterior):
    name = ('Multipla' if vehicle == 'multipla' else 'Valhalla')
    original = name + ('1998' if vehicle == 'multipla' else '2025') if exterior else name + 'Interior'
    document, binary = read_glb(MODELS / (original + '.glb'))
    original_nodes = document['nodes']
    entries = layout['exterior' if exterior else 'interior']
    source_by_name = {n.get('name'): i for i, n in enumerate(original_nodes)}
    quantified = {} if exterior else {name: case for case in layout.get('quantified_occurrences', []) for name in case['source_nodes']}
    for case in ([] if exterior else layout.get('quantified_occurrences', [])):
        vertices, centers = [], []
        for source_name in case['source_nodes']:
            node = original_nodes[source_by_name[source_name]]
            assert not any(field in node for field in ['matrix', 'rotation', 'scale', 'translation'])
            primitive = document['meshes'][node['mesh']]['primitives'][0]
            accessor = document['accessors'][primitive['attributes']['POSITION']]
            view = document['bufferViews'][accessor['bufferView']]
            assert accessor['componentType'] == 5126
            points = [struct.unpack_from('<3f', binary, view.get('byteOffset', 0) + accessor.get('byteOffset', 0) + i * view.get('byteStride', 12)) for i in range(accessor['count'])]
            vertices.append(points); centers.append([(a + b) / 2 for a, b in zip(accessor['min'], accessor['max'])])
        assert all(len(points) == len(vertices[0]) for points in vertices)
        error = max(abs((p[axis] - c[axis]) - (q[axis] - centers[0][axis])) for c, points in zip(centers, vertices) for p, q in zip(points, vertices[0]) for axis in range(3))
        assert error <= case['geometry_tolerance'], 'Meshes are not translated copies: ' + str(case['node_id'])
        case['deltas'] = [[c[axis] - centers[0][axis] for axis in range(3)] for c in centers]
    matrices, poses = {}, {}
    if exterior and vehicle == 'multipla':
        for n in json.loads((inspection / 'Multipla1998-measured.json').read_text())['nodes']:
            if n.get('mesh'): matrices[n['name']] = n['matrix']
    if exterior and vehicle == 'valhalla':
        for p in json.loads((inspection / 'valhalla-pose.json').read_text())['meshes']:
            matrices[p['nodes']] = p['matrix']; poses[p['nodes']] = p
    # Grouping changes hierarchy only; geometry bytes, UVs and embedded textures remain intact.
    grouped, records = defaultdict(list), {}
    nodes = []
    for entry in entries:
        source_index = entry.get('source_node_index') if exterior else source_by_name[entry['source_node']]
        source = original_nodes[source_index]
        node = dict(source)
        node.pop('skin', None)
        node.pop('children', None)
        node['name'] = 'mesh-' + str(source_index)
        if exterior:
            for field in ('translation', 'rotation', 'scale'): node.pop(field, None)
            node['matrix'] = matrices[source_index if vehicle == 'valhalla' else source['name']]
            if vehicle == 'multipla':
                material_index = document['meshes'][source['mesh']]['primitives'][0].get('material')
                if material_index is not None and document['materials'][material_index].get('name') == 'car_interior':
                    node.setdefault('extras', {})['stp_native_interior'] = True
            if vehicle == 'valhalla':
                pose = poses[source_index]
                primitive = document['meshes'][pose['meshes']]['primitives'][pose['primitives']]
                for attr, key, width in [('POSITION', 'position', 3), ('NORMAL', 'normal', 3), ('TANGENT', 'tangent', 4)]:
                    if pose[key]: primitive['attributes'][attr] = append_attribute(document, binary, pose[key], width)
                for attr in ('JOINTS_0', 'WEIGHTS_0'): primitive['attributes'].pop(attr, None)
        else:
            # Deliberately illustrative metal/rubber upholstery, keeping source vertex colours.
            label = source['name'].lower()
            soft = any(token in label for token in ('pneu', 'mousse', 'siege', 'ceinture', 'durite', 'cable', 'soufflet'))
            materials = document.setdefault('materials', [])
            for primitive in document['meshes'][source['mesh']]['primitives']:
                color = primitive['attributes'].get('COLOR_0')
                factor = [1, 1, 1, 1]
                if color is not None:
                    accessor = document['accessors'][color]
                    if accessor.get('min') == accessor.get('max') and accessor['componentType'] == 5121:
                        rgba = accessor['min']
                        factor = [v / 255 / 12.92 if v / 255 <= 0.04045 else ((v / 255 + 0.055) / 1.055) ** 2.4 for v in rgba[:3]] + [rgba[3] / 255]
                        primitive['attributes'].pop('COLOR_0')
                material = dict(name='Illustrative soft' if soft else 'Illustrative metal', pbrMetallicRoughness=dict(baseColorFactor=factor, metallicFactor=0 if soft else 0.5, roughnessFactor=0.85 if soft else 0.4))
                if material not in materials: materials.append(material)
                primitive['material'] = materials.index(material)
        target = entry['node_id']
        case = quantified.get(entry.get('source_node'))
        if case and entry['source_node'] != case['source_nodes'][0]:
            continue  # the second source position is reproduced by an explicit instance, not duplicated
        grouped[target].append(len(nodes)); records.setdefault(target, entry); nodes.append(node)
    roots = []
    for target, children in grouped.items():
        roots.append(len(nodes)); nodes.append(dict(name='occurrence-' + str(target), children=children))
    if not exterior:
        for variant in layout.get('variants', []):
            roots.append(battery_variant(document, binary, nodes, grouped[variant['node_id']], variant))
    document['nodes'] = nodes
    document['scenes'] = [dict(name=name + (' exterior' if exterior else ' illustrative interior'), nodes=roots)]
    document['scene'] = 0
    document.pop('skins', None); document.pop('animations', None)
    document['asset'].setdefault('extras', {})['stp_modification'] = 'Disjoint numeric occurrence groups; displayed pose frozen; originals preserved. No OEM certification.'
    filename = name + ('ExteriorPrepared' if exterior else 'InteriorPrepared') + '.glb'
    digest = write_glb(MODELS / filename, document, binary)
    return filename, digest, records


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('inspection', type=Path)
    args = parser.parse_args()
    layouts = json.loads((ROOT / 'docs/3d/soutenance-assembly-layout.json').read_text())
    report = []
    for vehicle, layout in layouts.items():
        manifest = json.loads((MODELS / 'manifests' / f'draft-{layout["bom_id"]}.json').read_text())
        exterior_definition = manifest['assets'][0]
        definitions, bindings = [], []
        for exterior in (True, False):
            filename, digest, records = prepare(vehicle, layout, args.inspection, exterior)
            definition = dict(exterior_definition)
            definition.update(key=filename[:-4], path='/models/' + filename, sha256=digest)
            definition['display_role'] = 'exterior' if exterior else 'interior'
            if not exterior:
                definition.update(unit_scale=1, source_url='http://127.0.0.1:41769/models/soutenance-credits.html',
                                  license='Création originale fournie pour STP — usage local autorisé, licence publique non spécifiée',
                                  license_url='http://127.0.0.1:41769/models/soutenance-credits.html', author='Pack illustratif fourni par l’utilisateur', geometry_status='schematic')
            definitions.append(definition)
            for target, entry in records.items():
                if exterior:
                    placement = dict(manifest['bindings'][0]['instances'][0])
                    placement['key'] = 'exterior'
                else:
                    scale = [1, 0.92, 1]
                    if vehicle == 'multipla':
                        scale[2] = {'engine': 0.74, 'gearbox': 0.9, 'cabin': 1.1, 'chassis': 0.95, 'cooling': 0.78}.get(entry['group'], 1)
                    else: scale = [0.92, 0.9, 0.82]
                    placement = dict(key='illustration', position=[0, -0.08 if vehicle == 'valhalla' else 0, 0], rotation=[-1.5707963267948966, 0, 3.141592653589793], scale=scale, explode_vector=[0, 0, 0])
                # At this checkpoint, no explode offset: validate the assembled integration first.
                placement['explode_vector'] = [0, 0, 0]
                case = next((c for c in layout.get('quantified_occurrences', []) if c['node_id'] == target), None) if not exterior else None
                instances = [placement]
                if case:
                    instances = []
                    for i, delta in enumerate(case['deltas']):
                        scaled = [value * placement['scale'][axis] for axis, value in enumerate(delta)]
                        # Calibrated source axes: +X -> -X, +Y -> +Z, +Z -> +Y.
                        offset = [-scaled[0], scaled[2], scaled[1]]
                        instances.append(dict(placement, key='instance-' + str(i + 1), position=[v + offset[axis] for axis, v in enumerate(placement['position'])]))
                bindings.append(dict(node_id=target, parent_id=entry['parent_id'], part_id=entry['part_id'], asset_key=definition['key'],
                                     mesh_path=['occurrence-' + str(target)], system=entry['system'], position_status='approximation', instances=instances))
            if not exterior:
                for variant in layout.get('variants', []):
                    original = next(binding for binding in bindings if binding['node_id'] == variant['node_id'] and binding['part_id'] == variant['original_part_id'])
                    bindings.append(dict(original, part_id=variant['part_id'], mesh_path=[variant['source_group']]))
            report.append(dict(file=filename, sha256=digest, bytes=(MODELS / filename).stat().st_size, occurrence_groups=len(records)))
        manifest.update(revision='soutenance-assembled-20261118', assets=definitions, bindings=bindings, assemblies=[])
        (MODELS / 'manifests' / f'draft-{layout["bom_id"]}.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    (ROOT / 'docs/3d/soutenance-prepared-assets.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
