const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const context=vm.createContext({});
vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../web/js/studio-eq.js'),'utf8'),context);

test('EQ frequency and gain coordinates cover the full draggable range',()=>{
 for(const hz of [20,120,350,1000,8000,20000]){
  assert.ok(Math.abs(vm.runInContext('eqFrequency(eqX('+hz+'))',context)-hz)<1e-8);
 }
 assert.equal(vm.runInContext('eqGain(-100)',context),24);
 assert.equal(vm.runInContext('eqGain(400)',context),-24);
});

test('EQ curve matches boost and cut at a movable band center and reflects width',()=>{
 for(const GainDB of [-24,-6,0,12,24]){
  assert.ok(Math.abs(context.eqResponse({Frequency:350,GainDB,Q:2},350)-GainDB)<1e-8);
 }
 const wide=context.eqResponse({Frequency:1000,GainDB:12,Q:.3},2000);
 const narrow=context.eqResponse({Frequency:1000,GainDB:12,Q:8},2000);
 assert.ok(wide>narrow);
});

test('EQ editor preserves old gains and treats removing every band as a flat response',()=>{
 const filter={BassDB:6,MidDB:-3,TrebleDB:2};
 const migrated=context.equalizerBands(filter);
 assert.deepEqual(Array.from(migrated,b=>b.GainDB),[6,-3,2]);
 assert.deepEqual(Array.from(migrated,b=>b.Frequency),[120,1000,8000]);
 assert.equal(filter.EQBands,undefined);
 assert.equal(context.equalizerBands({EQBands:[]}).length,0);
});
function eqEditorHarness(filter){
 class Element{constructor(){this.style={};this.dataset={};this.children=[];this.attrs={};this.events={};this.selectors=new Map();}append(...items){this.children.push(...items);}replaceChildren(...items){this.children=items;}setAttribute(k,v){this.attrs[k]=v;if(k==='data-band')this.dataset.band=String(v);}querySelector(key){if(!this.selectors.has(key))this.selectors.set(key,new Element());return this.selectors.get(key);}querySelectorAll(){if(!this.inputs)this.inputs=['Frequency','GainDB','Q'].map(field=>{const input=new Element();input.dataset.eqField=field;return input;});return this.inputs;}addEventListener(name,fn){this.events[name]=fn;}dispatchEvent(e){this.events[e.type]?.();}setPointerCapture(id){this.capture=id;}hasPointerCapture(id){return this.capture===id;}releasePointerCapture(){this.capture=null;}getScreenCTM(){return {inverse:()=>({})};}}
 const editors=[];let configured=0;const h={document:{createElement:()=>new Element(),createElementNS:()=>new Element(),querySelectorAll:()=>editors},Option:function(text,value){this.text=text;this.value=value;},Event:function(type){this.type=type;},configure:()=>configured++,DOMPoint:class{constructor(x,y){this.x=x;this.y=y;}matrixTransform(){return this;}}};vm.createContext(h);vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../web/js/studio-eq.js'),'utf8'),h);const editor=h.renderEqualizer(filter);editors.push(editor);return {h,editor,svg:editor.querySelector('svg'),configured:()=>configured};
}
test('clicking the EQ chart adds a band at the clicked frequency/gain and clicking a point only selects it',()=>{const filter={ID:'eq',EQBands:[]},{svg,editor}=eqEditorHarness(filter);const event=(x,y,index=null)=>({button:0,pointerId:1,clientX:x,clientY:y,target:{closest:()=>index===null?null:{dataset:{band:index}}},preventDefault(){},stopPropagation(){}});svg.onpointerdown(event(175.5,81));assert.equal(filter.EQBands.length,1);assert.ok(Math.abs(filter.EQBands[0].Frequency-632)<2);assert.equal(filter.EQBands[0].GainDB,0);svg.onpointerup({pointerId:1});svg.onpointerdown(event(250,40));svg.onpointerup({pointerId:1});assert.equal(filter.EQBands.length,2);svg.onpointerdown(event(175.5,81,0));assert.equal(filter.EQBands.length,2);assert.equal(editor.querySelector('select').value,'0');svg.onpointerup({pointerId:1});});
test('right-click removes only the targeted EQ band and blank right-click does not remove anything',()=>{const filter={ID:'eq',EQBands:[{Frequency:100,GainDB:3,Q:1},{Frequency:1000,GainDB:-2,Q:2}]},{svg}=eqEditorHarness(filter);const event=index=>({target:{closest:()=>index===null?null:{dataset:{band:index}}},preventDefault(){},stopPropagation(){}});svg.oncontextmenu(event(null));assert.equal(filter.EQBands.length,2);svg.oncontextmenu(event(0));assert.equal(filter.EQBands.length,1);assert.equal(filter.EQBands[0].Frequency,1000);svg.oncontextmenu(event(0));assert.equal(filter.EQBands.length,0);});
test('chart clicks respect the native eight-band capacity and the Add Band button is removed',()=>{const filter={ID:'eq',EQBands:Array.from({length:8},(_,i)=>({Frequency:100*(i+1),GainDB:0,Q:1}))},{svg}=eqEditorHarness(filter);svg.onpointerdown({button:0,pointerId:1,clientX:200,clientY:80,target:{closest:()=>null},preventDefault(){},stopPropagation(){}});assert.equal(filter.EQBands.length,8);assert.doesNotMatch(fs.readFileSync(require('node:path').join(__dirname,'../web/js/studio-eq.js'),'utf8'),/eq-add|\+ Band/);});

test('EQ points lie on the combined response even when neighboring bands overlap',()=>{const filter={ID:'eq',EQBands:[{Frequency:700,GainDB:8,Q:.4},{Frequency:1100,GainDB:7,Q:.5}]},{h,editor}=eqEditorHarness(filter),points=editor.querySelector('.eq-points');for(let i=0;i<2;i++){const point=points.children.find(p=>Number(p.dataset.band)===i);h.bands=filter.EQBands;h.frequency=filter.EQBands[i].Frequency;const y=vm.runInContext('eqY(eqCombinedResponse(bands,frequency))',h);const transform=point.attrs.transform;assert.ok(Math.abs(Number(transform.split(' ')[1].replace(')',''))-y)<1e-8);} });
test('adding and dragging an EQ band compensates for neighbors so its point follows the requested total gain',()=>{const filter={ID:'eq',EQBands:[{Frequency:1000,GainDB:8,Q:.5}]},{h,svg}=eqEditorHarness(filter);h.x=200;h.y=60;const hz=vm.runInContext('Math.round(eqFrequency(x))',h),gain=vm.runInContext('eqGain(y)',h);const event=(x,y,index=null)=>({button:0,pointerId:1,clientX:x,clientY:y,target:{closest:()=>index===null?null:{dataset:{band:index}}},preventDefault(){},stopPropagation(){}});svg.onpointerdown(event(200,60));h.bands=filter.EQBands;h.hz=hz;assert.ok(Math.abs(vm.runInContext('eqCombinedResponse(bands,hz)',h)-gain)<1e-6);svg.onpointermove(event(220,75,0));h.bands=filter.EQBands;const moved=filter.EQBands[1];h.hz=moved.Frequency;h.targetY=75;assert.ok(Math.abs(vm.runInContext('eqCombinedResponse(bands,hz)-eqGain(targetY)',h))<.01);assert.equal(filter.EQBands[0].Frequency,1000);assert.equal(filter.EQBands[0].GainDB,8);svg.onpointerup({pointerId:1});});
