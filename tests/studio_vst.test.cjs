const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const path=require('node:path');
class Element {
 constructor(text='',value=''){this.text=text;this.value=value;this.options=[];this.disabled=false;this.textContent='';}
 replaceChildren(...children){this.options=children;}
 add(option){this.options.push(option);}
}
function harness(){
 const controls=new Map();const element=id=>{if(!controls.has(id))controls.set(id,new Element());return controls.get(id);};
 const plugin={Name:'Example FX',Vendor:'Example',Path:'C:\\Plugins\\Example.vst3',ClassID:'0123456789abcdef0123456789abcdef'};
 const h={micFilters:[],ready:true,currentTab:'filters',configured:0,saved:0,scanned:null,lastMessage:'',
  $:element,Option:Element,filterNames:{gain:'Gain'},crypto:{randomUUID:()=> 'new-filter'},
  async nativeLoadVSTPlugins(){return {Plugins:[plugin]};},
  async nativeScanVST(folders){h.scanned=folders;},
  async nativeVSTScanStatus(){return {Running:false,Result:{Plugins:[plugin],Errors:['Broken plugin: scan failed']}};},
  beginLoading(){return ()=>{};},async paintLoading(){},async withLoading(label,task){return task();},async save(){h.saved++;},async configure(){h.configured++;},renderFilters(){},renderGroups(){},
  document:{querySelectorAll:()=>[]},message(text){h.lastMessage=text;},errorText:e=>e.message||String(e),setInterval(){},setTimeout,clearTimeout,queueMicrotask,
 };
 vm.createContext(h);vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/js/studio-vst.js'),'utf8'),h);
 return {h,element,plugin};
}
test('selecting a scanned plugin adds it immediately and preserves its identity',async()=>{
 const {h,element,plugin}=harness();await h.initVST({VSTFolders:['D:\\Effects']});
 assert.equal(element('vstSelect').disabled,false);element('vstSelect').value='0';await element('vstSelect').onchange();
 assert.equal(h.micFilters.length,1);const filter=h.micFilters[0];assert.equal(filter.Kind,'vst3');assert.equal(filter.PluginPath,plugin.Path);assert.equal(filter.ClassID,plugin.ClassID);assert.equal(filter.Enabled,true);
 assert.equal(h.configured,1);assert.equal(h.saved,1);assert.equal(element('vstSelect').value,'');assert.equal(h.filterTitle(filter),'Example FX');
});
test('plugin selection supports more than eight effects',async()=>{
 const {h,element}=harness();await h.initVST({});for(let i=0;i<8;i++)h.micFilters.push({Kind:'gain'});
 element('vstSelect').value='0';await element('vstSelect').onchange();assert.equal(h.micFilters.length,9);assert.equal(h.configured,1);assert.equal(h.lastMessage,'');
});
test('scan uses custom folders and exposes isolated failures without hiding valid plugins',async()=>{
 const {h,element}=harness();element('vstFolders').value=' D:\\Effects \n\n E:\\VST3';await element('scanVST').onclick();
 assert.deepEqual(Array.from(h.scanned),['D:\\Effects','E:\\VST3']);assert.match(element('vstScanStatus').textContent,/Broken plugin/);assert.equal(element('vstSelect').disabled,false);assert.equal(element('scanVST').disabled,false);
});
