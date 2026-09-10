// Actual installed Pi provider through a loopback gateway, with guarded fetch.
import { readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
const [runtime, configPath, reportPath] = process.argv.slice(2);
const config = JSON.parse(await readFile(configPath,'utf8'));
const gateway = new URL(config.url);
if(gateway.hostname !== '127.0.0.1' || gateway.protocol !== 'http:') throw new Error('loopback gateway required');
const nativeFetch = globalThis.fetch;
const guardedFetch = async (input, init) => {
  const url = new URL(input instanceof Request ? input.url : String(input));
  if(url.origin !== gateway.origin || url.pathname !== '/v1/messages') throw new Error('non-gateway request forbidden');
  return nativeFetch(input,{...init,redirect:'error'});
};
globalThis.fetch = guardedFetch;
const pkg = join(runtime,'node_modules/@earendil-works/pi-ai');
const metadata = JSON.parse(await readFile(join(pkg,'package.json'),'utf8'));
if(metadata.version !== '0.85.1') throw new Error('pinned Pi AI runtime required');
const { stream } = await import(pathToFileURL(join(pkg,'dist/api/anthropic-messages.js')));
const { ANTHROPIC_MODELS } = await import(pathToFileURL(join(pkg,'dist/providers/anthropic.models.js')));
const model = {...ANTHROPIC_MODELS['claude-sonnet-4-5-20250929'],baseUrl:config.url};
const report = {status:'running',version:metadata.version,cases:[]};
try {
  const usage={input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}};
  for(const name of ['text','tool-roundtrip','thinking']) {
    const context={systemPrompt:'Review code carefully.',messages:[{role:'user',content:'Read example.go.',timestamp:1}],
      tools:[{name:'read',description:'Read a file',parameters:{type:'object',properties:{path:{type:'string'}},required:['path']}}]};
    if(name==='tool-roundtrip')context.messages.push(
      {role:'assistant',content:[{type:'toolCall',id:'tool-1',name:'read',arguments:{path:'example.go'}}],api:model.api,provider:model.provider,model:model.id,usage,stopReason:'toolUse',timestamp:2},
      {role:'toolResult',toolCallId:'tool-1',toolName:'read',content:[{type:'text',text:'package main'}],isError:false,timestamp:3});
    const events=stream(model,context,{apiKey:config.token,fetch:guardedFetch,maxTokens:4096,maxRetries:0,timeoutMs:3000,
      cacheRetention:'short',...(name==='thinking'?{thinkingEnabled:true,thinkingBudgetTokens:1024}:{})});
    const types=[];for await(const event of events)types.push(event.type);
    const result=await events.result();
    report.cases.push({name,event_types:types,stop_reason:result.stopReason,content:result.content,usage:result.usage,error:result.errorMessage});
    if(result.stopReason!=='stop' || result.content.map(c=>c.type==='text'?c.text:'').join('')!=='offline gateway fixture')throw new Error('SDK did not receive complete fixture response');
  }
  report.status='sdk_gateway_passed';
}catch(error){report.status='failed';report.error=String(error);process.exitCode=1;}
await writeFile(reportPath,JSON.stringify(report,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({status:report.status,cases:report.cases.length}));
