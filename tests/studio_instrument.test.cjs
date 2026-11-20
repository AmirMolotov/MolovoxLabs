const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');

test('keyboard mode sends note pairs, ignores repeats, and releases notes when focus is lost',async()=>{
 const elements=new Map(),listeners={},calls=[];
 const node=()=>({children:[],style:{},classList:{add(){},remove(){}},append(...children){this.children.push(...children);},setAttribute(){}});
 const h={
  $:id=>{if(!elements.has(id))elements.set(id,node());return elements.get(id);},currentTab:'instruments',errorText:String,
  document:{createElement:node,hasFocus:()=>true,querySelector:()=>null},window:{addEventListener(name,fn){listeners[name]=fn;}},
  nativeInstrumentInput:async(...input)=>calls.push(input),nativeInstrumentNote:async(...note)=>calls.push(note),
 };
 vm.createContext(h);vm.runInContext(fs.readFileSync('web/js/studio-instrument.js','utf8'),h);
 const event=(type,key,code,repeat=false)=>({type,key,code,repeat,preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;}});
 h.$('instrumentKeyboard').checked=true;h.$('instrumentKeyboard').onchange();
 const press=event('keydown','a','KeyA');listeners.keydown(press);assert.equal(press.stopped,true);assert.equal(press.prevented,true);
 listeners.keydown(event('keydown','a','KeyA',true));listeners.keyup(event('keyup','a','KeyA'));
 listeners.keydown(event('keydown','s','KeyS'));listeners.blur();listeners.focus();
 listeners.keyup(event('keyup','s','KeyS'));
 listeners.keydown(event('keydown','d','KeyD'));
 h.currentTab='board';h.updateInstrumentInput();
 assert.equal(vm.runInContext('instrumentHeld.size',h),0);
 for(const key of [event('keyup','d','KeyD'),event('keydown','a','KeyA'),event('keydown','Escape','Escape')]){
  listeners[key.type](key);
  assert.equal(key.stopped,undefined);assert.equal(key.prevented,undefined);
 }
 listeners.focus();
 assert.equal(h.$('instrumentKeyboard').checked,true);
 h.currentTab='instruments';h.updateInstrumentInput();
 listeners.keydown(event('keydown','a','KeyA'));listeners.keyup(event('keyup','a','KeyA'));
 listeners.keydown(event('keydown','Escape','Escape'));
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls,[[true,true],[60,100],[60,0],[62,100],[false,true],[true,true],[64,100],[false,false],[false,false],[true,true],[60,100],[60,0],[false,true]]);
 assert.equal(h.$('instrumentKeyboard').checked,false);
});
