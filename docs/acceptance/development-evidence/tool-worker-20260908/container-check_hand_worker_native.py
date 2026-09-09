from pathlib import Path
import json,subprocess,hashlib,os,uuid
out=Path('/private/tmp/hand-tool-worker-container-20260908');out.mkdir(exist_ok=False)
workspace=out/'workspace';workspace.mkdir(mode=0o777);workspace.chmod(0o777)
binary=Path('/private/tmp/hand-tool-worker-linux-arm64-final-20260908');image='sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce'
secret=out/'outside';secret.write_text('outside');(workspace/'escape').symlink_to(secret)
records=[]
def run(tool,inputs,writable=True):
 name='hand-worker-test-'+uuid.uuid4().hex
 command=['docker','run','--name',name,'--rm','--pull=never','--network=none','--read-only','--cap-drop=ALL','--security-opt=no-new-privileges','--user',str(os.getuid())+':'+str(os.getgid()),'--mount','type=bind,src='+str(workspace)+',dst=/workspace,bind-recursive=disabled'+('' if writable else ',readonly'),'--mount','type=bind,src='+str(binary)+',dst=/hand-worker,readonly','--tmpfs','/tmp:rw,nosuid,nodev,size=64m,mode=1777','--workdir','/workspace','--interactive','--entrypoint','/hand-worker',image]
 request={'version':1,'tool':tool,'input':inputs}
 try:
  result=subprocess.run(command,input=json.dumps(request),capture_output=True,text=True,timeout=15)
  records.append({'request':request,'command':command,'exit_code':result.returncode,'stdout':result.stdout,'stderr':result.stderr})
  if result.returncode:raise RuntimeError(result.stderr)
  return json.loads(result.stdout)['result']
 finally:
  subprocess.run(['docker','rm','--force',name],capture_output=True,timeout=5)
report={'status':'failed','binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'image':image,'qualification':'development'}
try:
 assert not run('write_file',{'path':'file','content':'before'}).get('error')
 assert 'before' in run('read_file',{'path':'file'})['output']
 assert not run('edit_file',{'path':'file','old_string':'before','new_string':'after'}).get('error')
 assert (workspace/'file').read_text()=='after'
 assert run('write_file',{'path':'denied','content':'x'},False).get('error')
 assert not (workspace/'denied').exists()
 assert run('read_file',{'path':'escape'}).get('error')
 assert secret.read_text()=='outside'
 report.update(status='passed',scenarios=['container_worker_file_roundtrip','read_only_mount_denial','symlink_escape_denial'])
except Exception as e:report['error']=repr(e)
finally:
 (out/'records.json').write_text(json.dumps(records,indent=2)+'\n');(out/'run.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
raise SystemExit(0 if report['status']=='passed' else 1)
