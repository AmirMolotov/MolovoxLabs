const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');

test('startup renders the saved soundboard without waiting for other tabs',async()=>{
 const events=[],elements=new Map(),source=fs.readFileSync('web/js/studio.js','utf8');
 const h={$:id=>{if(!elements.has(id))elements.set(id,{});return elements.get(id);},
  nativeLoadPreferences:async()=>({BoardID:'saved',Saved:true,AutoListen:true}),URL,location:{href:'http://127.0.0.1/session/studio.html'},
  fetch:async url=>{assert.equal(url.pathname,'/session/sampler/library');return {ok:true,json:async()=>({Boards:[{ID:'saved'}]})};},
  beginLoading:()=>()=>events.push('board ready'),paintLoading:async()=>{},initSoundboardView(){},
  initWindowsStartup:()=>events.push('startup setting'),initVST:()=>events.push('plugins'),refreshMIDI:()=>events.push('MIDI devices'),
  fillDevices:()=>false,nativeDevices:async()=>({}),initSampler:()=>events.push('sampler'),initArrangement:()=>events.push('arrangement'),
  renderLibrary:id=>events.push('render '+id),loadFilterModels(){},updateLabels(){},initPatch(){},initSamplerPreferences:async()=>{},
  nativeConfigure:async()=>{},options:()=>({}),start:async()=>events.push('audio'),idle(){},setLive(){},message:text=>assert.fail(text),errorText:String};
 vm.createContext(h);vm.runInContext(source.slice(source.indexOf('async function init()')),h);
 await h.init();
 assert.deepEqual(events,['render saved','board ready','audio']);
});
