"""Apply inspected mechanical group offsets to the explicit local manifests."""
from pathlib import Path
import json
root=Path(__file__).resolve().parents[3];layouts=json.loads((root/'docs/3d/soutenance-assembly-layout.json').read_text())
interior_vectors={'multipla':{'chassis':[0,.08,0],'cabin':[.15,.55,0],'engine':[-.95,.45,.65],'gearbox':[-.55,.30,-.85],'suspension':[0,.12,0],'brakes':[0,.15,.12],'cooling':[-1.0,.30,-.15],'fuel':[.65,.18,-.3]},'valhalla':{'chassis':[0,.05,0],'v8':[.75,.45,.55],'front_edu':[-.6,.35,0],'dct':[1.05,.35,-.5],'battery':[0,.65,-.8],'cooling':[-1.0,.25,0],'suspension':[0,.10,0]}}
wheels={15863:[-.25,0,.70],15870:[-.25,0,-.70],15877:[.25,0,.70],15884:[.25,0,-.70],36135:[-.25,0,.70],36140:[-.25,0,-.70],36145:[.25,0,.70],36150:[.25,0,-.70]}
for vehicle,layout in layouts.items():
 ext={e['node_id']:e for e in layout['exterior']};inner={e['node_id']:e for e in layout['interior']}
 p=root/'web/public/models/manifests'/f'draft-{layout["bom_id"]}.json';m=json.loads(p.read_text())
 for binding in m['bindings']:
  is_ext=binding['asset_key'].endswith('ExteriorPrepared');entry=(ext if is_ext else inner)[binding['node_id']]
  if is_ext:
   group=entry['group'];target=binding['node_id'];vector=[0,2.05 if vehicle=='multipla' else 1.75,0]
   wheel_id=target if target in wheels else binding['parent_id']
   if group.startswith('wheel') and wheel_id in wheels:vector=wheels[wheel_id]
   elif group=='door-left':vector=[0,.75,1.20]
   elif group=='door-right':vector=[0,.75,-1.20]
   elif group=='engine-cover':vector=[.45,1.0,0]
   elif group=='spoiler':vector=[.65,1.15,0]
   elif group=='seat-driver':vector=[.1,.45,.18]
   elif group=='seat-passenger':vector=[.1,.45,-.18]
   elif group.startswith('brake-'):vector=[-.25 if group.endswith(('fl','fr')) else .25,.10,.70 if group.endswith(('fl','rl')) else -.70]
  else:
   vector=list(interior_vectors[vehicle][entry['group']]);desc=' > '.join(entry['expected_path']).lower()
   if entry['group'] in ['suspension','brakes'] and ('gauche' in desc or 'droite' in desc):vector=[-.25 if 'avant' in desc else .25,.12,.70 if 'gauche' in desc else -.70]
   if entry['group']=='front_edu' and entry['source_part_key']=='vlh.moteur-av':vector[2]=.5 if 'gauche' in desc else -.5
  # Inverse of the calibrated source rotation: values in the manifest are source-local.
  local=[vector[2],vector[1],-vector[0]] if is_ext else [-vector[0],vector[2],vector[1]]
  for instance in binding['instances']:instance['explode_vector']=local
 m['revision']='soutenance-exploded-20261118';p.write_text(json.dumps(m,ensure_ascii=False,indent=2)+'\n')
print('Explicit coherent vectors installed for both manifests')
