const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
function harness(){
 const controls=new Map();let id=0;
 const h={micFilters:[{ID:'reverb',Kind:'reverb'},{ID:'echo',Kind:'echo'}],library:{Boards:[]},devices:{Outputs:[]},configured:0,rendered:0,
  crypto:{randomUUID:()=>String(++id)},structuredClone,
  $:key=>{if(!controls.has(key))controls.set(key,{textContent:'',value:'',checked:true,style:{},listeners:{},addEventListener(name,handler){this.listeners[name]=handler;}});return controls.get(key);},
  configure:async()=>{h.configured++;},window:{listeners:{},addEventListener(name,handler){this.listeners[name]=handler;}},document:{listeners:{},querySelector:()=>null,addEventListener(name,handler){(this.listeners[name]??=[]).push(handler);}},filterTitle:f=>f.Kind,
 };
 vm.createContext(h);vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/js/studio-patch.js'),'utf8'),h);
 vm.runInContext('renderPatch=()=>{};drawPatchCables=()=>{};patchGraph=defaultPatch("mic",true)',h);
 return h;
}
test('soundboard preset connects effects to sounds and keeps microphone on a dry cable',()=>{
 const h=harness();h.$('patchBoardPreset').onclick();const graph=h.patchOptions();
 const board=graph.Nodes.find(n=>n.Kind==='board'),mic=graph.Nodes.find(n=>n.Kind==='mic'),out=graph.Nodes.find(n=>n.Kind==='output'),fx=graph.Nodes.filter(n=>n.Kind==='effect');
 assert.equal(graph.Enabled,true);assert.ok(graph.Cables.some(c=>c.From===board.ID&&c.To===fx[0].ID));assert.ok(graph.Cables.some(c=>c.From===fx[1].ID&&c.To===out.ID));assert.ok(graph.Cables.some(c=>c.From===mic.ID&&c.To===out.ID));assert.equal(h.configured,1);
});
test('wheel zoom preserves the point under the pointer and Ctrl-wheel only pans',()=>{
 const h=harness(),viewport=h.$('patchViewport'),stage=h.$('patchStage');Object.assign(viewport,{getBoundingClientRect:()=>({left:10,top:20})});vm.runInContext('patchCameraX=-200;patchCameraY=-100',h);stage.style={};let prevented=0;
 h.patchWheel({clientX:110,clientY:120,deltaY:-100,preventDefault(){prevented++;}});const zoom=vm.runInContext('patchZoom',h);assert.ok(zoom>1);assert.ok(Math.abs((100-vm.runInContext("patchCameraX",h))/zoom-300)<1e-6);assert.ok(Math.abs((100-vm.runInContext("patchCameraY",h))/zoom-200)<1e-6);
 const oldLeft=vm.runInContext("patchCameraX",h),oldTop=vm.runInContext("patchCameraY",h);h.patchWheel({ctrlKey:true,deltaY:80,preventDefault(){prevented++;}});assert.equal(vm.runInContext("patchCameraX",h),oldLeft-160);assert.equal(vm.runInContext("patchCameraY",h),oldTop);assert.equal(vm.runInContext('patchZoom',h),zoom);assert.equal(prevented,2);
});
test('a pad can create its own effect chain without changing microphone effects',()=>{
 const h=harness();h.library.Boards=[{ID:'board',Sounds:[{ID:'clip-one',Name:'Clip one'}]}];h.filterDefaults={gain:{GainDB:6}};h.filterNames={gain:'Gain'};h.renderFilters=()=>{};h.renderGroups=()=>{};
 vm.runInContext('patchGraph.Preset="";patchGraph.Nodes.push(patchNode("clip",{ClipID:"clip-one"},40,450));',h);const source=h.patchOptions().Nodes.find(n=>n.Kind==='clip');h.attachClipEffect(source.ID,'gain');const graph=h.patchOptions(),filter=h.micFilters.at(-1),effect=graph.Nodes.find(n=>n.FilterID===filter.ID);assert.equal(filter.ClipID,'clip-one');assert.ok(graph.Cables.some(c=>c.From===source.ID&&c.To===effect.ID));assert.ok(graph.Cables.some(c=>c.From===effect.ID&&graph.Nodes.some(n=>n.ID===c.To&&n.Kind==='output')));
 h.$('patchMicPreset').onclick();const preset=h.patchOptions(),mic=preset.Nodes.find(n=>n.Kind==='mic'),owned=preset.Nodes.find(n=>n.FilterID===filter.ID);assert.ok(!preset.Cables.some(c=>c.From===mic.ID&&c.To===owned.ID));assert.ok(preset.Nodes.some(n=>n.Kind==='clip'&&n.ClipID==='clip-one'));
});
test('cable connection rejects cycles and duplicate cables without applying audio changes',()=>{
 const h=harness(),graph=h.patchOptions(),fx=graph.Nodes.filter(n=>n.Kind==='effect');
 h.connectPatch(fx[1].ID,fx[0].ID);assert.equal(h.configured,0);assert.match(h.$('patchStatus').textContent,/feedback loop/);
 const edge=graph.Cables[0];h.connectPatch(edge.From,edge.To);assert.equal(h.configured,0);
 const board=graph.Nodes.find(n=>n.Kind==='board');h.connectPatch(board.ID,fx[0].ID);assert.equal(h.configured,1);
});
test('removing an effect definition removes dangling cables and saved layout is a copy',()=>{
 const h=harness(),first=h.patchOptions(),removed=first.Nodes.find(n=>n.FilterID==='reverb');h.micFilters.shift();const saved=h.patchOptions();
 assert.ok(!saved.Nodes.some(n=>n.ID===removed.ID));assert.ok(!saved.Cables.some(c=>c.From===removed.ID||c.To===removed.ID));saved.Nodes[0].X=999;
 assert.notEqual(h.patchOptions().Nodes[0].X,999);
});
function effectRoute(graph){
 let current=graph.Nodes.find(n=>n.Kind==='board').ID;const route=[];
 for(let i=0;i<graph.Nodes.length;i++){const edge=graph.Cables.find(c=>c.From===current);if(!edge)break;const node=graph.Nodes.find(n=>n.ID===edge.To);if(node.Kind==='output')return route;route.push(node.FilterID);current=node.ID;}
 throw new Error('Soundboard route is broken');
}
test('soundboard preset immediately wires added effects and repairs removal/reordering',()=>{
 const h=harness();h.$('patchBoardPreset').onclick();h.micFilters.push({ID:'chorus',Kind:'chorus'});
 assert.deepEqual(effectRoute(h.patchOptions()),['reverb','echo','chorus']);
 h.micFilters.splice(1,1);assert.deepEqual(effectRoute(h.patchOptions()),['reverb','chorus']);
 h.micFilters.reverse();assert.deepEqual(effectRoute(h.patchOptions()),['chorus','reverb']);
});
test('saved and legacy soundboard quick-start layouts update without switching presets',()=>{
 const h=harness();h.$('patchBoardPreset').onclick();const legacy=h.patchOptions();legacy.Preset='';legacy.Version=0;
 h.micFilters.push({ID:'gain',Kind:'gain'});h.initPatch({Options:{Patch:legacy}});
 const restored=h.patchOptions();assert.equal(restored.Preset,'board');assert.deepEqual(effectRoute(restored),['reverb','echo','gain']);
 h.initPatch({Options:{Patch:structuredClone(restored)}});h.micFilters.push({ID:'robot',Kind:'robot'});
 assert.deepEqual(effectRoute(h.patchOptions()),['reverb','echo','gain','robot']);
});
test('manual cable edits leave quick-start mode and retain the chosen custom routing',()=>{
 const h=harness(),graph=h.patchOptions(),board=graph.Nodes.find(n=>n.Kind==='board'),fx=graph.Nodes.find(n=>n.Kind==='effect');
 h.connectPatch(board.ID,fx.ID);const custom=h.patchOptions();assert.equal(custom.Preset,'');
 h.micFilters.push({ID:'chorus',Kind:'chorus'});assert.equal(h.patchOptions().Nodes.filter(n=>n.Kind==='effect').length,2);
 assert.ok(h.patchOptions().Cables.some(c=>c.From===board.ID&&c.To===fx.ID));
});
test('an explicitly custom saved layout is not mistaken for a legacy quick-start layout',()=>{
 const h=harness();h.$('patchBoardPreset').onclick();const custom=h.patchOptions();custom.Preset='';
 h.initPatch({Options:{Patch:custom}});h.micFilters.push({ID:'chorus',Kind:'chorus'});
 const saved=h.patchOptions();assert.equal(saved.Preset,'');assert.equal(saved.Nodes.filter(n=>n.Kind==='effect').length,2);
});
test('right-click cancels a pending cable without changing or applying the graph',()=>{
 const h=harness(),before=h.patchOptions();vm.runInContext('patchPending=patchGraph.Nodes[0].ID;patchPointer={x:300,y:200}',h);
 let prevented=false;h.$('patchStage').oncontextmenu({preventDefault(){prevented=true;},stopPropagation(){}});
 assert.equal(prevented,true);assert.equal(vm.runInContext('patchPending',h),null);assert.equal(vm.runInContext('patchPointer',h),null);
 assert.deepEqual(h.patchOptions(),before);assert.equal(h.configured,0);
});
test('right-click cancels a node move and restores its previous position',()=>{
 const h=harness(),before=h.patchOptions();vm.runInContext('patchDragging={node:patchGraph.Nodes[0],card:{style:{}},startX:40,startY:60};patchDragging.node.X=400;patchDragging.node.Y=300',h);
 h.$('patchStage').oncontextmenu({preventDefault(){},stopPropagation(){}});
 assert.equal(vm.runInContext('patchDragging',h),null);assert.deepEqual(h.patchOptions(),before);assert.equal(h.configured,0);
});
test('releasing the right mouse button cannot finish a cable or save a node move',()=>{
 const h=harness();vm.runInContext('patchPending=patchGraph.Nodes[0].ID',h);
 h.window.listeners.pointerup({button:2});assert.equal(h.configured,0);assert.notEqual(vm.runInContext('patchPending',h),null);
});

test('Space and primary drag pan the viewport without moving nodes or applying routing',()=>{
 const h=harness(),viewport=h.$('patchViewport'),before=h.patchOptions();
 h.currentTab='patch';
 let captured=false,prevented=false;
 Object.assign(viewport,{scrollLeft:200,scrollTop:150,classList:{add(){},remove(){},toggle(){}},
  setPointerCapture(){captured=true;},hasPointerCapture(){return captured;},releasePointerCapture(){captured=false;}});
 const key={code:'Space',target:{closest:selector=>selector.includes('button')},preventDefault(){prevented=true;}};
 for(const handler of h.document.listeners.keydown)handler(key);
 assert.equal(prevented,false);
 viewport.listeners.pointerdown({button:0,pointerId:5,clientX:80,clientY:50,currentTarget:viewport,preventDefault(){},stopPropagation(){}});
 assert.equal(captured,true);
 h.window.listeners.pointermove({pointerId:5,clientX:50,clientY:10});
 assert.equal(vm.runInContext("patchCameraX",h),-30);assert.equal(vm.runInContext("patchCameraY",h),-40);
 h.window.listeners.pointerup({button:0,pointerId:5});assert.equal(captured,false);
 h.document.listeners.keyup[0](key);assert.equal(prevented,true);
 assert.deepEqual(h.patchOptions(),before);assert.equal(h.configured,0);
});
test('duplicate effect nodes get independent settings and no inherited shortcuts or cables',()=>{
 const h=harness();
 h.renderFilters=()=>{};h.renderGroups=()=>{};
 Object.assign(h.micFilters[0],{Mix:40,Hotkeys:[{Key:65}]});
 const before=h.patchOptions();
 h.addPatchEffectInstance(h.micFilters[0],800,100);
 const duplicate=h.micFilters.at(-1),after=h.patchOptions();
 assert.notEqual(duplicate.ID,h.micFilters[0].ID);
 assert.equal(duplicate.Mix,40);assert.equal(duplicate.Hotkeys.length,0);
 duplicate.Mix=65;assert.equal(h.micFilters[0].Mix,40);
 assert.ok(after.Nodes.some(n=>n.FilterID===duplicate.ID));
 assert.deepEqual(after.Cables,before.Cables);
});

test('full-screen patch view fits nodes and restores view, focus and background on exit',()=>{
 const h=harness(),viewport=h.$('patchViewport'),stage=h.$('patchStage'),workspace=h.$('patchWorkspace');
 const background={tagName:'ASIDE',inert:false};
 let restored=false,focused=false;
 const signals={before(){},tagName:'DIV'};
 let signalsRestored=false;
 const marker={replaceWith(node){restored=node===workspace;}};
 const signalMarker={replaceWith(node){signalsRestored=node===signals;}};
 h.document.createComment=name=>name==='shared signals'?signalMarker:marker;
 h.document.querySelector=selector=>selector==='.signal-dock'?signals:null;
 h.document.activeElement={isConnected:true,focus(){focused=true;}};
 h.document.body={children:[background],classList:{add(){},remove(){}},append(){}};
 Object.assign(workspace,{before(node){assert.equal(node,marker);},querySelector(){return {after(node){assert.equal(node,signals);}};}});
 for(const id of ['patchViewport','patchFocus']){
  Object.assign(h.$(id),{classList:{toggle(){},remove(){}},focus(){},setAttribute(){}});
 }
 const cards=h.patchOptions().Nodes.map(node=>({offsetLeft:node.X,offsetTop:node.Y,offsetWidth:210,offsetHeight:150}));
 h.$('patchNodes').children=cards;
 Object.assign(viewport,{scrollLeft:120,scrollTop:80,clientWidth:1000,clientHeight:600});
 stage.style={};
 vm.runInContext('patchCameraX=-120;patchCameraY=80',h);
 h.requestAnimationFrame=callback=>callback();
 h.togglePatchFocus(true);
 assert.equal(background.inert,true);
 assert.ok(vm.runInContext('patchZoom',h)>0);
 assert.equal(h.$('patchFocus').textContent,'Exit full screen');
 assert.equal(h.configured,0);
 h.togglePatchFocus(false);
 assert.equal(background.inert,false);
 assert.equal(vm.runInContext("patchCameraX",h),-120);
 assert.equal(vm.runInContext("patchCameraY",h),80);
 assert.equal(vm.runInContext('patchZoom',h),1);
 assert.ok(restored&&focused&&signalsRestored);
 assert.equal(h.$('patchFocus').textContent,'Full screen');
});
test('pressing empty space starts selection while Space-drag remains reserved for panning',()=>{
 const h=harness(),viewport=h.$('patchViewport');let selected=false;
 h.beginPatchSelection=()=>selected=true;
 viewport.listeners.pointerdown({button:0,pointerId:8,target:h.$('patchStage'),currentTarget:viewport,clientX:100,clientY:100,preventDefault(){},stopPropagation(){}});
 assert.equal(selected,true);assert.equal(vm.runInContext('patchPan',h),null);assert.equal(vm.runInContext('patchCameraX',h),0);assert.equal(h.configured,0);
});

test('camera can pan through zero and far beyond either edge without moving graph nodes',()=>{
 const h=harness(),viewport=h.$('patchViewport'),before=h.patchOptions();
 vm.runInContext('patchSpace=true',h);
 Object.assign(viewport,{classList:{add(){},remove(){}},setPointerCapture(){},hasPointerCapture(){return false;}});
 viewport.listeners.pointerdown({button:0,pointerId:9,target:h.$('patchStage'),currentTarget:viewport,clientX:0,clientY:0,preventDefault(){},stopPropagation(){}});
 for(const position of [-100000,100000,-250000]){
  h.window.listeners.pointermove({pointerId:9,clientX:position,clientY:-position});
  assert.equal(vm.runInContext('patchCameraX',h),position);
  assert.equal(vm.runInContext('patchCameraY',h),-position);
 }
 h.window.listeners.pointerup({button:0,pointerId:9});
 assert.deepEqual(h.patchOptions(),before);
});
test('effect instances and nodes can be added beyond the old eight-effect and 32-node limits',()=>{
 const h=harness();
 h.renderFilters=()=>{};h.renderGroups=()=>{};
 for(let i=0;i<40;i++)h.addPatchEffectInstance({Kind:'gain',GainDB:0,Enabled:true},100+i*40,-12000);
 assert.equal(h.micFilters.length,42);
 assert.ok(h.patchOptions().Nodes.length>32);
});
