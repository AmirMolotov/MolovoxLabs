const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');

test('keyboard mode sends notes without intercepting shortcuts and releases held notes on blur and tab changes',async()=>{
 const elements=new Map(),listeners={},calls=[];
 const node=()=>{const state=new Set();return {children:[],get firstElementChild(){return this.children[0];},style:{},state,classList:{add:name=>state.add(name),remove:name=>state.delete(name)},append(...children){this.children.push(...children);},setAttribute(){},setPointerCapture(id){this.captured=id;},hasPointerCapture(id){return this.captured===id;}};};
 const h={
  $:id=>{if(!elements.has(id))elements.set(id,node());return elements.get(id);},currentTab:'instruments',errorText:String,
  document:{createElement:node,hasFocus:()=>true,querySelector:()=>null},window:{addEventListener(name,fn){listeners[name]=fn;}},
  nativeInstrumentInput:async(...input)=>calls.push(input),nativeInstrumentNote:async(...note)=>calls.push(note),
 };
 vm.createContext(h);vm.runInContext(fs.readFileSync('web/js/studio-instrument.js','utf8'),h);
 const event=(type,key,code,repeat=false)=>({type,key,code,repeat,preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;}});
 h.$('instrumentKeyboard').checked=true;h.$('instrumentKeyboard').onchange();
 const press=event('keydown','a','KeyA'),shortcut={...event('keydown','s','KeyS'),ctrlKey:true};
 for(const key of [press,shortcut]){
  listeners.keydown(key);assert.equal(key.stopped,undefined);assert.equal(key.prevented,undefined);
 }
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
 const escape=event('keydown','Escape','Escape');listeners.keydown(escape);
 assert.equal(escape.stopped,undefined);assert.equal(escape.prevented,undefined);
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls,[[true,true],[60,100],[60,0],[62,100],[false,true],[true,true],[64,100],[false,false],[false,false],[true,true],[60,100],[60,0],[true,true]]);
 assert.equal(h.$('instrumentKeyboard').checked,false);
 const key=h.$('instrumentPiano').children[0],pointer={button:0,pointerId:7};
 key.onpointerdown(pointer);
 assert.equal(key.captured,7);
 assert.ok(key.state.has('played'));
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls.at(-1),[60,100]);
 const next=h.$('instrumentPiano').children[2];
 h.document.elementFromPoint=()=>({closest:()=>next});
 key.onpointermove(pointer);
 assert.ok(!key.state.has('played'));
 assert.ok(next.state.has('played'));
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls.slice(-2),[[60,0],[62,100]]);
 key.onlostpointercapture(pointer);
 assert.ok(!next.state.has('played'));
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls.at(-1),[62,0]);
 key.onpointerdown(pointer);
 const down={...event('keydown','z','KeyZ'),target:{closest:selector=>selector.startsWith('input,')}};
 listeners.keydown(down);
 assert.equal(down.prevented,undefined);
 h.$('instrumentKeyboard').checked=true;
 listeners.keydown(down);
 assert.equal(down.prevented,true);
 listeners.keydown(event('keydown','z','KeyZ',true));
 assert.equal(h.$('instrumentOctave').textContent,'Octave 3');
 assert.equal(key.firstElementChild.textContent,'C3');
 key.onpointerup(pointer);
 key.onpointerdown(pointer);
 key.onpointerup(pointer);
 listeners.keydown({...event('keydown','x','KeyX'),target:{closest:()=>true}});
 assert.equal(h.$('instrumentOctave').textContent,'Octave 3');
 listeners.keydown({...down,key:'x',code:'KeyX'});
 assert.equal(h.$('instrumentOctave').textContent,'Octave 4');
 for(let i=0;i<7;i++)listeners.keydown(event('keydown','x','KeyX'));
 assert.equal(h.$('instrumentOctave').textContent,'Octave 8');
 h.$('instrumentPiano').children.at(-1).onpointerdown(pointer);
 h.$('instrumentPiano').children.at(-1).onpointerup(pointer);
 await vm.runInContext('instrumentBridge',h);
 assert.deepEqual(calls.slice(-6),[[60,100],[60,0],[48,100],[48,0],[124,100],[124,0]]);
});
