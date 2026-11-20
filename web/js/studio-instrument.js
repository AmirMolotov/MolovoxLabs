'use strict';
const instrumentKeys=['KeyA','KeyW','KeyS','KeyE','KeyD','KeyF','KeyT','KeyG','KeyY','KeyH','KeyU','KeyJ','KeyK','KeyO','KeyL','KeyP','Semicolon'];
const instrumentNoteNames=['C','C♯','D','D♯','E','F','F♯','G','G♯','A','A♯','B'];
const instrumentHeld=new Set();
let instrumentCatalog=[],activeInstruments=[],instrumentBusy=false,instrumentBridge=Promise.resolve();
function instrumentStatus(text){$('instrumentStatus').textContent=text;}
function instrumentID(plugin){return plugin.Path+'|'+plugin.ClassID;}
function showInstrumentCatalog(result){
 instrumentCatalog=(result.Plugins||[]).filter(isInstrumentPlugin);
 renderInstrumentRack();
 if(!activeInstruments.length)instrumentStatus(instrumentCatalog.length?'Choose an instrument to play.':'No VST3 instruments found. Install an instrument and scan again.');
}
function renderInstrumentRack(){
 const select=$('instrumentPlugin'),list=$('instrumentPlugins'),available=instrumentCatalog.filter(plugin=>!activeInstruments.some(item=>instrumentID(item)===instrumentID(plugin)));
 select.replaceChildren(new Option('Add a VST3 instrument…',''));
 for(const plugin of available)select.add(new Option(plugin.Name+(plugin.Vendor?' · '+plugin.Vendor:''),instrumentID(plugin)));
 select.disabled=instrumentBusy||!available.length||activeInstruments.length>=4;
 list.hidden=!activeInstruments.length;list.replaceChildren();
 for(const plugin of activeInstruments){
  const row=document.createElement('li'),name=document.createElement('strong'),editor=document.createElement('button'),remove=document.createElement('button');
  name.textContent=plugin.Name;editor.className=remove.className='secondary';
  editor.textContent='Open editor';remove.textContent='Remove';editor.disabled=remove.disabled=instrumentBusy;
  editor.setAttribute('aria-label','Open '+plugin.Name+' editor');remove.setAttribute('aria-label','Remove '+plugin.Name);
  editor.onclick=()=>nativeInstrumentEditor(instrumentID(plugin)).catch(error=>instrumentStatus(errorText(error)));
  remove.onclick=()=>setInstrumentRack(activeInstruments.filter(item=>item!==plugin));
  row.append(name,editor,remove);list.append(row);
 }
}
async function setInstrumentRack(plugins){
 if(instrumentBusy)return;
 instrumentBusy=true;renderInstrumentRack();
 try{
  await withLoading('Updating instruments…',()=>nativeSetInstruments(plugins.map(instrumentID)));
  activeInstruments=plugins;
  if(plugins.length&&!active)await start(false);
  instrumentStatus(!plugins.length?'Choose an instrument to play.':!active?'Resume listening after choosing an output in Settings.':patchGraph?.Enabled?'Connect the Instruments source to an output in Patch bay.':'Ready to play.');
 }catch(error){instrumentStatus(errorText(error));}
 finally{instrumentBusy=false;renderInstrumentRack();}
}
async function refreshInstrumentMIDI(){
 try{
  const select=$('instrumentMIDI'),selected=$('midiInput').value,devices=await nativeMIDIDevices();
  select.replaceChildren(new Option('Off',''));
  for(const device of devices)select.add(new Option(device.Name,device.Name));
  if(selected&&!devices.some(device=>device.Name===selected))select.add(new Option(selected+' (disconnected)',selected));
  select.value=selected;
 }catch(error){instrumentStatus(errorText(error));}
}
function openInstruments(){if(!vstCatalogLoading)vstCatalogLoading=initVST();refreshInstrumentMIDI();}
function queueInstrumentInput(action){instrumentBridge=instrumentBridge.then(action).catch(error=>instrumentStatus(errorText(error)));return instrumentBridge;}
function releaseInstrumentKeys(){
 for(const index of instrumentHeld)$('instrumentPiano').children[index].classList.remove('played');
 instrumentHeld.clear();
}
function updateInstrumentInput(focused=document.hasFocus()){
 const midi=currentTab==='instruments',keyboard=midi&&$('instrumentKeyboard').checked&&focused;
 if(!keyboard)releaseInstrumentKeys();
 queueInstrumentInput(()=>nativeInstrumentInput(keyboard,midi));
}
function instrumentKey(event){
 const index=instrumentKeys.indexOf(event.code);
 if(event.type==='keyup'&&instrumentHeld.delete(index)){
  $('instrumentPiano').children[index].classList.remove('played');
  queueInstrumentInput(()=>nativeInstrumentNote(60+index,0));
 }
 if(currentTab!=='instruments'||!$('instrumentKeyboard').checked||event.key==='Alt'||document.querySelector('dialog[open]'))return;
 const target=event.target,editing=target?.isContentEditable||target?.closest?.('input:not([type="checkbox"]),textarea,select,[contenteditable="true"]');
 if(event.type==='keydown'&&event.key==='Escape'){
  event.preventDefault();event.stopImmediatePropagation();$('instrumentKeyboard').checked=false;updateInstrumentInput();return;
 }
 if(event.key==='Tab'||(target?.closest?.('[role="tab"]')&&event.key.startsWith('Arrow')))return;
 if(event.type==='keydown'&&index>=0&&!editing&&!event.ctrlKey&&!event.metaKey&&!event.altKey){
  if(!event.repeat&&!instrumentHeld.has(index)){
   instrumentHeld.add(index);$('instrumentPiano').children[index].classList.add('played');
   queueInstrumentInput(()=>nativeInstrumentNote(60+index,100));
  }
 }
 event.stopImmediatePropagation();
 if(!editing&&(index>=0||!target?.closest?.('button,input,label,summary')))event.preventDefault();
}
let instrumentWhiteKeys=0;
instrumentKeys.forEach((code,index)=>{
 const note=instrumentNoteNames[index%12],key=document.createElement('span'),black=note.endsWith('♯');
 key.className='instrument-key'+(black?' black':'');
 if(black)key.style.left=instrumentWhiteKeys*10+'%';else instrumentWhiteKeys++;
 key.setAttribute('aria-hidden','true');key.append(note+(4+Math.floor(index/12)));
 const label=document.createElement('kbd');label.textContent=code==='Semicolon'?';':code.slice(-1);key.append(label);$('instrumentPiano').append(key);
});
$('instrumentKeyboard').onchange=()=>updateInstrumentInput();
$('instrumentPlugin').onchange=()=>{const select=$('instrumentPlugin'),plugin=instrumentCatalog.find(item=>instrumentID(item)===select.value);if(plugin)setInstrumentRack([...activeInstruments,plugin]);};
$('instrumentScan').onclick=()=>{instrumentStatus('Scanning for VST3 instruments…');return $('scanVST').onclick();};
$('instrumentMIDI').onchange=async()=>{
 const select=$('instrumentMIDI'),name=select.value,previous=$('midiInput').value;
 select.disabled=true;
 try{await nativeSelectMIDIInput(name);if(name&&![...$('midiInput').options].some(option=>option.value===name))$('midiInput').add(new Option(name,name));$('midiInput').value=name;}
 catch(error){select.value=previous;instrumentStatus(errorText(error));}
 finally{select.disabled=false;}
};
$('instrumentPatch').onclick=()=>{
 openTab('patch');
 if(!patchGraph.Nodes.some(node=>node.Kind==='instrument'))addPatchNode(patchNode('instrument',{},40,80+patchGraph.Nodes.length*30));
 patchStatus('Connect Instruments to an effect or output, then enable Patch bay.');
};
window.addEventListener('keydown',instrumentKey,true);
window.addEventListener('keyup',instrumentKey,true);
window.addEventListener('focus',()=>updateInstrumentInput(true));
window.addEventListener('blur',()=>updateInstrumentInput(false));
