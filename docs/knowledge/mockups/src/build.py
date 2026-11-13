import json,re,sys
import os; sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
import lib, shell, screens
from lib import F
# components
rc=F("Vesper rail",-420,0,52,900); shell.rail(rc); rc.node.update(type="component",id="n_cmp_rail"); rc.node.pop("radius")
ec=F("Explorer (default tree)",-420,960,308,900); shell.explorer(ec,active=None,bg=False); ec.node.update(type="component",id="n_cmp_explorer"); ec.node.pop("fill"); ec.node.pop("radius"); ec.node["clip"]=False
def rail(f,active="book"):
    f.add({"type":"instance","name":"Vesper rail","component":"n_cmp_rail","x":0,"y":0,"w":52,"h":900})
orig_explorer=shell.explorer
def explorer(f,active="The old harbour",tree=None,view="yours",footer=True,bg=True,body=True):
    if tree is None and view=="yours":
        f.rect(52,0,256,900,fill=lib.SIDE,name="Explorer"); shell.explorer_hl(f,active)
        f.add({"type":"instance","name":"Explorer","component":"n_cmp_explorer","x":0,"y":0,"w":308,"h":900})
    else: orig_explorer(f,active,tree,view,footer)
screens.rail=rail; screens.explorer=explorer
frames=[]
for i,fn in enumerate(screens.ORDER):
    fr=fn((i%2)*1560,(i//2)*1020).node
    if fn is screens.s_editor: fr.update(type="component",id="n_scr_editor")
    frames.append(fr)
def num(v):
    if isinstance(v,float):
        v=round(v,1); return int(v) if v==int(v) else v
    return v
def compact(n):
    if n.get('name','').startswith('icon'):
        n.pop('name'); n.pop('join',None)
    if 'd' in n: n['d']=re.sub(r'-?\d+\.\d+',lambda m:('%g'%round(float(m.group()),1)),n['d'])
    for k in list(n): n[k]=num(n[k])
    for p in n.get('paragraphs',[]):
        for r in p['runs']:
            for k in r: r[k]=num(r[k])
    for c in n.get('children',[]): compact(c)
for n in [rc.node,ec.node]+frames: compact(n)
# frame ids for review clarity
for i,fr in enumerate(frames):
    fr.setdefault("id",f"n_kv2_{i:02d}")
comps=[rc.node,ec.node]
tot=0
for n in comps+frames:
    s=len(json.dumps(n,separators=(',',':'),ensure_ascii=False)); tot+=s; print(n['name'][:34],s)
print('total',tot)
json.dump({"components":comps,"frames":frames},open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'final.json'),'w'),separators=(',',':'),ensure_ascii=False)
