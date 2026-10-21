#!/usr/bin/env python3
"""Losslessly retain text/vision tensors; omit unused audio and build-only assets."""
import argparse,json,pathlib,shutil,struct
parser=argparse.ArgumentParser()
parser.add_argument('resources',type=pathlib.Path)
args=parser.parse_args()
resources=args.resources
weight=resources/'model/model.safetensors'
with weight.open('rb') as src:
    original_size=struct.unpack('<Q',src.read(8))[0]
    original=json.loads(src.read(original_size))
    header={}
    if '__metadata__' in original: header['__metadata__']=original['__metadata__']
    offset=0
    retained=[]
    for name,tensor in original.items():
        if name=='__metadata__' or name.startswith(('audio_tower.','embed_audio.')): continue
        start,end=tensor['data_offsets']
        header[name]={**tensor,'data_offsets':[offset,offset+end-start]}
        offset+=end-start
        retained.append((start,end))
    encoded=json.dumps(header,separators=(',',':')).encode()
    encoded+=b' '*((-len(encoded))%8)
    temporary=weight.with_suffix('.tmp')
    with temporary.open('wb') as dst:
        dst.write(struct.pack('<Q',len(encoded)));dst.write(encoded)
        for start,end in retained:
            src.seek(8+original_size+start)
            remaining=end-start
            while remaining:
                data=src.read(min(remaining,4*1024*1024))
                if not data: raise ValueError('Truncated safetensors input')
                dst.write(data);remaining-=len(data)
previous=weight.stat().st_size
temporary.replace(weight)
python=resources/'python'
# These are development/install tools, not inference dependencies.
for relative in ('include','share','lib/pkgconfig','lib/python3.12/site-packages/torch/include','lib/python3.12/site-packages/pip','lib/python3.12/site-packages/setuptools'):
    path=python/relative
    if path.is_dir(): shutil.rmtree(path)
for site in (python/'Lib/site-packages', python/'lib/python3.12/site-packages'):
    for relative in ('torch/include', 'pip', 'setuptools', 'wheel'):
        path=site/relative
        if path.is_dir(): shutil.rmtree(path)
for cache in python.rglob('__pycache__'): shutil.rmtree(cache)
manifest=resources/'model-manifest.json'
info=json.loads(manifest.read_text())
info['localens_modifications']=['Omitted unused audio_tower and embed_audio tensors; retained text and vision tensors byte-for-byte.']
info['weights_bytes']=weight.stat().st_size
manifest.write_text(json.dumps(info,indent=2)+'\n')
print(f'Weights: {previous:,} -> {weight.stat().st_size:,} bytes',flush=True)
