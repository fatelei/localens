#!/usr/bin/env python3
"""Package a self-contained arm64 app after scripts/build.sh and scripts/setup-model.sh.
Usage: .venv/bin/python scripts/package_macos.py [--assets prepared-Resources]
"""
import argparse,json,pathlib,shutil,subprocess,sys,sysconfig,tempfile
parser=argparse.ArgumentParser()
parser.add_argument('--assets',type=pathlib.Path)
args=parser.parse_args()
project=pathlib.Path(__file__).resolve().parent.parent
output=project.parent/'Shiwa-macOS-arm64.zip'
with tempfile.TemporaryDirectory(prefix='localens-full-',dir='/private/tmp') as temporary:
    app=pathlib.Path(temporary)/'拾物.app'
    subprocess.run(['cp','-R','-X',str(project/'build/bin/Shiwa.app'),str(app)],check=True)
    resources=app/'Contents/Resources'
    if args.assets:
        subprocess.run(['cp','-R','-X',str(args.assets.resolve())+'/.',str(resources)],check=True)
    else:
        if sys.prefix==sys.base_prefix:
            raise SystemExit('Run with the project .venv Python after scripts/setup-model.sh')
        shutil.copytree(sys.base_prefix,resources/'python',symlinks=False,dirs_exist_ok=True)
        version=f'python{sys.version_info.major}.{sys.version_info.minor}'
        shutil.copytree(sysconfig.get_path('purelib'),resources/'python/lib'/version/'site-packages',symlinks=False,dirs_exist_ok=True)
        cache=project/'.models/hub/models--google--embeddinggemma-2'
        revision=(cache/'refs/main').read_text().strip()
        shutil.copytree(cache/'snapshots'/revision,resources/'model',symlinks=False,dirs_exist_ok=True)
        (resources/'model-manifest.json').write_text(json.dumps({'id':'google/embeddinggemma-2','revision':revision,'offline':True},indent=2))
    subprocess.run([sys.executable,str(project/'scripts/optimize_assets.py'),str(resources)],check=True)
    shutil.copy2(project/'engine/THIRD-PARTY-NOTICES.txt',resources)
    for cache in resources.rglob('__pycache__'):
        shutil.rmtree(cache)
    subprocess.run(['xattr','-cr',str(app)],check=True)
    magic={b'\xfe\xed\xfa\xce',b'\xce\xfa\xed\xfe',b'\xfe\xed\xfa\xcf',b'\xcf\xfa\xed\xfe',b'\xca\xfe\xba\xbe',b'\xbe\xba\xfe\xca'}
    binaries=[]
    for file in resources.rglob('*'):
        if file.is_file() and not file.is_symlink():
            with file.open('rb') as stream:
                if stream.read(4) in magic: binaries.append(file)
    print(f'Signing {len(binaries)} embedded binaries',flush=True)
    for file in binaries:
        subprocess.run(['codesign','--force','--sign','-',str(file)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    subprocess.run(['codesign','--force','--deep','--sign','-',str(app)],check=True)
    subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
    # Retain the exact signed app for relocation/inference verification.
    verification=project.parent/'拾物.app'
    verification.parent.mkdir(parents=True,exist_ok=True)
    if verification.exists(): shutil.rmtree(verification)
    subprocess.run(['cp','-R','-X',str(app),str(verification)],check=True)
    print('Self-contained app copied to outputs/拾物.app',flush=True)
    subprocess.run(['ditto','-c','-k','--keepParent',str(app),str(output)],check=True)
    print(f'Packaged {output} ({output.stat().st_size/1024**3:.2f} GiB)',flush=True)
