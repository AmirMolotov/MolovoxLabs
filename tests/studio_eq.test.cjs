const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const context=vm.createContext({});
vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../dist/studio-eq.js'),'utf8'),context);

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
