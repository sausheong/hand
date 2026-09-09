import sys,os,json,tempfile,hashlib,subprocess,stat,shutil
from pathlib import Path
sys.path.insert(0,'scripts')
from evidence import source_snapshot
from evaluation_checkout import prepare_checkout,git
source=Path('/private/tmp/hand-harness-persistence-20260908');base='09c30fe2d529d6125f423593f621e970a7ce8735'
out=Path(tempfile.mkdtemp(prefix='hand-harness-gate-review-20260909-',dir='/private/tmp'))
before=source_snapshot(source);original_index=(source/'.git/index').read_bytes()
staging=out/'snapshot'
prepare_checkout(source,base,staging)
names=git(source,'ls-files','-z','--cached','--others','--exclude-standard')
source_names={n.decode() for n in names.split(b'\0') if n}
for raw in git(staging,'ls-files','-z').split(b'\0'):
 if raw and raw.decode() not in source_names:
  (staging/raw.decode()).unlink()
for name in source_names:
 src=source/name;dst=staging/name
 if dst.is_file() or dst.is_symlink():dst.unlink()
 if src.is_symlink():
  dst.parent.mkdir(parents=True,exist_ok=True);dst.symlink_to(os.readlink(src))
 elif src.is_file():
  dst.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(src,dst)
def run(*args):return git(staging,*args)
run('add','--force','--all')
patch=out/'harness.patch';patch.write_bytes(run('diff','--cached','--binary','--full-index',base,'--','.'))
reconstructed=out/'reconstructed';prepare_checkout(source,base,reconstructed)
git(reconstructed,'apply','--check','--index','--',str(patch));git(reconstructed,'apply','--index','--',str(patch))
def files(repo):
 result={}
 for name in git(repo,'ls-files','-z','--cached','--others','--exclude-standard').split(b'\0'):
  if not name:continue
  name=name.decode();p=repo/name
  if p.is_symlink():result[name]=dict(kind='symlink',target=os.readlink(p))
  elif p.is_file():result[name]=dict(kind='regular',sha256=hashlib.sha256(p.read_bytes()).hexdigest(),executable=bool(p.stat().st_mode&0o111))
 return result
original=files(source);copy=files(reconstructed);assert original==copy
assert original_index==(source/'.git/index').read_bytes()
assert source_snapshot(source)['sha256']==before['sha256']
vet_env=dict(os.environ,GOCACHE='/private/tmp/hand-review-gocache',GOPROXY='off',GOWORK='off')
with (out/'vet.log').open('wb') as f:vet=subprocess.run(['go','vet','./...'],cwd=reconstructed,env=vet_env,stdout=f,stderr=subprocess.STDOUT)
report=dict(status='cumulative_patch_reconstructed',base_commit=base,source_head=git(source,'rev-parse','HEAD').decode().strip(),source_worktree_sha256=before['sha256'],patch=str(patch),patch_sha256=hashlib.sha256(patch.read_bytes()).hexdigest(),patch_bytes=patch.stat().st_size,files_verified=len(copy),source_index_unchanged=True,source_unchanged=True,reconstruction=str(reconstructed),reconstructed_files=copy,vet_exit_code=vet.returncode,vet_log=str(out/'vet.log'))
Path('/private/tmp/hand-harness-gate-review-20260909.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k!='reconstructed_files'}))
assert vet.returncode==0
