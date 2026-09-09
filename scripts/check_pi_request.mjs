// Exercise the pinned real Pi provider and Anthropic SDK up to fetch only.
// No network transport or real credentials; the injected response is a failure.
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';

const [runtimeArg, outputArg] = process.argv.slice(2);
if (!runtimeArg || !outputArg) throw new Error('runtime and new output directory required');
const runtime = resolve(runtimeArg), output = resolve(outputArg);
const pkg = join(runtime, 'node_modules/@earendil-works/pi-ai');
const metadata = JSON.parse(await readFile(join(pkg, 'package.json'), 'utf8'));
if (metadata.version !== '0.85.1') throw new Error('Pi AI 0.85.1 required');
let unexpectedFetches = 0;
globalThis.fetch = async () => { unexpectedFetches++; throw new Error('network forbidden'); };
const { stream } = await import(pathToFileURL(join(pkg, 'dist/api/anthropic-messages.js')));
const { ANTHROPIC_MODELS } = await import(pathToFileURL(join(pkg, 'dist/providers/anthropic.models.js')));
const model = ANTHROPIC_MODELS['claude-sonnet-4-5-20250929'];
if (!model) throw new Error('pinned catalogue model missing');
await mkdir(output, { mode: 0o700 });
const report = { status:'running', version:metadata.version, cases:[], limitations:[
  'Actual installed Pi provider and SDK serialize requests, but fetch is injected and returns HTTP 400.',
  'No HTTP socket, full agent session, provider call, model quality or billing verification.',
  'Catalogue model is an offline probe input, not an approved comparison model.'
] };
try {
  const usage = { input:0, output:0, cacheRead:0, cacheWrite:0, totalTokens:0,
    cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0} };
  for (const name of ['text', 'tool-roundtrip', 'thinking']) {
    const context = { systemPrompt:'Review code carefully.', messages:[
      {role:'user',content:'Read example.go and propose a correction.',timestamp:1}
    ], tools:[{name:'read',description:'Read a file',parameters:{type:'object',properties:{path:{type:'string'}},required:['path']}}] };
    if (name === 'tool-roundtrip') context.messages.push(
      {role:'assistant',content:[{type:'toolCall',id:'tool-1',name:'read',arguments:{path:'example.go'}}],
       api:model.api,provider:model.provider,model:model.id,usage,stopReason:'toolUse',timestamp:2},
      {role:'toolResult',toolCallId:'tool-1',toolName:'read',content:[{type:'text',text:'package main'}],isError:false,timestamp:3});
    let calls = 0;
    const captures = [];
    const capture = async (input, init) => {
      calls++;
      const request = new Request(input, init);
      const raw = Buffer.from(await request.arrayBuffer());
      const file = `${name}-${calls}.json`;
      await writeFile(join(output,file),raw,{flag:'wx'});
      const headers = Object.fromEntries([...request.headers].filter(([key]) => !['x-api-key','authorization'].includes(key)));
      captures.push({url:request.url,method:request.method,headers,body:file,
        body_sha256:createHash('sha256').update(raw).digest('hex')});
      return new Response(JSON.stringify({type:'error',error:{type:'invalid_request_error',message:'offline capture complete'}}),
        {status:400,headers:{'content-type':'application/json'}});
    };
    const events = stream(model,context,{apiKey:'offline-fixture-key',fetch:capture,maxTokens:4096,maxRetries:0,
      timeoutMs:3000,cacheRetention:'short',...(name==='thinking'?{thinkingEnabled:true,thinkingBudgetTokens:1024}:{})});
    const types=[];
    for await (const event of events) types.push(event.type);
    const result = await events.result();
    report.cases.push({name,captures,event_types:types,stop_reason:result.stopReason});
    if(calls !== 1 || result.stopReason !== 'error') throw new Error('expected one captured rejected request');
  }
  if(unexpectedFetches) throw new Error('unexpected default fetch');
  report.status='requests_captured';
} catch(error) {
  report.status='failed'; report.error=String(error); process.exitCode=1;
} finally {
  report.unexpected_fetches=unexpectedFetches;
  report.runtime_sources=[];
  for (const file of ['package.json','dist/api/anthropic-messages.js','dist/providers/data/anthropic.json']) {
    report.runtime_sources.push({file,sha256:createHash('sha256').update(await readFile(join(pkg,file))).digest('hex')});
  }
  report.probe_sha256=createHash('sha256').update(await readFile(new URL(import.meta.url))).digest('hex');
  await writeFile(join(output,'report.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx'});
  console.log(JSON.stringify({status:report.status,cases:report.cases.length,report:join(output,'report.json')}));
}
