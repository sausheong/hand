"""Summarise raw long-transcript benchmark samples; not release qualification."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re


def summarise(raw):
    if not re.search(r'^PASS$', raw, re.M) or re.search(r'^FAIL', raw, re.M):
        raise ValueError('benchmark must finish successfully')
    metrics=re.findall(r'^BenchmarkTranscript10000Blocks-\d+\s+(\d+)\s+[\d.]+ ns/op\s+([\d.]+) p95-ms$',raw,re.M)
    groups=[list(map(float, value.split())) for value in re.findall(r'latency_ms_sorted=\[([^\]]*)\]',raw)]
    if any(not group or any(not math.isfinite(x) or x<0 for x in group) for group in groups):
        raise ValueError('invalid latency samples')
    # Go first calibrates each benchmark with N=1. Keep this count visible;
    # measured groups must correspond one-for-one to the reported iterations.
    measured=[group for group in groups if len(group)>1]
    if len(measured)!=len(metrics) or not measured:
        raise ValueError('missing or unmatched raw sample groups')
    runs=[]
    for samples,(iterations,reported) in zip(measured,metrics):
        if len(samples)!=int(iterations): raise ValueError('iteration/sample mismatch')
        if samples!=sorted(samples): raise ValueError('expected sorted raw samples')
        p95=samples[math.ceil(len(samples)*.95)-1]
        if not math.isclose(p95,float(reported),rel_tol=.001,abs_tol=.0001):
            raise ValueError('reported percentile disagrees with raw samples')
        runs.append(dict(events=len(samples),p95_ms=p95,max_ms=max(samples)))
    combined=sorted(x for group in measured for x in group)
    p95=combined[math.ceil(len(combined)*.95)-1]
    sufficient=len(runs)>=30 and all(run['events']>=1000 for run in runs)
    return dict(status='development_workload_passed' if sufficient and p95<=100 else 'incomplete_or_failed',
                blocks=10000,runs=runs,total_events=len(combined),ui_key_p95_ms=p95,
                worst_run_p95_ms=max(run['p95_ms'] for run in runs),max_ms=max(combined),
                calibration_groups=sum(len(group)==1 for group in groups),
                workload_sufficient=sufficient,threshold_ms=100,
                scope='In-process stream refresh, key handling and View; excludes terminal display and OS event delivery.')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('log',type=Path)
    parser.add_argument('--out',type=Path,required=True)
    args=parser.parse_args()
    try:
        payload=args.log.read_bytes()
        result=summarise(payload.decode())
        result['raw_sha256']=hashlib.sha256(payload).hexdigest()
        with args.out.open('x') as output: json.dump(result,output,indent=2);output.write('\n')
        return 0 if result['status']=='development_workload_passed' else 1
    except (ValueError,OSError) as error:
        print(json.dumps(dict(status='invalid',error=str(error))))
        return 2


if __name__=='__main__': raise SystemExit(main())
