const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function harness(){
 const elements=new Map(),events={};
 class El{constructor(cls=''){this.className=cls;this.children=[];this.attrs={};this.classes=new Set();this.classList={toggle:(name,on)=>on?this.classes.add(name):this.classes.delete(name)};}
 append(...items){for(const item of items){item.parent?.removeChild(item);item.parent=this;this.children.push(item);}}
 removeChild(item){this.children=this.children.filter(c=>c!==item);}
 insertBefore(item,before){item.parent?.removeChild(item);item.parent=this;const at=this.children.indexOf(before);if(at<0)this.children.push(item);else this.children.splice(at,0,item);}
 get lastElementChild(){return this.children.at(-1);}
 setAttribute(name,value){this.attrs[name]=value;}
 querySelector(selector){const cls=selector.slice(1);for(const c of this.children){if(c.className.split(' ').includes(cls))return c;const found=c.querySelector(selector);if(found)return found;}return null;}}
 const pad=new El('sound-pad'),top=new El('sound-pad-top'),bottom=new El('sound-pad-bottom'),grip=new El('pad-drag-grip'),group=new El('pad-group-select'),remove=new El('pad-remove'),keys=new El('pad-edit'),trim=new El('pad-edit pad-trim'),effects=new El('pad-edit pad-effects'),time=new El('clip-time');
 grip.draggable=true;group.value='group-1';keys.textContent='Ctrl+1 · Shift+F2';trim.onclick=()=>{};effects.onclick=()=>{};top.append(grip,group,remove);bottom.append(keys,trim,effects,time);pad.append(top,bottom);
 const view={pad,sound:{ID:'pad',Name:'Victory'},fill:{style:{width:'42%'}}};
 const h={padViews:new Map([['pad',view]]),document:{createElement:()=>new El(),addEventListener:(name,fn)=>events[name]=fn,querySelector:()=>null,querySelectorAll:()=>[]},$:(id)=>{if(!elements.has(id))elements.set(id,new El());return elements.get(id);},saves:0,scheduleSave:()=>h.saves++};
 vm.createContext(h);vm.runInContext(fs.readFileSync('dist/studio-board-view.js','utf8'),h);return {h,pad,top,bottom,group,grip,keys,trim,effects,view};
}
test('saved Compact view restores without restarting playback or changing the pad and its drag handle',()=>{
 const {h,pad,grip,keys,view}=harness();h.initSoundboardView('compact');assert.equal(h.$('board').classes.has('compact-soundboard'),true);assert.equal(h.$('boardViewCompact').attrs['aria-pressed'],'true');assert.equal(h.$('boardViewStandard').attrs['aria-pressed'],'false');assert.equal(h.saves,0);assert.equal(h.padViews.get('pad').pad,pad);assert.equal(grip.draggable,true);assert.equal(keys.textContent,'Ctrl+1 · Shift+F2');assert.equal(view.fill.style.width,'42%');
});
test('switching views moves existing controls into the compact menu and restores them with their handlers',()=>{
 const {h,pad,top,bottom,group,trim,effects}=harness(),trimHandler=trim.onclick;
 h.$('boardViewCompact').onclick();const content=pad.querySelector('.sound-pad-menu-content');assert.equal(trim.parent,content);assert.equal(effects.parent,content);assert.equal(group.parent,content);assert.equal(group.value,'group-1');assert.equal(h.saves,1);
 h.$('boardViewStandard').onclick();assert.equal(trim.parent,bottom);assert.equal(effects.parent,bottom);assert.equal(group.parent,top);assert.equal(trim.onclick,trimHandler);assert.equal(group.value,'group-1');assert.equal(h.$('board').classes.has('compact-soundboard'),false);assert.equal(h.saves,2);
});
test('the chosen view is part of the existing preferences payload and missing legacy settings use Standard',()=>{
 const {h}=harness();h.initSoundboardView(undefined);assert.equal(vm.runInContext('soundboardView',h),'standard');h.$('boardViewCompact').onclick();
 Object.assign(h,{vstFolders:()=>[],selections:()=>({}),options:()=>({}),filterGroups:[],allFilterHotkeys:[],allFilterMIDIs:[]});const source=fs.readFileSync('dist/studio.js','utf8');vm.runInContext(source.slice(source.indexOf('function snapshotPreferences'),source.indexOf('async function save')),h);assert.equal(h.snapshotPreferences().SoundboardView,'compact');
});
