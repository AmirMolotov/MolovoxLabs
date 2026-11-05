'use strict';
const $=id=>document.getElementById(id);
let active=false,busy=false,ready=false,polling=false,pollTimer=null,saveTimer=null,preferences=null,devices=null,library={Boards:[]},currentTab='board';
const message=text=>$('status').textContent=text;
const errorText=e=>e?.message||String(e);
function openTab(name){
 currentTab=name;
 for(const tab of document.querySelectorAll('[data-tab]')){const on=tab.dataset.tab===name;tab.setAttribute('aria-selected',String(on));tab.tabIndex=on?0:-1;$(tab.getAttribute('aria-controls')).hidden=!on;}
 if(active)poll();else idle();
}
document.querySelectorAll('[data-tab]').forEach(tab=>{
 tab.onclick=()=>openTab(tab.dataset.tab);
 tab.onkeydown=e=>{if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;e.preventDefault();const tabs=[...document.querySelectorAll('[data-tab]')];const delta=['ArrowLeft','ArrowUp'].includes(e.key)?-1:1;const next=tabs[(tabs.indexOf(tab)+delta+tabs.length)%tabs.length];openTab(next.dataset.tab);next.focus();};
});
$('configureTriggers').onclick=()=>openTab('settings');
let micFilters=[];
function options(){return {Volume:Number($('master').value)/100,BoardVolume:Number($('boardVolume').value)/100,Enabled:false,MonitorMic:$('monitorMic').checked,Inverted:false,DelayMS:0,Filter:false,Filters:micFilters};}
function selections(){
 return {
 Inputs:[...document.querySelectorAll('#inputs .device')].filter(row=>row.querySelector('[type=checkbox]').checked).map(row=>({ID:row.dataset.id,Gain:Number(row.querySelector('[type=range]').value)/100})),
 Outputs:[...document.querySelectorAll('#outputs input:checked')].map(el=>el.value)
 };
}
function snapshotPreferences(){return {Saved:true,AutoListen:$('autoListen').checked,BoardID:$('boardSelect').value,GlobalKeys:$('globalKeys').checked,MIDIInput:$('midiInput').value,...selections(),Options:options()};}
async function save(){
 if(!ready)return;
 try{const value=snapshotPreferences();await nativeSavePreferences(value);preferences=value;$('saveStatus').textContent='Saved on this computer.';}
 catch(e){$('saveStatus').textContent='Could not save: '+errorText(e);message('Could not save preferences: '+errorText(e));}
}
function scheduleSave(){if(!ready)return;clearTimeout(saveTimer);saveTimer=setTimeout(save,180);}
function updateLabels(){
 $('masterValue').textContent=$('master').value+'%';$('boardVolumeValue').textContent=$('boardVolume').value+'%';
}
async function configure(){
 updateLabels();
 try{await nativeConfigure(options());scheduleSave();}
 catch(e){message('Could not apply audio controls: '+errorText(e));}
}
['master','boardVolume'].forEach(id=>$(id).oninput=configure);
$('monitorMic').onchange=configure;
$('autoListen').onchange=scheduleSave;
function wave(id,samples,color){
 const canvas=$(id);if(!canvas.clientWidth)return;
 const pen=canvas.getContext('2d'),ratio=window.devicePixelRatio||1;
 const w=canvas.width=Math.max(1,Math.round(canvas.clientWidth*ratio)),h=canvas.height=Math.max(1,Math.round(canvas.clientHeight*ratio));
 pen.clearRect(0,0,w,h);pen.strokeStyle='#ffffff09';pen.lineWidth=1;
 for(let x=0;x<w;x+=w/12){pen.beginPath();pen.moveTo(x,0);pen.lineTo(x,h);pen.stroke();}
 for(let y=0;y<=h;y+=h/4){pen.beginPath();pen.moveTo(0,y);pen.lineTo(w,y);pen.stroke();}
 pen.strokeStyle=color;pen.lineWidth=1.5*ratio;pen.beginPath();
 if(samples?.length)samples.forEach((v,i)=>{const x=i/(samples.length-1)*w,y=h/2+v*h*.45;i?pen.lineTo(x,y):pen.moveTo(x,y);});
 else{pen.moveTo(0,h/2);pen.lineTo(w,h/2);}
 pen.stroke();
}
function idle(){wave('micWave',null,'#b9e78d55');wave('outWave',null,'#cf968155');}
function setLive(running){
 active=running;$('pause').disabled=busy;$('pause').textContent=running?'Pause listening':'Resume listening';
 $('liveLabel').textContent=running?'Listening · microphone active':'Listening paused';
 $('liveDot').style.background=running?'#b9e78d':'#718275';
}
function summary(){
 const selection=selections();
 const ins=selection.Inputs.map(x=>devices?.Inputs.find(d=>d.ID===x.ID)?.Name||'Unavailable input');
 const outs=selection.Outputs.map(x=>devices?.Outputs.find(d=>d.ID===x)?.Name||'Unavailable output');
 $('routingSummary').textContent=(ins.join(' + ')||'No microphones selected')+' → '+(outs.join(' + ')||'No outputs selected');
}
const isPhysical=name=>!/cable|voicemeeter|virtual/i.test(name);
function fillDevices(list,prefs){
 devices=list;
 const savedInputs=new Map((prefs.Inputs||[]).map(x=>[x.ID,x.Gain])),savedOutputs=new Set(prefs.Outputs||[]);
 const defaultInput=list.Inputs.find(d=>d.Default&&isPhysical(d.Name))||list.Inputs.find(d=>isPhysical(d.Name))||list.Inputs[0];
 const defaultOutput=list.Outputs.find(d=>d.Default&&isPhysical(d.Name))||list.Outputs.find(d=>isPhysical(d.Name))||list.Outputs[0];
 $('inputs').replaceChildren();$('outputs').replaceChildren();
 for(const d of list.Inputs){
  const row=document.createElement('div');row.className='device';row.dataset.id=d.ID;
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.checked=prefs.Saved?savedInputs.has(d.ID):d===defaultInput;
  label.append(box,document.createTextNode(d.Name));box.onchange=()=>{summary();scheduleSave();};
  const gain=document.createElement('input');gain.type='range';gain.min=0;gain.max=100;gain.value=Math.round((savedInputs.get(d.ID)??1)*100);gain.setAttribute('aria-label',d.Name+' level');
  const value=document.createElement('output');value.textContent=gain.value+'%';
  gain.oninput=()=>{value.textContent=gain.value+'%';scheduleSave();};
  row.append(label,gain,value);$('inputs').append(row);
 }
 for(const d of list.Outputs){
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.value=d.ID;box.checked=prefs.Saved?savedOutputs.has(d.ID):d===defaultOutput;
  label.append(box,document.createTextNode(d.Name));box.onchange=()=>{summary();scheduleSave();};$('outputs').append(label);
 }
 if(!list.Inputs.length)$('inputs').textContent='No microphones available.';
 if(!list.Outputs.length)$('outputs').textContent='No speakers available.';
 summary();
 return prefs.Saved&&((prefs.Inputs||[]).some(x=>!list.Inputs.some(d=>d.ID===x.ID))||(prefs.Outputs||[]).some(id=>!list.Outputs.some(d=>d.ID===id)));
}
async function stop(reason='Listening paused. Devices released.'){
 if(busy)return;busy=true;$('pause').disabled=true;
 try{await nativeStop();clearInterval(pollTimer);pollTimer=null;setLive(false);idle();$('inputDB').textContent=$('outputDB').textContent='— dBFS';message(reason);}
 catch(e){message('Could not pause: '+errorText(e));}
 finally{busy=false;$('pause').disabled=false;}
}
async function start(){
 if(active||busy)return false;
 const selection=selections();
 if(!selection.Inputs.length||!selection.Outputs.length){message('Select a microphone and output in Settings.');setLive(false);return false;}
 busy=true;$('pause').disabled=true;
 try{
  await nativeStart({...selection,Options:options()});busy=false;setLive(true);
  clearInterval(pollTimer);pollTimer=setInterval(poll,66);await poll();message('Listening automatically. Your microphone is active.');return true;
 }catch(e){busy=false;setLive(false);message('Could not start listening: '+errorText(e));return false;}
 finally{busy=false;$('pause').disabled=false;}
}
async function poll(){
 if(!active||polling)return;polling=true;
 try{
  const s=await nativeSnapshot();if(!active)return;
  if(s.Fault){await stop('An audio device disconnected. Choose available devices in Settings.');return;}
  wave('micWave',s.Mic,'#b9e78d');wave('outWave',s.Output,'#cf9681');
  $('inputDB').textContent=s.InputDB.toFixed(1)+' dBFS';$('outputDB').textContent=s.OutputDB.toFixed(1)+' dBFS';
  $('engineDetails').textContent=s.Backend+' · '+s.Inputs+' inputs · '+s.Outputs+' outputs · underruns '+s.Underruns+' · clipped samples '+s.Clipped;
 }catch(e){await stop('Engine error: '+errorText(e));}
 finally{polling=false;}
}
$('pause').onclick=()=>active?stop():start();
$('apply').onclick=async()=>{await save();if(active)await stop('Switching audio devices…');await start();};
$('refresh').onclick=async()=>{
 try{const current=snapshotPreferences();fillDevices(await nativeDevices(),current);message('Devices refreshed. Apply your selections to change active routing.');}
 catch(e){message(errorText(e));}
};
$('testTone').onclick=async()=>{if(!active&&!await start())return;try{await nativeTone();message('Playing a quiet test tone on selected outputs.');}catch(e){message(errorText(e));}};
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!busy){nativeStopSounds().catch(()=>{});stop();}});
window.addEventListener('resize',()=>{if(!active)idle();});
window.addEventListener('pagehide',()=>{nativeStop().catch(()=>{});});
function renderLibrary(preferred){
 const previous=preferred||$('boardSelect').value;
 $('boardSelect').replaceChildren();
 for(const b of library.Boards)$('boardSelect').add(new Option(b.Name,b.ID));
 if(library.Boards.some(b=>b.ID===previous))$('boardSelect').value=previous;
 renderPads();
}
function renderPads(){
 $('soundPads').replaceChildren();
 const board=library.Boards.find(b=>b.ID===$('boardSelect').value);
 if(!board?.Sounds.length){const empty=document.createElement('div');empty.className='empty';empty.textContent='Your next sound starts here. Add a WAV or MP3 clip to build this board.';$('soundPads').append(empty);return;}
 for(const sound of board.Sounds){
  const pad=document.createElement('div');pad.className='pad';
  const play=document.createElement('button');play.className='play';play.setAttribute('aria-label','Play '+sound.Name);
  const icon=document.createElement('span');icon.textContent='▷';play.append(icon,document.createTextNode(sound.Name));
  play.onclick=async()=>{if(!active&&!await start())return;play.disabled=true;try{await nativePlaySound(sound.ID);$('boardStatus').textContent='Triggered '+sound.Name+'.';}catch(e){$('boardStatus').textContent=errorText(e);}finally{play.disabled=false;}};
  const row=document.createElement('div');row.className='row';
  const length=document.createElement('span');length.textContent=sound.Seconds.toFixed(1)+' sec';
  const remove=document.createElement('button');remove.className='remove';remove.textContent='×';remove.setAttribute('aria-label','Remove '+sound.Name);
  remove.onclick=async()=>{try{library=await nativeRemoveSound(sound.ID);renderPads();}catch(e){$('boardStatus').textContent=errorText(e);}};
  row.append(length,remove);
  const triggerRow=document.createElement('div');triggerRow.className='pad-triggers';
  const key=document.createElement('button');key.className='secondary';key.textContent=sound.Hotkey?.Label||'Shortcut';key.setAttribute('aria-label','Set shortcut for '+sound.Name);key.onclick=()=>learnKeyboard(sound);
  const midi=document.createElement('button');midi.className='secondary';midi.textContent=sound.MIDI?(sound.MIDI.Kind+' '+sound.MIDI.Number+' · ch '+sound.MIDI.Channel):'Learn MIDI';midi.setAttribute('aria-label','Learn MIDI for '+sound.Name);midi.onclick=()=>learnMIDI(sound);
  const clear=document.createElement('button');clear.className='text-button';clear.textContent='Clear triggers';clear.onclick=async()=>{try{library=await nativeSetBindings(sound.ID,null,null);renderPads();}catch(e){$('boardStatus').textContent=errorText(e);}};
  triggerRow.append(key,midi,clear);pad.append(play,triggerRow,row);$('soundPads').append(pad);
 }
}
$('boardSelect').onchange=()=>{renderPads();scheduleSave();};
$('createBoard').onclick=async()=>{try{library=await nativeCreateBoard($('boardName').value);$('boardName').value='';renderLibrary(library.Boards.at(-1).ID);scheduleSave();}catch(e){$('boardStatus').textContent=errorText(e);}};
$('importSound').onclick=async()=>{
 $('importSound').disabled=true;
 try{library=await nativeImportSound($('boardSelect').value);renderPads();$('boardStatus').textContent='Library saved on this computer.';}
 catch(e){$('boardStatus').textContent='Could not import: '+errorText(e);}
 finally{$('importSound').disabled=false;}
};
$('stopSounds').onclick=async()=>{await nativeStopSounds();$('boardStatus').textContent='All sounds stopped.';};
async function init(){
 try{
  preferences=await nativeLoadPreferences();const o=preferences.Options||{};
  $('master').value=Math.round((o.Volume??1)*100);$('boardVolume').value=Math.round((o.BoardVolume??1)*100);
  $('monitorMic').checked=!!(o.MonitorMic||o.Enabled);$('autoListen').checked=preferences.AutoListen??true;
  $('globalKeys').checked=preferences.GlobalKeys??true;
  micFilters=o.Filters||[];renderFilters();
  await refreshMIDI(preferences.MIDIInput||'');
  updateLabels();await nativeConfigure(options());
  const missing=fillDevices(await nativeDevices(),preferences);
  library=await nativeLibrary();renderLibrary(preferences.BoardID);
  ready=true;$('pause').disabled=false;
  if(missing){setLive(false);openTab('settings');message('A saved device is missing. Choose its replacement and Apply devices.');}
  else if($('autoListen').checked){await start();if(!preferences.Saved)await save();}
  else{setLive(false);message('Automatic listening is off. Resume when ready.');}
 }catch(e){setLive(false);message('Could not open workspace: '+errorText(e));}
 idle();
}

const filterNames={gain:'Gain',highpass:'High-pass',lowpass:'Low-pass',gate:'Noise gate',compressor:'Compressor'};
const filterDefaults={gain:{GainDB:0},highpass:{Frequency:80},lowpass:{Frequency:12000},gate:{Threshold:-45},compressor:{Threshold:-18,Ratio:4}};
function renderFilters(){
 $('filterCards').replaceChildren();
 if(!micFilters.length){const empty=document.createElement('div');empty.className='empty';empty.textContent='Your microphone is unfiltered. Add a filter above to shape its sound.';$('filterCards').append(empty);return;}
 micFilters.forEach((filter,index)=>{
  const card=document.createElement('div');card.className='card';
  const top=document.createElement('div');top.className='filter-top';
  const label=document.createElement('label');label.className='toggle';
  const enabled=document.createElement('input');enabled.type='checkbox';enabled.checked=filter.Enabled;
  enabled.onchange=()=>{filter.Enabled=enabled.checked;configure();};label.append(enabled,document.createTextNode((index+1)+'. '+filterNames[filter.Kind]));
  const actions=document.createElement('div');actions.className='filter-actions';
  for(const [text,delta] of [['↑',-1],['↓',1]]){
   const button=document.createElement('button');button.className='secondary';button.textContent=text;button.setAttribute('aria-label','Move '+filterNames[filter.Kind]+' '+(delta<0?'up':'down'));button.disabled=index+delta<0||index+delta>=micFilters.length;
   button.onclick=()=>{[micFilters[index],micFilters[index+delta]]=[micFilters[index+delta],micFilters[index]];renderFilters();configure();};actions.append(button);
  }
  const remove=document.createElement('button');remove.className='secondary';remove.textContent='Remove';remove.onclick=()=>{micFilters.splice(index,1);renderFilters();configure();};actions.append(remove);top.append(label,actions);card.append(top);
  const controls=document.createElement('div');controls.className='filter-controls';
  const range=(field,title,min,max,step,unit)=>{
   const wrap=document.createElement('div'),head=document.createElement('div');head.className='rangehead';
   const name=document.createElement('label'),readout=document.createElement('output');name.textContent=title;
   const slider=document.createElement('input');slider.type='range';slider.min=min;slider.max=max;slider.step=step;slider.value=filter[field];slider.setAttribute('aria-label',filterNames[filter.Kind]+' '+title);
   readout.textContent=filter[field]+unit;slider.oninput=()=>{filter[field]=Number(slider.value);readout.textContent=slider.value+unit;configure();};
   head.append(name,readout);wrap.append(head,slider);controls.append(wrap);
  };
  if(filter.Kind==='gain')range('GainDB','Gain',-24,24,.5,' dB');
  if(filter.Kind==='highpass')range('Frequency','Cutoff',20,1000,10,' Hz');
  if(filter.Kind==='lowpass')range('Frequency','Cutoff',1000,20000,100,' Hz');
  if(filter.Kind==='gate')range('Threshold','Threshold',-90,-5,1,' dB');
  if(filter.Kind==='compressor'){range('Threshold','Threshold',-60,0,1,' dB');range('Ratio','Ratio',1,20,.5,':1');}
  card.append(controls);$('filterCards').append(card);
 });
}
$('addFilter').onclick=()=>{
 if(micFilters.length>=8){message('Maximum eight microphone filters.');return;}
 const kind=$('filterKind').value;micFilters.push({Kind:kind,Enabled:true,...filterDefaults[kind]});renderFilters();configure();
};
let pendingKey=null,pendingMIDI=null,lastMIDISequence=0,lastTriggerMessage='',triggerPollBusy=false;
async function refreshMIDI(preferred){
 const selected=preferred??$('midiInput').value;
 const list=await nativeMIDIDevices();$('midiInput').replaceChildren(new Option('Off',''));
 for(const d of list)$('midiInput').add(new Option(d.Name,d.Name));
 if(selected&&!list.some(d=>d.Name===selected))$('midiInput').add(new Option(selected+' (disconnected)',selected));
 $('midiInput').value=selected||'';
}
$('refreshMIDI').onclick=()=>refreshMIDI().catch(e=>message(errorText(e)));
$('globalKeys').onchange=save;
$('midiInput').onchange=save;
async function cancelLearn(){
 pendingKey=null;pendingMIDI=null;$('cancelLearn').hidden=true;
 await nativeLearnMIDI(false);await nativeSuspendShortcuts(false);
}
$('cancelLearn').onclick=()=>cancelLearn().catch(e=>{$('triggerStatus').textContent=errorText(e);});
async function learnKeyboard(sound){
 try{await cancelLearn();await nativeSuspendShortcuts(true);pendingKey=sound;$('cancelLearn').hidden=false;$('triggerStatus').textContent='Press a shortcut for '+sound.Name+' (for example Ctrl+Alt+1). Escape cancels.';}
 catch(e){$('triggerStatus').textContent=errorText(e);}
}
async function learnMIDI(sound){
 try{
  await cancelLearn();
  if(!$('midiInput').value){openTab('settings');message('Select a MIDI controller in Settings first.');return;}
  await nativeLearnMIDI(true);const s=await nativeTriggerStatus();lastMIDISequence=s.Sequence;pendingMIDI=sound;$('cancelLearn').hidden=false;
  $('triggerStatus').textContent='Press a MIDI note or controller pad for '+sound.Name+'.';
 }catch(e){$('triggerStatus').textContent=errorText(e);}
}
document.addEventListener('keydown',async e=>{
 if(!pendingKey&&!pendingMIDI)return;
 if(e.key==='Escape'){e.preventDefault();e.stopImmediatePropagation();cancelLearn().catch(err=>message(errorText(err)));return;}
 if(!pendingKey)return;
 e.preventDefault();e.stopImmediatePropagation();
 if(['Control','Alt','Shift','Meta'].includes(e.key))return;
 if(e.metaKey){$('triggerStatus').textContent='Use Ctrl, Alt, Shift or a function key.';return;}
 const key=e.keyCode;
 if(!key||(!e.ctrlKey&&!e.altKey&&!e.shiftKey&&!/^F\d+$/.test(e.key))){$('triggerStatus').textContent='Use a modifier combination or a function key.';return;}
 const parts=[];if(e.ctrlKey)parts.push('Ctrl');if(e.altKey)parts.push('Alt');if(e.shiftKey)parts.push('Shift');parts.push(e.key.length===1?e.key.toUpperCase():e.key);
 const binding={Key:key,Modifiers:(e.altKey?1:0)|(e.ctrlKey?2:0)|(e.shiftKey?4:0),Label:parts.join('+')};
 const sound=pendingKey;pendingKey=null;$('cancelLearn').hidden=true;
 try{await nativeSuspendShortcuts(false);library=await nativeSetBindings(sound.ID,binding,sound.MIDI||null);renderPads();$('triggerStatus').textContent='Assigned '+binding.Label+' to '+sound.Name+'.';}
 catch(err){$('triggerStatus').textContent='Could not assign shortcut: '+errorText(err);}
},true);
async function pollTriggers(){
 if(triggerPollBusy)return;triggerPollBusy=true;
 try{
  const status=await nativeTriggerStatus();
  if(status.Sequence!==lastMIDISequence){
   lastMIDISequence=status.Sequence;
   if(pendingMIDI&&status.MIDI){
    const sound=pendingMIDI;pendingMIDI=null;$('cancelLearn').hidden=true;
    try{library=await nativeSetBindings(sound.ID,sound.Hotkey||null,status.MIDI);renderPads();$('triggerStatus').textContent='MIDI assigned to '+sound.Name+'.';}
    finally{await nativeLearnMIDI(false);}
   }
  }
  if(status.Message&&status.Message!==lastTriggerMessage&&!pendingMIDI&&!pendingKey){lastTriggerMessage=status.Message;$('triggerStatus').textContent=status.Message;}
 }catch(e){if(pendingMIDI){pendingMIDI=null;nativeLearnMIDI(false).catch(()=>{});$('triggerStatus').textContent=errorText(e);}}
 finally{triggerPollBusy=false;}
}
setInterval(pollTriggers,200);
$('exportBoard').onclick=async()=>{try{const path=await nativeExportBoard($('boardSelect').value);if(path)$('boardStatus').textContent='Soundboard exported with audio and trigger assignments.';}catch(e){$('boardStatus').textContent='Export failed: '+errorText(e);}};
$('importBoard').onclick=async()=>{
 $('importBoard').disabled=true;
 try{const previousCount=library.Boards.length,previousBoard=$('boardSelect').value;library=await nativeImportBoard();renderLibrary(library.Boards.length>previousCount?library.Boards.at(-1)?.ID:previousBoard);scheduleSave();$('boardStatus').textContent='Soundboard imported. Duplicate trigger assignments are left unassigned.';}
 catch(e){$('boardStatus').textContent='Import failed: '+errorText(e);}
 finally{$('importBoard').disabled=false;}
};
init();
