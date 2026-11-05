'use strict';
const $=id=>document.getElementById(id);
const errorText=e=>e?.message||String(e);
const message=text=>$('status').textContent=text;
let active=false,busy=false,ready=false,polling=false,pollTimer=null,saveTimer=null,preferences=null,devices=null,library={Boards:[]},currentTab='board';
let micFilters=[],filterGroups=[],allFilterHotkeys=[],allFilterMIDIs=[],bypassFilters=false;
let wantListening=false,routeApplying=false,routeDirty=false,routeTimer=null;
let padViews=new Map(),playingSounds=new Map();
function openTab(name){
 currentTab=name;
 for(const tab of document.querySelectorAll('[data-tab]')){const on=tab.dataset.tab===name;tab.setAttribute('aria-selected',String(on));tab.tabIndex=on?0:-1;$(tab.getAttribute('aria-controls')).hidden=!on;}
 if(active)poll();else idle();if(name==='patch')renderPatch();
}
document.querySelectorAll('[data-tab]').forEach(tab=>{
 tab.onclick=()=>openTab(tab.dataset.tab);
 tab.onkeydown=e=>{if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;e.preventDefault();const tabs=[...document.querySelectorAll('[data-tab]')],delta=['ArrowLeft','ArrowUp'].includes(e.key)?-1:1,next=tabs[(tabs.indexOf(tab)+delta+tabs.length)%tabs.length];openTab(next.dataset.tab);next.focus();};
});
$('configureTriggers').onclick=()=>openTab('settings');
function selections(){return {
 Inputs:[...document.querySelectorAll('#inputs .device')].filter(row=>row.querySelector('[type=checkbox]').checked).map(row=>({ID:row.dataset.id,Gain:Number(row.querySelector('[type=range]').value)/100})),
 Outputs:[...document.querySelectorAll('#outputs input:checked')].map(el=>el.value)
};}
function options(){return {Patch:patchOptions(),Volume:Number($('master').value)/100,BoardVolume:Number($('boardVolume').value)/100,Enabled:false,MonitorMic:$('monitorMic').checked,Inverted:false,DelayMS:0,Filter:false,Filters:micFilters,BypassFilters:bypassFilters,InputGains:Object.fromEntries(selections().Inputs.map(x=>[x.ID,x.Gain]))};}
function snapshotPreferences(){return {VSTFolders:vstFolders(),Saved:true,AutoListen:$('autoListen').checked,BoardID:$('boardSelect').value,GlobalKeys:$('globalKeys').checked,MIDIInput:$('midiInput').value,...selections(),Options:options(),FilterGroups:filterGroups,AllFilterHotkeys:allFilterHotkeys,AllFilterMIDIs:allFilterMIDIs};}
async function save(){
 if(!ready)return false;
 clearTimeout(saveTimer);
 try{return await queueSettings(()=>withLoading('Saving changes…',async()=>{const value=snapshotPreferences();await nativeSavePreferences(value);preferences=value;$('saveStatus').textContent='Saved on this computer.';return true;}));}
 catch(e){$('saveStatus').textContent='Could not save: '+errorText(e);message(errorText(e));return false;}
}
function scheduleSave(){if(!ready)return;clearTimeout(saveTimer);saveTimer=setTimeout(save,180);}
function updateLabels(){$('masterValue').textContent=$('master').value+'%';$('boardVolumeValue').textContent=$('boardVolume').value+'%';}
let configurePending=false,configurePromise=null;
function configure(){
 updateLabels();configurePending=true;if(configurePromise)return configurePromise;
 configurePromise=queueSettings(()=>withLoading('Applying effects and routing…',async()=>{
  try{do{configurePending=false;await nativeConfigure(options());}while(configurePending);scheduleSave();refreshVSTCards();if(patchGraph)renderPatch();return true;}
  catch(e){message(errorText(e));return false;}
 })).finally(()=>{configurePromise=null;});return configurePromise;
}
['master','boardVolume'].forEach(id=>$(id).oninput=configure);
$('monitorMic').onchange=configure;
$('autoListen').onchange=save;
function wave(id,samples,color){
 const canvas=$(id);if(!canvas.clientWidth)return;
 const pen=canvas.getContext('2d'),ratio=window.devicePixelRatio||1,w=canvas.width=Math.max(1,Math.round(canvas.clientWidth*ratio)),h=canvas.height=Math.max(1,Math.round(canvas.clientHeight*ratio));
 pen.clearRect(0,0,w,h);pen.strokeStyle='#ffffff09';pen.lineWidth=1;
 for(let x=0;x<w;x+=w/12){pen.beginPath();pen.moveTo(x,0);pen.lineTo(x,h);pen.stroke();}
 for(let y=0;y<=h;y+=h/4){pen.beginPath();pen.moveTo(0,y);pen.lineTo(w,y);pen.stroke();}
 pen.strokeStyle=color;pen.lineWidth=1.5*ratio;pen.beginPath();
 if(samples?.length)samples.forEach((v,i)=>{const x=i/(samples.length-1)*w,y=h/2+v*h*.45;i?pen.lineTo(x,y):pen.moveTo(x,y);});else{pen.moveTo(0,h/2);pen.lineTo(w,h/2);}
 pen.stroke();
}
function idle(){wave('micWave',null,'#b9e78d55');wave('outWave',null,'#cf968155');}
function setLive(running){active=running;$('pause').disabled=busy;$('pause').textContent=running?'Pause listening':'Resume listening';$('liveLabel').textContent=running?'Listening · microphone active':'Listening paused';$('liveDot').style.background=running?'#b9e78d':'#718275';}
function summary(){
 const s=selections(),ins=s.Inputs.map(x=>devices?.Inputs.find(d=>d.ID===x.ID)?.Name||'Unavailable input'),outs=s.Outputs.map(x=>devices?.Outputs.find(d=>d.ID===x)?.Name||'Unavailable output');
 $('routingSummary').textContent=(ins.join(' + ')||'No microphones selected')+' → '+(outs.join(' + ')||'No outputs selected');
}
const isPhysical=name=>!/cable|voicemeeter|virtual/i.test(name);
function fillDevices(list,prefs){
 devices=list;
 const savedInputs=new Map((prefs.Inputs||[]).map(x=>[x.ID,x.Gain])),savedOutputs=new Set(prefs.Outputs||[]);
 const firstInput=list.Inputs.find(d=>d.Default&&isPhysical(d.Name))||list.Inputs.find(d=>isPhysical(d.Name))||list.Inputs[0],firstOutput=list.Outputs.find(d=>d.Default&&isPhysical(d.Name))||list.Outputs.find(d=>isPhysical(d.Name))||list.Outputs[0];
 $('inputs').replaceChildren();$('outputs').replaceChildren();
 for(const d of list.Inputs){
  const row=document.createElement('div');row.className='device';row.dataset.id=d.ID;
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.checked=prefs.Saved?savedInputs.has(d.ID):d===firstInput;label.append(box,document.createTextNode(d.Name));
  box.onchange=()=>{summary();deviceChoiceChanged();};
  const gain=document.createElement('input');gain.type='range';gain.min=0;gain.max=100;gain.value=Math.round((savedInputs.get(d.ID)??1)*100);gain.setAttribute('aria-label',d.Name+' level');
  const value=document.createElement('output');value.textContent=gain.value+'%';gain.oninput=()=>{value.textContent=gain.value+'%';configure();};
  row.append(label,gain,value);$('inputs').append(row);
 }
 for(const d of list.Outputs){
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.value=d.ID;box.checked=prefs.Saved?savedOutputs.has(d.ID):d===firstOutput;
  label.append(box,document.createTextNode(d.Name));box.onchange=()=>{summary();deviceChoiceChanged();};$('outputs').append(label);
 }
 if(!list.Inputs.length)$('inputs').textContent='No microphones available.';
 if(!list.Outputs.length)$('outputs').textContent='No outputs available.';
 summary();
 return prefs.Saved&&((prefs.Inputs||[]).some(x=>!list.Inputs.some(d=>d.ID===x.ID))||(prefs.Outputs||[]).some(id=>!list.Outputs.some(d=>d.ID===id)));
}
async function stop(reason='Listening paused. Devices released.'){
 if(busy)return;busy=true;$('pause').disabled=true;
 try{await queueSettings(()=>withLoading('Pausing audio…',()=>nativeStop()));clearInterval(pollTimer);pollTimer=null;setLive(false);idle();updatePlayback([]);$('inputDB').textContent=$('outputDB').textContent='— dBFS';message(reason);}
 catch(e){message(errorText(e));}finally{busy=false;$('pause').disabled=false;}
}
async function start(){
 if(active||busy)return false;
 wantListening=true;
 const s=selections();if(!s.Inputs.length||!s.Outputs.length){setLive(false);message('Select a microphone and an output in Settings.');return false;}
 busy=true;$('pause').disabled=true;
 try{await queueSettings(()=>withLoading('Opening audio devices…',()=>nativeStart({...selections(),Options:options()}),true));busy=false;setLive(true);clearInterval(pollTimer);pollTimer=setInterval(poll,66);await poll();message('Listening. Device changes apply automatically.');return true;}
 catch(e){busy=false;setLive(false);message(errorText(e));return false;}
 finally{busy=false;$('pause').disabled=false;}
}
async function poll(){
 if(!active||polling)return;polling=true;
 try{
  const s=await nativeSnapshot();if(!active)return;
  if(s.Fault){await stop('An audio device disconnected. Select available devices in Settings.');return;}
  wave('micWave',s.Mic,'#b9e78d');wave('outWave',s.Output,'#cf9681');updatePlayback(s.Playback||[]);
  $('inputDB').textContent=s.InputDB.toFixed(1)+' dBFS';$('outputDB').textContent=s.OutputDB.toFixed(1)+' dBFS';
  $('engineDetails').textContent=s.Backend+' · '+s.Inputs+' inputs · '+s.Outputs+' outputs · underruns '+s.Underruns+' · clipped samples '+s.Clipped;
 }catch(e){await stop(errorText(e));}finally{polling=false;}
}
$('pause').onclick=async()=>{wantListening=!active;if(active)await stop();else await start();};
function deviceChoiceChanged(){routeDirty=true;clearTimeout(routeTimer);routeTimer=setTimeout(applyRouting,90);}
async function applyRouting(){
 if(!ready||routeApplying)return;
 if(busy){routeTimer=setTimeout(applyRouting,90);return;}
 routeApplying=true;
 try{do{routeDirty=false;await save();if(wantListening){if(active)await stop('Switching devices…');const s=selections();if(s.Inputs.length&&s.Outputs.length)await start();else message('Select a microphone and output to resume listening.');}}while(routeDirty);}
 finally{routeApplying=false;}
}
$('refresh').onclick=async()=>{try{fillDevices(await withLoading('Finding audio devices…',()=>nativeDevices()),snapshotPreferences());deviceChoiceChanged();}catch(e){message(errorText(e));}};
$('testTone').onclick=async()=>{if(!active&&!await start())return;try{await nativeTone();message('Playing a quiet test tone.');}catch(e){message(errorText(e));}};
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!busy&&!document.querySelector('dialog[open]')){wantListening=false;nativeStopSounds().catch(()=>{});stop();}});
window.addEventListener('resize',()=>{if(!active)idle();});
window.addEventListener('pagehide',()=>{nativeStop().catch(()=>{});});
function renderLibrary(preferred){
 const previous=preferred||$('boardSelect').value;$('boardSelect').replaceChildren();
 for(const b of library.Boards)$('boardSelect').add(new Option(b.Name,b.ID));
 if(library.Boards.some(b=>b.ID===previous))$('boardSelect').value=previous;renderPads();if(patchGraph)renderPatch();
}
function clipKeys(sound){return sound.Hotkeys??(sound.Hotkey?[sound.Hotkey]:[]);}
function clipMIDIs(sound){return sound.MIDIs??(sound.MIDI?[sound.MIDI]:[]);}
function renderPads(){
 $('soundPads').replaceChildren();padViews=new Map();
 const board=library.Boards.find(b=>b.ID===$('boardSelect').value);
 $('boardMeta').textContent=(board?.Sounds.length||0)+' sounds · click to play or stop';
 if(!board?.Sounds.length){const empty=document.createElement('div');empty.className='empty sound-empty';empty.textContent='Make some noise. Add your first sound to start building this board.';$('soundPads').append(empty);return;}
 for(const sound of board.Sounds){
  const pad=document.createElement('article');pad.className='sound-pad';
  const fill=document.createElement('div');fill.className='playback-fill';pad.append(fill);
  const top=document.createElement('div');top.className='sound-pad-top';const kind=document.createElement('span');kind.className='clip-kind';kind.textContent=(sound.File?.split('.').pop()||'audio').toUpperCase();
  const remove=document.createElement('button');remove.className='pad-remove';remove.textContent='×';remove.setAttribute('aria-label','Remove '+sound.Name);
  remove.onclick=async()=>{try{await nativeStopSound(sound.ID);library=await nativeRemoveSound(sound.ID);renderPads();}catch(e){message(errorText(e));}};top.append(kind,remove);pad.append(top);
  const play=document.createElement('button');play.className='sound-pad-play';play.setAttribute('aria-label','Play or stop '+sound.Name);
  const playIcon=document.createElement('span');playIcon.className='play-icon';playIcon.textContent='▶';const title=document.createElement('strong');title.textContent=sound.Name;
  const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 240 36');svg.classList.add('pad-wave');svg.setAttribute('aria-hidden','true');
  const defs=document.createElementNS(svg.namespaceURI,'defs'),gradient=document.createElementNS(svg.namespaceURI,'linearGradient'),gradientID='clip-gradient-'+sound.ID;gradient.id=gradientID;
  const stop1=document.createElementNS(svg.namespaceURI,'stop'),stop2=document.createElementNS(svg.namespaceURI,'stop');stop1.setAttribute('stop-color','#c4ed9e');stop2.setAttribute('stop-color','#49624f');stop1.setAttribute('offset','0%');stop2.setAttribute('offset','0%');gradient.append(stop1,stop2);defs.append(gradient);svg.append(defs);
  const peaks=sound.Waveform||[];
  for(let i=0;i<40;i++){const bar=document.createElementNS(svg.namespaceURI,'rect'),height=Math.max(2,(peaks[i]||0)*30);bar.setAttribute('x',String(i*6));bar.setAttribute('y',String(18-height/2));bar.setAttribute('width','3');bar.setAttribute('height',String(height));bar.setAttribute('rx','1.5');bar.setAttribute('fill','url(#'+gradientID+')');svg.append(bar);}
  play.append(playIcon,title,svg);pad.append(play);
  const bottom=document.createElement('div');bottom.className='sound-pad-bottom';const edit=document.createElement('button');edit.className='pad-edit';const keys=clipKeys(sound),midis=clipMIDIs(sound);
  edit.textContent=keys.length||midis.length?'⌨ '+keys.length+' · MIDI '+midis.length:'⌨ Add triggers';edit.setAttribute('aria-label','Edit triggers for '+sound.Name);edit.onclick=()=>openTriggerEditor(sound.ID);
  const time=document.createElement('span');time.className='clip-time';time.textContent=sound.Seconds.toFixed(1)+'s';bottom.append(edit,time);pad.append(bottom);
  play.onclick=async()=>{if(playingSounds.has(sound.ID)){await nativeStopSound(sound.ID);return;}if(!active&&!await start())return;play.disabled=true;try{await withLoading('Loading sound…',()=>nativePlaySound(sound.ID));$('boardStatus').textContent='Playing '+sound.Name+'.';}catch(e){message(errorText(e));}finally{play.disabled=false;}};
  padViews.set(sound.ID,{pad,fill,playIcon,time,stops:[stop1,stop2],sound});$('soundPads').append(pad);
 }
 updatePlayback([...playingSounds.values()]);
}
function updatePlayback(items){
 playingSounds=new Map();
 for(const item of items){const previous=playingSounds.get(item.ID);if(!previous||item.Sequence>previous.Sequence)playingSounds.set(item.ID,item);}
 for(const [id,v] of padViews){const item=playingSounds.get(id),on=!!item,p=on?Math.max(0,Math.min(1,item.Progress)):0;v.pad.classList.toggle('is-playing',on);v.fill.style.width=(p*100)+'%';v.playIcon.textContent=on?'■':'▶';v.time.textContent=on?item.Remaining.toFixed(1)+'s left':v.sound.Seconds.toFixed(1)+'s';for(const stop of v.stops)stop.setAttribute('offset',(p*100)+'%');}
}
$('boardSelect').onchange=async()=>{renderPads();await save();};
let deletingBoardID=null;
$('deleteBoard').onclick=()=>{
 const board=library.Boards.find(b=>b.ID===$('boardSelect').value);if(!board)return;
 deletingBoardID=board.ID;$('deleteBoardDescription').textContent='Delete “'+board.Name+'” and its '+board.Sounds.length+' sound'+(board.Sounds.length===1?'':'s')+'?';
 $('deleteBoardStatus').textContent='';$('confirmDeleteBoard').disabled=false;document.querySelector('.board-menu').open=false;$('deleteBoardDialog').showModal();$('cancelDeleteBoard').focus();
};
function cancelBoardDeletion(){$('deleteBoardDialog').close();deletingBoardID=null;}
$('closeDeleteBoard').onclick=cancelBoardDeletion;$('cancelDeleteBoard').onclick=cancelBoardDeletion;
$('confirmDeleteBoard').onclick=async()=>{
 if(!deletingBoardID)return;const id=deletingBoardID,index=library.Boards.findIndex(b=>b.ID===id);$('confirmDeleteBoard').disabled=true;
 try{clearTimeout(saveTimer);const result=await withLoading('Deleting soundboard…',()=>nativeRemoveBoard(id),true);library=result.Library;renderLibrary(library.Boards[Math.min(Math.max(index,0),library.Boards.length-1)]?.ID);await save();cancelBoardDeletion();$('boardStatus').textContent='Soundboard deleted.'+((result.Warnings||[]).length?' '+result.Warnings.join(' '):'');}
 catch(e){$('deleteBoardStatus').textContent=errorText(e);}
 finally{$('confirmDeleteBoard').disabled=false;}
};

$('createBoard').onclick=async()=>{try{library=await nativeCreateBoard($('boardName').value);$('boardName').value='';renderLibrary(library.Boards.at(-1).ID);await save();}catch(e){message(errorText(e));}};
$('importSound').onclick=async()=>{$('importSound').disabled=true;try{library=await withLoading('Adding audio to your soundboard…',()=>nativeImportSound($('boardSelect').value),true);renderPads();$('boardStatus').textContent='Audio copied into your library.';}catch(e){message(errorText(e));}finally{$('importSound').disabled=false;}};
$('stopSounds').onclick=async()=>{await nativeStopSounds();updatePlayback([]);$('boardStatus').textContent='All sounds stopped.';};
$('exportBoard').onclick=async()=>{try{const path=await withLoading('Exporting soundboard and audio…',()=>nativeExportBoard($('boardSelect').value),true);if(path)$('boardStatus').textContent='Exported board, original audio files and all trigger assignments in one .qlboard package.';}catch(e){message(errorText(e));}};
$('importBoard').onclick=async()=>{
 $('importBoard').disabled=true;
 try{const count=library.Boards.length,id=$('boardSelect').value;library=await withLoading('Importing soundboard and audio…',()=>nativeImportBoard(),true);renderLibrary(library.Boards.length>count?library.Boards.at(-1)?.ID:id);await save();$('boardStatus').textContent='Board and embedded audio imported. Conflicting triggers remain unassigned.';}
 catch(e){message(errorText(e));}finally{$('importBoard').disabled=false;}
};
function loadFilterModels(p){
 micFilters=p.Options?.Filters||[];filterGroups=p.FilterGroups||[];allFilterHotkeys=p.AllFilterHotkeys||[];allFilterMIDIs=p.AllFilterMIDIs||[];bypassFilters=!!p.Options?.BypassFilters;renderFilters();renderGroups();
}
async function init(){
 const finishLoading=beginLoading('Opening your audio workspace…',true);
 try{
  await paintLoading();
  preferences=await nativeLoadPreferences();await initVST(preferences);const o=preferences.Options||{};
  $('master').value=Math.round((o.Volume??1)*100);$('boardVolume').value=Math.round((o.BoardVolume??1)*100);$('monitorMic').checked=!!(o.MonitorMic||o.Enabled);$('autoListen').checked=preferences.AutoListen??true;$('globalKeys').checked=preferences.GlobalKeys??true;
  loadFilterModels(preferences);await refreshMIDI(preferences.MIDIInput||'');updateLabels();
  const missing=fillDevices(await nativeDevices(),preferences);
  library=await nativeLibrary();renderLibrary(preferences.BoardID);initPatch(preferences);await nativeConfigure(options());ready=true;wantListening=$('autoListen').checked;$('pause').disabled=false;
  if(missing){setLive(false);openTab('settings');message('A saved device is missing. Select a replacement to switch automatically.');}
  else if(wantListening){await start();if(!preferences.Saved)await save();}
  else{setLive(false);message('Automatic listening is off. Resume when ready.');}
 }catch(e){setLive(false);message('Could not open Studio: '+errorText(e));}
 finally{finishLoading();}
 idle();
}
