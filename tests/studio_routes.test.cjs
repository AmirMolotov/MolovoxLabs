const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(require('node:path').join(__dirname,'../dist/studio.js'),'utf8');
const start=source.indexOf('function deviceChoiceChanged()');
const end=source.indexOf("$('refresh').onclick",start);
function routingHarness(){
 const state={ready:true,routeApplying:false,busy:false,routeDirty:false,routeTimer:null,wantListening:true,active:true,setTimeout,clearTimeout,choices:{Inputs:[{ID:'mic'}],Outputs:['headphones']},saved:0,stops:0,starts:0,
  async save(){this.saved++;},selections(){return state.choices;},async stop(){state.stops++;state.active=false;},async start(){state.starts++;state.active=true;},message(){}};
 state.save=async()=>{state.saved++;};
 vm.createContext(state);vm.runInContext(source.slice(start,end),state);return state;
}
test('checking an output saves and switches routing without Apply',async()=>{
 const h=routingHarness();h.routeDirty=true;await h.applyRouting();
 assert.equal(h.saved,1);assert.equal(h.stops,1);assert.equal(h.starts,1);
});
test('removing final output stops; adding one resumes requested listening',async()=>{
 const h=routingHarness();h.choices.Outputs=[];await h.applyRouting();
 assert.equal(h.active,false);assert.equal(h.starts,0);
 h.choices.Outputs=['cable'];await h.applyRouting();assert.equal(h.active,true);
});
test('device changes respect an explicit user pause',async()=>{
 const h=routingHarness();h.wantListening=false;h.active=false;await h.applyRouting();
 assert.equal(h.saved,1);assert.equal(h.starts,0);
});
