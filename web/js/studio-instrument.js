'use strict';
const instrumentKeys=['KeyA','KeyW','KeyS','KeyE','KeyD','KeyF','KeyT','KeyG','KeyY','KeyH','KeyU','KeyJ','KeyK','KeyO','KeyL','KeyP','Semicolon'];
const instrumentNoteNames=['C','C♯','D','D♯','E','F','F♯','G','G♯','A','A♯','B'];
const instrumentHeld=new Set(),instrumentPianoHeld=new Map();
let instrumentOctave=4;
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
function instrumentPressed(index){
 return instrumentHeld.has(index)||[...instrumentPianoHeld.values()].includes(index);
}
function syncInstrumentKey(index,wasPressed){
 const pressed=instrumentPressed(index);
 if(pressed===wasPressed)return;
 const classes=$('instrumentPiano').children[index].classList;
 if(pressed)classes.add('played');else classes.remove('played');
 const pitch=12*(instrumentOctave+1)+index;
 queueInstrumentInput(()=>nativeInstrumentNote(pitch,pressed?100:0));
}
function holdPianoKey(index,source){
 if(instrumentPianoHeld.has(source))return;
 const wasPressed=instrumentPressed(index);
 instrumentPianoHeld.set(source,index);
 syncInstrumentKey(index,wasPressed);
}
function releasePianoKey(source){
 const index=instrumentPianoHeld.get(source);
 if(index===undefined)return;
 instrumentPianoHeld.delete(source);
 syncInstrumentKey(index,true);
}
function updateInstrumentInput(focused=document.hasFocus()){
 const midi=currentTab==='instruments',piano=midi&&focused;
 if(!piano){
  for(const key of $('instrumentPiano').children)key.classList.remove('played');
  instrumentHeld.clear();
  instrumentPianoHeld.clear();
 }else if(!$('instrumentKeyboard').checked){
  for(const index of instrumentHeld){
   instrumentHeld.delete(index);
   syncInstrumentKey(index,true);
  }
 }
 queueInstrumentInput(()=>nativeInstrumentInput(piano,midi));
}
function changeInstrumentOctave(delta){
 const next=Math.max(-1,Math.min(8,instrumentOctave+delta));
 if(next===instrumentOctave)return;
 const released=new Set([...instrumentHeld,...instrumentPianoHeld.values()]);
 instrumentHeld.clear();instrumentPianoHeld.clear();
 for(const index of released)syncInstrumentKey(index,true);
 instrumentOctave=next;
 $('instrumentOctave').textContent='Octave '+next;
 [...$('instrumentPiano').children].forEach((key,index)=>{
  const pitch=instrumentNoteNames[index%12]+(next+Math.floor(index/12));
  key.firstElementChild.textContent=pitch;
  key.setAttribute('aria-label','Play '+pitch);
 });
}
function instrumentKey(event){
 const index=instrumentKeys.indexOf(event.code);
 if(event.type==='keyup'&&instrumentHeld.delete(index))syncInstrumentKey(index,true);
 if(event.type!=='keydown'||currentTab!=='instruments'||document.querySelector('dialog[open]'))return;
 if(!event.isComposing&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&(event.code==='KeyZ'||event.code==='KeyX')){
  const target=event.target;
  if(!target?.isContentEditable&&!target?.closest?.('input,textarea,select,[contenteditable="true"]')){
   event.preventDefault();
   if(!event.repeat)changeInstrumentOctave(event.code==='KeyZ'?-1:1);
  }
  return;
 }
 if(!$('instrumentKeyboard').checked)return;
 if(event.key==='Escape'){
  $('instrumentKeyboard').checked=false;updateInstrumentInput();return;
 }
 if(index<0||event.repeat||instrumentHeld.has(index)||event.ctrlKey||event.metaKey||event.altKey)return;
 const target=event.target;
 if(target?.isContentEditable||target?.closest?.('input:not([type="checkbox"]),textarea,select,[contenteditable="true"]'))return;
 const wasPressed=instrumentPressed(index);
 instrumentHeld.add(index);
 syncInstrumentKey(index,wasPressed);
}
let instrumentWhiteKeys=0;
instrumentKeys.forEach((code,index)=>{
 const note=instrumentNoteNames[index%12],key=document.createElement('button'),black=note.endsWith('♯');
 const pitch=note+(4+Math.floor(index/12)),focusKey='key:'+index;
 key.type='button';
 key.className='instrument-key'+(black?' black':'');
 if(black)key.style.left=instrumentWhiteKeys*10+'%';else instrumentWhiteKeys++;
 const noteLabel=document.createElement('span');noteLabel.textContent=pitch;
 key.setAttribute('aria-label','Play '+pitch);key.append(noteLabel);
 const label=document.createElement('kbd');label.textContent=code==='Semicolon'?';':code.slice(-1);label.setAttribute('aria-hidden','true');key.append(label);
 key.onpointerdown=e=>{
  if(e.button!==0)return;
  key.setPointerCapture(e.pointerId);
  holdPianoKey(index,e.pointerId);
 };
 key.onpointermove=e=>{
  if(!key.hasPointerCapture(e.pointerId))return;
  const hovered=document.elementFromPoint(e.clientX,e.clientY)?.closest('.instrument-key');
  const keys=$('instrumentPiano').children;
  const next=Array.prototype.indexOf.call(keys,hovered);
  if(next===instrumentPianoHeld.get(e.pointerId))return;
  releasePianoKey(e.pointerId);
  if(next>=0)holdPianoKey(next,e.pointerId);
 };
 key.onpointerup=key.onpointercancel=key.onlostpointercapture=e=>releasePianoKey(e.pointerId);
 key.onkeydown=e=>{
  if(e.key!==' '&&e.key!=='Enter')return;
  e.preventDefault();
  holdPianoKey(index,focusKey);
 };
 key.onkeyup=e=>{
  if(e.key!==' '&&e.key!=='Enter')return;
  e.preventDefault();
  releasePianoKey(focusKey);
 };
 key.onblur=()=>releasePianoKey(focusKey);
 key.onclick=e=>{
  if(e.detail!==0)return;
  const source=Symbol();
  holdPianoKey(index,source);
  setTimeout(()=>releasePianoKey(source),150);
 };
 $('instrumentPiano').append(key);
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
