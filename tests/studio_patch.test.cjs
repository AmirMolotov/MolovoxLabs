const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
function harness(){
 const controls=new Map();let id=0;
 const h={micFilters:[{ID:'reverb',Kind:'reverb'},{ID:'echo',Kind:'echo'}],library:{Boards:[]},devices:{Outputs:[]},configured:0,rendered:0,
  crypto:{randomUUID:()=>String(++id)},structuredClone,
  $:key=>{if(!controls.has(key))controls.set(key,{textContent:'',value:'',checked:true});return controls.get(key);},
  configure:async()=>{h.configured++;},window:{addEventListener(){}},document:{addEventListener(){}},filterTitle:f=>f.Kind,
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
