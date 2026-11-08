const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function harness(){
 const elements=new Map(),listeners={},timers=new Map();let next=0;
 const element=()=>({hidden:false,textContent:'',value:'board',inert:false,querySelectorAll:()=>[],classList:{remove(){}},onclick:null});
 const h={console,crypto,Option:function(){},library:{Boards:[{ID:'board',Sounds:[{ID:'a',Name:'Victory shout'},{ID:'b',Name:'Victory music'},{ID:'c',Name:'Defeat'}]}]},currentTab:'board',active:true,playingSounds:new Map(),padViews:new Map(),calls:[],
  $:id=>{if(!elements.has(id))elements.set(id,element());return elements.get(id);},
  document:{addEventListener:(n,fn)=>listeners[n]=fn,querySelector:()=>null},window:{addEventListener(){}},
  setTimeout:(fn,delay)=>{timers.set(++next,{fn,delay});return next;},clearTimeout:id=>timers.delete(id),
  nativePlaySound:async id=>h.calls.push(id),start:async()=>true,message(){},errorText:String,
 };
 for(const s of h.library.Boards[0].Sounds)h.padViews.set(s.ID,{pad:element()});
 vm.createContext(h);vm.runInContext(fs.readFileSync('dist/studio-board-layout.js','utf8'),h);
 h.key=(key,other={})=>{const e={key,target:{closest:()=>null},preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;},...other};listeners.keydown(e);return e;};
 h.run=async delay=>{for(const [id,t] of [...timers])if(t.delay===delay){timers.delete(id);await t.fn();}await new Promise(r=>setImmediate(r));};
 return h;
}
test('drag reorder can move a pad into a group and back without duplicating it',()=>{
 const h=harness(),b=h.library.Boards[0];h.moveBoardPad(b,'c','g','a');assert.deepEqual(b.Sounds.map(s=>s.ID),['c','a','b']);assert.equal(b.Sounds[0].GroupID,'g');h.moveBoardPad(b,'c','');assert.deepEqual(b.Sounds.map(s=>s.ID),['a','b','c']);assert.equal(b.Sounds[2].GroupID,'');h.moveBoardPad(b,'c','','c');assert.equal(b.Sounds.length,3);
});
test('typeahead filters case insensitively and plays a unique match once',async()=>{
 const h=harness();for(const key of 'VICTORY')h.key(key);await h.run(280);assert.deepEqual(h.calls,[]);assert.equal(h.padViews.get('c').pad.hidden,true);
 for(const key of ' s')h.key(key);await h.run(280);assert.deepEqual(h.calls,['a']);for(const key of 'hout')h.key(key);await h.run(280);assert.deepEqual(h.calls,['a']);
 const escape=h.key('Escape');assert.equal(escape.stopped,true);assert.equal(h.$('boardSearchOverlay').hidden,true);assert.equal(h.padViews.get('c').pad.hidden,false);
});
test('search does not capture text fields, dialogs, other tabs, hotkeys or composition',async()=>{
 const h=harness();for(const other of [{ctrlKey:true},{altKey:true},{metaKey:true},{isComposing:true},{target:{closest:()=>({})}}])assert.equal(h.key('D',other).prevented,undefined);
 h.currentTab='sampler';assert.equal(h.key('D').prevented,undefined);h.currentTab='board';h.document.querySelector=()=>({});assert.equal(h.key('D').prevented,undefined);await h.run(280);assert.deepEqual(h.calls,[]);
});
test('cancelling or switching boards before debounce prevents playback',async()=>{
 const h=harness();h.key('D');h.key('Escape');await h.run(280);assert.deepEqual(h.calls,[]);h.key('D');h.$('boardSelect').value='other';await h.run(280);assert.deepEqual(h.calls,[]);
});
test('search never toggles off an already playing result; backspace and timeout restore pads',async()=>{
 const h=harness();h.playingSounds.set('c',{});h.key('D');await h.run(280);assert.deepEqual(h.calls,[]);h.key('Backspace');assert.equal(h.$('boardSearchOverlay').hidden,true);h.key('x');assert.equal(h.$('boardSearchCount').textContent,'0 matches · Escape to clear');await h.run(3000);assert.equal(h.padViews.get('a').pad.hidden,false);
});
test('search begun before audio startup must still match after startup',async()=>{
 const h=harness();h.active=false;let release;h.start=()=>new Promise(r=>release=r);h.key('D');const pending=h.run(280);await new Promise(r=>setImmediate(r));h.clearBoardSearch();release(true);await pending;assert.deepEqual(h.calls,[]);
});
test('group headers collapse with animation state, persist, and keep pad playback controls',async()=>{
 const h=harness();
 class El{
  constructor(tag='div'){this.tag=tag;this.children=[];this.dataset={};this.style={};this.handlers={};this.classes=new Set();this.classList={add:c=>this.classes.add(c),remove:c=>this.classes.delete(c),toggle:(c,on)=>on?this.classes.add(c):this.classes.delete(c)};}
  set className(v){this._className=v;this.classes=new Set(v.split(' '));}get className(){return this._className;}
  append(...nodes){for(const n of nodes){if(n.parent)n.parent.children=n.parent.children.filter(c=>c!==n);n.parent=this;this.children.push(n);}}
  prepend(n){n.parent=this;this.children.unshift(n);}
  insertBefore(n,b){n.parent=this;this.children.splice(this.children.indexOf(b),0,n);}
  replaceChildren(...n){this.children=[];this.append(...n);}
  add(n){this.append(n);}setAttribute(n,v){this[n]=v;}
  addEventListener(n,fn){this.handlers[n]=fn;}
  get lastElementChild(){return this.children.at(-1);}
  querySelector(selector){const cls=selector.slice(1);for(const c of this.children){if(c.classes?.has(cls))return c;const nested=c.querySelector?.(selector);if(nested)return nested;}return null;}
  click(){this.onclick?.();}
 }
 h.document.createElement=tag=>new El(tag);const root=new El();const old=h.$;h.$=id=>id==='soundPads'?root:old(id);
 h.renderPads=()=>{};h.nativeSaveBoardLayout=async()=>h.library;h.nativeLibrary=async()=>h.library;
 const b=h.library.Boards[0];b.Groups=[{ID:'g',Name:'Reactions',Collapsed:false}];b.Sounds[0].GroupID='g';
 for(const s of b.Sounds){const pad=new El('article');pad.className='sound-pad';const top=new El();top.className='sound-pad-top';top.append(new El('span'),new El('button'));const play=new El('button');play.className='sound-pad-play';pad.append(top,play);h.padViews.set(s.ID,{pad});}
 h.decorateSoundboard(b);assert.equal(root.children.length,2);const section=root.children[1],toggle=section.querySelector('.pad-group-toggle');assert.equal(section.querySelector('.board-pad-grid').children.length,1);assert.ok(section.querySelector('.sound-pad-play'));
 toggle.click();await new Promise(r=>setImmediate(r));assert.equal(b.Groups[0].Collapsed,true);assert.equal(section.classes.has('is-collapsed'),true);assert.equal(section.querySelector('.pad-group-body').inert,true);
 toggle.click();await new Promise(r=>setImmediate(r));assert.equal(b.Groups[0].Collapsed,false);assert.equal(section.classes.has('is-collapsed'),false);
});
