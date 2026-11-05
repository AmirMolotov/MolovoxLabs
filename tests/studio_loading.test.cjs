const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const loading=fs.readFileSync(path.join(__dirname,'../dist/studio-loading.js'),'utf8');
function harness(){
 const elements=new Map();const h={painted:0,$:id=>{if(!elements.has(id))elements.set(id,{hidden:true,textContent:''});return elements.get(id);},document:{querySelector:selector=>h.$(selector)},requestAnimationFrame:fn=>setImmediate(()=>{h.painted++;fn();}),setTimeout};
 vm.createContext(h);vm.runInContext(loading,h);return h;
}
test('blocking loading is painted before work starts and always clears after failure',async()=>{
 const h=harness();await assert.rejects(h.withLoading('Loading effect',async()=>{assert.equal(h.$('appLoader').hidden,false);assert.ok(h.painted>=2);assert.equal(h.$('aside').inert,true);throw new Error('bad plugin');},true),/bad plugin/);
 assert.equal(h.$('appLoader').hidden,true);assert.equal(h.$('appActivity').hidden,true);assert.equal(h.$('aside').inert,false);
});
test('nested jobs keep the loader visible until all blocking work finishes',()=>{
 const h=harness(),first=h.beginLoading('Starting',true),second=h.beginLoading('Opening devices',true);
 first();assert.equal(h.$('appLoader').hidden,false);assert.equal(h.$('appLoaderMessage').textContent,'Opening devices');second();assert.equal(h.$('appLoader').hidden,true);
});
test('settings queue survives failure and applies the latest configuration after in-flight work',async()=>{
 const h=harness(),applied=[];let release,value='old';const gate=new Promise(resolve=>release=resolve);
 const first=h.queueSettings(async()=>{await gate;throw new Error('old request failed');});
 const next=h.queueSettings(async()=>{applied.push(value);});value='latest';release();await assert.rejects(first);await next;
 assert.deepEqual(applied,['latest']);
});
test('rapid routing edits coalesce and final soundboard route wins',async()=>{
 const h=harness(),applied=[];let release,value='microphone';const gate=new Promise(resolve=>release=resolve);
 Object.assign(h,{updateLabels(){},options:()=>({route:value}),async nativeConfigure(o){applied.push(o.route);if(applied.length===1)await gate;},scheduleSave(){},refreshVSTCards(){},patchGraph:null,message(){}});
 const source=fs.readFileSync(path.join(__dirname,'../dist/studio.js'),'utf8'),start=source.indexOf('let configurePending='),end=source.indexOf("['master','boardVolume']",start);
 vm.runInContext(source.slice(start,end),h);const first=h.configure();
 while(!applied.length)await new Promise(resolve=>setImmediate(resolve));
 value='soundboard';const second=h.configure();release();await Promise.all([first,second]);
 assert.deepEqual(applied,['microphone','soundboard']);
});
