const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
function harness(){
 const controls=new Map();let id=0;
 const h={micFilters:[{ID:'reverb',Kind:'reverb'},{ID:'echo',Kind:'echo'}],library:{Boards:[]},devices:{Outputs:[]},configured:0,rendered:0,
  crypto:{randomUUID:()=>String(++id)},structuredClone,
  $:key=>{if(!controls.has(key))controls.set(key,{textContent:'',value:'',checked:true});return controls.get(key);},
  configure:async()=>{h.configured++;},window:{listeners:{},addEventListener(name,handler){this.listeners[name]=handler;}},document:{addEventListener(){}},filterTitle:f=>f.Kind,
 };
 vm.createContext(h);vm.runInContext(fs.readFileSync(path.join(__dirname,'../dist/studio-patch.js'),'utf8'),h);
 vm.runInContext('renderPatch=()=>{};drawPatchCables=()=>{};patchGraph=defaultPatch("mic",true)',h);
 return h;
}
test('soundboard preset connects effects to sounds and keeps microphone on a dry cable',()=>{
 const h=harness();h.$('patchBoardPreset').onclick();const graph=h.patchOptions();
 const board=graph.Nodes.find(n=>n.Kind==='board'),mic=graph.Nodes.find(n=>n.Kind==='mic'),out=graph.Nodes.find(n=>n.Kind==='output'),fx=graph.Nodes.filter(n=>n.Kind==='effect');
 assert.equal(graph.Enabled,true);assert.ok(graph.Cables.some(c=>c.From===board.ID&&c.To===fx[0].ID));assert.ok(graph.Cables.some(c=>c.From===fx[1].ID&&c.To===out.ID));assert.ok(graph.Cables.some(c=>c.From===mic.ID&&c.To===out.ID));assert.equal(h.configured,1);
});
test('wheel zoom preserves the point under the pointer and Ctrl-wheel only pans',()=>{
 const h=harness(),viewport=h.$('patchViewport'),stage=h.$('patchStage');Object.assign(viewport,{scrollLeft:200,scrollTop:100,getBoundingClientRect:()=>({left:10,top:20})});stage.style={};let prevented=0;
 h.patchWheel({clientX:110,clientY:120,deltaY:-100,preventDefault(){prevented++;}});const zoom=vm.runInContext('patchZoom',h);assert.ok(zoom>1);assert.ok(Math.abs((viewport.scrollLeft+100)/zoom-300)<1e-6);assert.ok(Math.abs((viewport.scrollTop+100)/zoom-200)<1e-6);
 const oldLeft=viewport.scrollLeft,oldTop=viewport.scrollTop;h.patchWheel({ctrlKey:true,deltaY:80,preventDefault(){prevented++;}});assert.equal(viewport.scrollLeft,oldLeft+160);assert.equal(viewport.scrollTop,oldTop);assert.equal(vm.runInContext('patchZoom',h),zoom);assert.equal(prevented,2);
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
