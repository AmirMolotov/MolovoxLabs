const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');

test('mixer retains recent apps, caps history and assigns relative controls and groups apps by drag and drop',async()=>{
 const controls=new Map();
 const element=()=>({
  children:[],options:[],textContent:'',attrs:{},get firstElementChild(){return this.children[0];},
  classList:{toggle(name,on){this[name]=on;}},
  contains(child){return this.children.includes(child);},
  add(option){this.options.push(option);},
  style:{setProperty(){}},
  getAttribute(){return this.dataset.tab;},
  setAttribute(name,value){this.attrs[name]=value;},
  focus(){this.focused=true;},select(){this.selected=true;},
  append(...children){for(const child of children){child.remove?.();child.parentNode=this;this.children.push(child);}},
  replaceChildren(...children){this.children=[];this.options=[...children];this.append(...children);},
  remove(){if(this.parentNode)this.parentNode.children=this.parentNode.children.filter(child=>child!==this);},
  querySelectorAll(){return [];},
  matches(){return false;},
 });
 const $=id=>{
  if(!controls.has(id))controls.set(id,element());
  return controls.get(id);
 };
 const player={Path:'C:\\Music\\player.exe',Name:'player.exe',Volume:65,Playing:true,Icon:'iconBase64'};
 const other={Path:'C:\\Browser\\browser.exe',Name:'browser.exe',Volume:20,Playing:false};
 const writes=[];
 const visiblePaths=[];
 let apps=[player,other],now=0,tick=null,interval=0,scans=0,finishWrite,edited;
 const tabs=['board','appMixer'].map(tab=>Object.assign(element(),{dataset:{tab}}));
 $('appMixer').hidden=true;
 const context=vm.createContext({
  $,document:{createElement:element,querySelectorAll:()=>tabs},performance:{now:()=>now},
  currentTab:'board',active:false,idle(){},preferences:{AppMixerGroups:[],AppMixerShortcuts:[]},
  structuredClone,crypto:{randomUUID:()=>'qa-group'},
  queueSettings:async task=>task(),
  snapshotPreferences:()=>({...context.preferences}),
  nativeSavePreferences:async value=>{context.saved=value;},
  Option:class{constructor(text,value){this.text=text;this.value=String(value);}},
  openTriggerEditor:id=>{edited=id;},
  nativeListAppMixer:async paths=>{scans++;visiblePaths.push(Array.from(paths));return apps;},
  nativeSetAppMixerVolume:async(...args)=>{
   writes.push(args);
   if(writes.length===1)await new Promise(resolve=>{finishWrite=resolve;});
   player.Volume=args[1];
  },
  errorText:String,setInterval:(fn,period)=>{tick=fn;interval=period;},clearInterval:()=>{tick=null;},
 });
 const studio=fs.readFileSync(path.join(__dirname,'../web/js/studio.js'),'utf8');
 vm.runInContext(studio.slice(studio.indexOf('function openTab'),studio.indexOf("document.querySelectorAll('[data-tab]').forEach")),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/js/studio-shortcuts.js'),'utf8'),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/js/studio-app-mixer.js'),'utf8'),context);
 assert.equal(scans,0);
 context.openTab('appMixer');
 assert.equal($('appMixer').hidden,false);
 assert.equal($('board').hidden,true);
 await new Promise(setImmediate);
 const list=$('appMixerList');
 assert.equal(list.children.length,1);
 const [identity,level,slider,scale,actions]=list.children[0].children;
 const [icon,name]=identity.children;
 assert.equal(name.textContent,'player.exe');
 const [image]=icon.children;
 assert.equal(image.src,'data:image/x-icon;base64,iconBase64');
 assert.equal(image.alt,'');
 image.onerror();
 assert.equal(icon.children.length,0);
 assert.deepEqual(visiblePaths,[[]]);
 assert.equal(level.textContent,'65%');
 assert.equal(slider.step,10);
 assert.deepEqual(scale.children.map(mark=>mark.textContent),Array.from({length:11},(_,i)=>`${i*10}%`));
 assert.deepEqual(writes,[]);
 const [activity,,shortcut]=actions.children;
 assert.equal(activity.textContent,'Playing');
 shortcut.value='down';
 shortcut.onchange();
 assert.equal(edited,'@app:down:'+player.Path);
 slider.value='40';
 const pending=slider.oninput();
 assert.deepEqual(writes,[[player.Path,40]]);
 assert.equal(level.textContent,'40%');
 slider.value='20';
 await slider.oninput();
 slider.value='0';
 await slider.oninput();
 assert.equal(level.textContent,'0%');
 await tick();
 assert.equal(level.textContent,'0%');
 finishWrite();
 await pending;
 await tick();
 assert.deepEqual(visiblePaths.at(-1),[player.Path]);
 assert.deepEqual(writes,[[player.Path,40],[player.Path,0]]);
 assert.equal(level.textContent,'0%');
 player.Playing=false;
 now=1000;
 await tick();
 assert.equal(list.children.length,1);
 now=1600;
 await tick();
 assert.equal(list.children.length,1);
 assert.equal(activity.textContent,'Idle');
 assert.equal(list.children[0].classList['is-recent'],true);
 apps=[];
 await tick();
 assert.equal(list.children.length,1);
 assert.equal(activity.textContent,'Session closed');
 assert.equal(slider.disabled,true);
 context.openTab('board');
 assert.equal($('appMixer').hidden,true);
 assert.equal($('board').hidden,false);
 assert.equal(interval,1000);
 await new Promise(setImmediate);
 apps=[other,...Array.from({length:10},(_,i)=>({...other,Path:'C:\\Audio\\app'+i+'.exe',Name:'app'+i+'.exe'}))];
 for(const app of apps)app.Playing=true;
 now=2000;
 await tick();
 assert.equal(list.children.length,10);
 assert.ok(!list.children.some(row=>row.children[0].children[1].textContent===player.Name));
 context.openTab('appMixer');
 assert.equal(interval,250);
 assert.equal(list.children.length,10);
 const [source,second]=list.children;
 const activeRows=()=>list.children.filter(row=>!row.hidden);
 const appPath=row=>row.children[0].children[1].title;
 const pills=$('appMixerGroups'),groupControls=$('appMixerGroupControls');
 await $('appMixerNewGroup').onclick();
 assert.equal(context.saved.AppMixerGroups[0].Paths.length,0);
 assert.equal(pills.children[2].attrs['aria-selected'],'true');
 assert.equal(groupControls.firstElementChild.focused,true);
 groupControls.firstElementChild.value='Music';
 await groupControls.firstElementChild.onchange?.();
 await new Promise(setImmediate);
 assert.equal(pills.children[2].firstElementChild.textContent,'Music');
 pills.children[0].onclick();
 const drag={dataTransfer:{setData(){}},preventDefault(){}};
 for(const row of [source,second]){
  row.children[0].firstElementChild.ondragstart(drag);
  await pills.children[2].ondrop(drag);
 }
 assert.deepEqual(Array.from(context.saved.AppMixerGroups[0].Paths),[appPath(source),appPath(second)]);
 assert.equal(activeRows().length,10);
 assert.equal(source.children[0].children[2].textContent,'Music');
 pills.children[2].onclick();
 assert.deepEqual(activeRows(),[source,second]);
 const groupShortcut=groupControls.children[1];
 groupShortcut.value='knob';groupShortcut.onchange();
 assert.equal(edited,'@app:group-knob:qa-group');
 source.children[0].firstElementChild.ondragstart(drag);
 await pills.children[1].ondrop(drag);
 assert.equal(source.children[0].children[2].textContent,'Ungrouped');
 assert.deepEqual(Array.from(context.saved.AppMixerGroups[0].Paths),[appPath(second)]);
 context.preferences.AppMixerShortcuts=[{ID:'@app:group-knob:qa-group'},{ID:'@app:up:'+appPath(second)}];
 pills.children[2].onclick();
 await groupControls.children[2].onclick();
 assert.equal(context.saved.AppMixerGroups.length,0);
 assert.deepEqual(Array.from(context.saved.AppMixerShortcuts,x=>x.ID),['@app:up:'+appPath(second)]);
 assert.equal(second.children[0].children[2].textContent,'Ungrouped');
 assert.equal(activeRows().length,10);
});
