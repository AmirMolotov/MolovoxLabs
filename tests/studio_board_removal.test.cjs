const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('web/js/studio.js','utf8');
test('removal popup renews its ten-second timer after hover or keyboard focus and can redo after dismissal',async()=>{
 const controls=new Map(),keyboard={};
 const element=()=>({
  hidden:true,listeners:{},held:false,matches(){return this.held;},querySelector(){return null;},
  addEventListener(name,fn){this.listeners[name]=fn;},setAttribute(){},
 });
 let timer=null,result;
 const h={
  $:id=>{if(!controls.has(id))controls.set(id,element());return controls.get(id);},
  document:{addEventListener:(name,fn)=>{keyboard[name]=fn;},querySelector:()=>null},
  setTimeout:(callback,delay)=>{timer={callback,delay};return timer;},clearTimeout:()=>{timer=null;},
  queueMicrotask:fn=>fn(),currentTab:'board',boardRecording:null,
  library:{Boards:[]},withLoading:(_,action)=>action(),queueSettings:action=>action(),
  renderLibrary(){},configure:async()=>true,
  nativeChangeRemovalHistory:async redo=>{result=redo;return {Library:redo?removed:restored};},
 };
 const item={Board:{Name:'Library',Sounds:[{Name:'Chime'}]},PadID:'pad'};
 const removed={Boards:[],UndoRemovals:[item],RedoRemovals:[]},restored={Boards:[],UndoRemovals:[],RedoRemovals:[item]};
 h.library=removed;
 vm.createContext(h);
 vm.runInContext(source.slice(source.indexOf('let deletingBoardID='),source.indexOf('function requestBoardDeletion')),h);
 const popup=h.$('removalPopup');
 h.updateRemovalControls();h.showRemovalPopup('Removed Chime');
 assert.equal(popup.hidden,false);assert.equal(timer.delay,10000);
 for(const [enter,leave] of [['pointerenter','pointerleave'],['focusin','focusout']]){
  popup.held=true;popup.listeners[enter]();assert.equal(timer,null);
  popup.held=false;popup.listeners[leave]();assert.equal(timer.delay,10000);
 }
 await h.$('removalUndo').onclick();
 assert.equal(result,false);assert.equal(h.$('removalPopupMessage').textContent,'Restored Chime');
 assert.equal(h.$('removalUndo').disabled,true);assert.equal(h.$('removalRedo').disabled,false);
 timer.callback();assert.equal(popup.hidden,true);
 let prevented=false;
 keyboard.keydown({ctrlKey:true,key:'y',target:{closest:()=>null},preventDefault(){prevented=true;}});
 await new Promise(setImmediate);
 assert.equal(prevented,true);assert.equal(result,true);assert.equal(popup.hidden,false);
 assert.equal(h.$('removalPopupMessage').textContent,'Removed Chime');
});
