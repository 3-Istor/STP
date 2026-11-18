#!/usr/bin/env python3
"""Install only the supplied, fingerprinted GLBs. Never execute scripts from the ZIPs."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import struct
from zipfile import ZipFile

ROOT = Path(__file__).resolve().parents[3]


def validate(data, expected):
    if hashlib.sha256(data).hexdigest() != expected:
        raise ValueError('Asset fingerprint differs from the inspected file')
    if len(data) < 20 or struct.unpack_from('<4sII', data) != (b'glTF', 2, len(data)):
        raise ValueError('Invalid GLB header')
    document = json.loads(data[20:20 + struct.unpack_from('<I', data, 12)[0]])
    if any('uri' in resource for key in ('buffers', 'images') for resource in document.get(key, [])):
        raise ValueError('External resources are not covered by the fingerprint')


def read_asset(name, downloads):
    if name == 'Multipla1998.glb':
        return (downloads / 'fiat_multipla_1998_3d_model.glb').read_bytes()
    if name == 'Valhalla2025.glb':
        with ZipFile(downloads / '2025-aston-martin-valhalla.zip') as archive:
            return archive.read('source/2025_aston_martin_valhalla.glb')
    if name == 'MultiplaInterior.glb':
        with ZipFile(downloads / 'STP-Fiat-Multipla-Interieur-3D.zip') as archive:
            return archive.read('STP-Fiat-Multipla-Interieur-3D/models/multipla_interieur_technique.glb')
    with ZipFile(downloads / 'STP-Valhalla-Pack-Reset.zip') as archive:
        nested = archive.read('STP-Valhalla-Pack-Reset/STP-Valhalla-Interieur-3D.zip')
    with ZipFile(io.BytesIO(nested)) as archive:
        return archive.read('models/valhalla_interieur_hybride.glb')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--downloads', type=Path, default=Path.home() / 'Downloads')
    parser.add_argument('--verify', action='store_true', help='Only verify installed files')
    args = parser.parse_args()
    destination = ROOT / 'web/public/models'
    destination.mkdir(parents=True, exist_ok=True)
    assets = json.loads((ROOT / 'docs/3d/soutenance-assets.json').read_text())
    for asset in assets:
        target = destination / asset['file']
        if target.exists() or args.verify:
            validate(target.read_bytes(), asset['sha256'])
        else:
            data = read_asset(asset['file'], args.downloads)
            validate(data, asset['sha256'])
            target.write_bytes(data)
        print(asset['file'], 'verified', asset['sha256'])


if __name__ == '__main__':
    main()
