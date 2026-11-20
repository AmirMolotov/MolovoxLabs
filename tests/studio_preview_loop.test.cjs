const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function harness(){const elements=new Map(),listeners={};const h={URL,location:{href:'http://local/session/studio.html'},audioTrimBuffer:null,audioTrimSource:{ID:'audio-source'},audioTrimBoard:'audio-board',samplerSource:{ID:'video-source'},audioTrimPreviewRange:null,active:true,currentTab:'sampler',errorText:String,samplerSeconds:String,nativeStopSound:async()=>{},audioSelectionWAV:async()=>new ArrayBuffer(100),mediaSelectionWAV:async(source,start,end)=>{h.selection={source,start,end};return new ArrayBuffer(100);},samplerWAV:()=>new ArrayBuffer(100),readSamplerRange:()=>({In:2,Out:6}),seekSamplerVideo:async(video,time)=>{video.currentTime=time;},
 $:id=>{if(!elements.has(id))elements.set(id,{value:'video-board',open:false,currentTime:0,paused:true,pause(){this.paused=true;},async play(){this.paused=false;},addEventListener(){}});return elements.get(id);},document:{addEventListener:(name,fn)=>{listeners[name]=fn;},querySelector:selector=>selector.includes('triggerDialog')?(h.triggerOpen?{}:null):{hidden:false}},fetch:async(url,options)=>{h.request=options;return {ok:true};},audioTrim:{start:4,end:7,duration:100,position:4,setSpacePan(on){this.spacePan=on;if(on)this.panUsed=false;},playhead(time){this.position=time;}},videoTrim:{start:2,end:6,duration:100,setSpacePan(on){this.spacePan=on;if(on)this.panUsed=false;},playhead(time){this.position=time;}}};vm.createContext(h);vm.runInContext(fs.readFileSync('web/js/studio-preview-loop.js','utf8'),h);h.listeners=listeners;return h;}
test('tap Space pauses and resumes at the cursor while a Space-drag only pans',async()=>{
 const h=harness();h.$('audioTrimDialog').open=true;const e={code:'Space',target:{closest(){return null;}},preventDefault(){},stopImmediatePropagation(){}};
 h.listeners.keydown(e);h.audioTrim.panUsed=true;h.listeners.keyup(e);assert.equal(h.request,undefined);
 h.listeners.keydown(e);h.listeners.keyup(e);await new Promise(r=>setImmediate(r));assert.equal(h.selection.start,4);
 h.audioTrim.position=5;h.listeners.keydown(e);h.listeners.keyup(e);assert.equal(vm.runInContext('audioTrimPlaying',h),false);assert.equal(h.audioTrim.position,5);
 h.listeners.keydown(e);h.listeners.keyup(e);await new Promise(r=>setImmediate(r));assert.equal(h.selection.start,5);
});
test('Space pauses and resumes without toggling a focused Loop selection checkbox',async()=>{
 const h=harness(),loop=h.$('audioTrimLoop');h.$('audioTrimDialog').open=true;loop.checked=true;
 loop.closest=selector=>selector.includes('input:not([type="range"])')&&!selector.includes(':not([type="checkbox"])')?loop:null;
 const tap=()=>{
  let prevented=false;
  const event={code:'Space',target:loop,timeStamp:10,preventDefault(){prevented=true;},stopImmediatePropagation(){}};
  h.listeners.keydown(event);h.listeners.keyup({...event,timeStamp:50});
  if(!prevented)loop.checked=!loop.checked;
 };
 await h.playTrimFrom('audio',5);tap();
 assert.equal(loop.checked,true);assert.equal(vm.runInContext('audioTrimPlaying',h),false);
 h.audioTrim.position=5.5;tap();await new Promise(resolve=>setImmediate(resolve));
 assert.equal(loop.checked,true);assert.equal(h.request.headers['X-Preview-Offset'],'1.5');
});
test('L toggles the loop and loop resume posts the offset without changing its boundaries',async()=>{
 const h=harness();h.$('audioTrimDialog').open=true;h.$('audioTrimLoop').onchange=()=>{};
 h.listeners.keydown({code:'KeyL',target:{closest(){return null;}},preventDefault(){},stopImmediatePropagation(){}});
 assert.equal(h.$('audioTrimLoop').checked,true);await h.playTrimFrom('audio',5.5);
 assert.equal(h.request.headers['X-Preview-Loop'],'true');assert.equal(h.request.headers['X-Preview-Offset'],'1.5');assert.equal(h.selection.start,4);assert.equal(h.selection.end,7);
 await h.toggleAudioTrimPlayback();h.audioTrim.position=5;await h.toggleAudioTrimPlayback();assert.equal(h.request.headers['X-Preview-Offset'],'1');
});
test('video loop restarts at trim start and follows native cycle position',async()=>{const h=harness();await h.beginTrimLoop('video');assert.equal(h.$('samplerEditorVideo').currentTime,2);h.$('samplerEditorVideo').currentTime=6.1;h.$('samplerEditorVideo').ontimeupdate();assert.equal(h.$('samplerEditorVideo').currentTime,2);h.updateLoopPreview([{ID:'@video-preview',Position:.25,Sequence:1}]);assert.equal(h.videoTrim.position,2.25);});
test('trimmer shortcuts respect text inputs and shortcut assignment',()=>{
 const h=harness();const event={code:'Space',key:' ',target:{closest:()=>({})},preventDefault(){assert.fail('typing shortcut was intercepted');}};
 h.listeners.keydown(event);assert.equal(h.videoTrim.spacePan,undefined);
 event.target.closest=()=>null;h.triggerOpen=true;h.listeners.keydown(event);assert.equal(h.videoTrim.spacePan,undefined);
});
test('brackets select a tool and Space temporarily pans in either trimmer',()=>{
 for(const kind of ['audio','video']){
  const h=harness(),timeline=h[kind+'Trim'];h.$('audioTrimDialog').open=kind==='audio';
  timeline.setTrimTool=edge=>{timeline.trimTool=edge;};
  const press=(key,timeStamp=0)=>h.listeners.keydown({key,timeStamp,preventDefault(){},stopImmediatePropagation(){}});
  press('[');assert.equal(timeline.trimTool,'start');assert.equal(h.request,undefined);
  press(']');press(' ',10);assert.equal(timeline.spacePan,true);
  h.listeners.keyup({key:' ',timeStamp:500,preventDefault(){},stopImmediatePropagation(){}});
  assert.equal(timeline.spacePan,false);assert.equal(timeline.trimTool,'end');assert.equal(h.request,undefined);
  assert.equal(vm.runInContext('trimLoopMode',h),null);
  press('Escape');assert.equal(timeline.trimTool,null);
 }
});
test('tapping Space still starts and stops the video loop',async()=>{
 const h=harness();
 const event={code:'Space',timeStamp:10,preventDefault(){},stopImmediatePropagation(){}};
 h.listeners.keydown(event);assert.equal(h.videoTrim.spacePan,true);assert.equal(h.request,undefined);
 h.listeners.keyup({...event,timeStamp:50});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(h.request.headers['X-Preview-Loop'],'true');
 h.listeners.keydown(event);h.listeners.keyup({...event,timeStamp:50});
 assert.equal(h.$('samplerEditorVideo').paused,true);
});
test('click audition starts at the selected source position instead of trim start',async()=>{const h=harness();await h.playTrimFrom('audio',5.5);assert.equal(h.selection.start,5.5);assert.equal(h.selection.end,7);assert.equal(h.audioTrim.position,5.5);assert.equal(h.request.headers['X-Preview-Loop'],undefined);await h.playTrimFrom('video',40);assert.equal(h.selection.start,40);assert.equal(h.$('samplerEditorVideo').currentTime,40);h.$('samplerEditorVideo').currentTime=45;h.$('samplerEditorVideo').ontimeupdate();assert.equal(h.$('samplerEditorVideo').paused,false);assert.equal(h.videoTrim.position,45);});
