const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('dist/studio.js','utf8');
test('delay toggle is included in saved audio options and applies without restarting listening',()=>{
 const controls=new Map();const h={$:id=>{if(!controls.has(id))controls.set(id,{checked:false,value:'100'});return controls.get(id);},patchOptions:()=>({}),micFilters:[],bypassFilters:false,selections:()=>({Inputs:[]}),configure:value=>h.configured=value};vm.createContext(h);vm.runInContext(source.slice(source.indexOf('function options()'),source.indexOf('function snapshotPreferences()')),h);vm.runInContext(source.slice(source.indexOf("$('monitorMic').onchange"),source.indexOf("$('autoListen').onchange")),h);
 assert.equal(h.options().SoundboardDelay,false);h.$('boardDelay').checked=true;h.$('boardDelay').onchange();assert.equal(h.options().SoundboardDelay,true);assert.equal(h.configured,true);assert.equal(h.options().MonitorMic,false);
});
test('pending pad shows a queue indicator with zero progress and transitions to normal playback',()=>{
 const classes=new Set(),view={pad:{classList:{toggle:(name,on)=>on?classes.add(name):classes.delete(name)}},fill:{style:{}},playIcon:{},time:{},sound:{Seconds:1},stops:[]};const h={playingSounds:new Map(),padViews:new Map([['pad',view]]),updateSamplerPlayback(){},updateTrimPlayback(){},updateLoopPreview(){}};vm.createContext(h);vm.runInContext(source.slice(source.indexOf('function updatePlayback('),source.indexOf("$('boardSelect').onchange")),h);
 h.updatePlayback([{ID:'pad',Sequence:1,Queued:true,DelayRemaining:.5,Remaining:1,Progress:0}]);assert.equal(classes.has('is-queued'),true);assert.equal(view.fill.style.width,'0%');assert.equal(view.playIcon.textContent,'…');assert.equal(view.time.textContent,'0.5s delay');
 h.updatePlayback([{ID:'pad',Sequence:1,Queued:false,DelayRemaining:0,Remaining:.8,Progress:.2}]);assert.equal(classes.has('is-queued'),false);assert.equal(view.fill.style.width,'20%');assert.equal(view.playIcon.textContent,'■');h.updatePlayback([]);assert.equal(view.playIcon.textContent,'▶');assert.equal(classes.has('is-playing'),false);
});
