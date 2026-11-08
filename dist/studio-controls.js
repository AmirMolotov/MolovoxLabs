'use strict';
const filterNames={gain:'Gain',eq:'Equalizer',highpass:'High-pass',lowpass:'Low-pass',gate:'Noise gate',expander:'Downward expander',compressor:'Compressor',pitch:'Pitch',noise:'Noise reduction',addnoise:'Add noise',reverb:'Reverb',echo:'Echo',chorus:'Chorus',distortion:'Distortion',robot:'Robot'};
const filterDefaults={gain:{GainDB:0},eq:{BassDB:0,MidDB:0,TrebleDB:0},highpass:{Frequency:80},lowpass:{Frequency:12000},gate:{Threshold:-45},expander:{Threshold:-45,Ratio:2},compressor:{Threshold:-18,Ratio:4},pitch:{Semitones:0},noise:{Strength:75,NoiseDB:-45},addnoise:{NoiseLevelDB:-36},reverb:{Mix:25,RoomSize:55,Damping:45},echo:{Mix:30,DelayMS:300,Feedback:35},chorus:{Mix:40,RateHz:1.2,DepthMS:5},distortion:{Mix:65,DriveDB:18,OutputDB:-12},robot:{Mix:100,Frequency:90}};
function renderFilters(){
 $('filtersActive').checked=!bypassFilters;$('filterCards').replaceChildren();
 if(!micFilters.length){const empty=document.createElement('div');empty.className='empty';empty.textContent='No effects yet. Choose an effect above to add its parameters.';$('filterCards').append(empty);}
 micFilters.forEach((filter,index)=>{
  const card=document.createElement('div');card.className='card filter-card'+(bypassFilters?' chain-bypassed':'');card.dataset.filterId=filter.ID;
  const top=document.createElement('div');top.className='filter-top';
  const label=document.createElement('label');label.className='toggle';const enabled=document.createElement('input');enabled.type='checkbox';enabled.checked=filter.Enabled;
  enabled.onchange=()=>{filter.Enabled=enabled.checked;configure();};label.append(enabled,document.createTextNode((index+1)+'. '+filterTitle(filter)));
  const actions=document.createElement('div');actions.className='filter-actions';
  const shortcut=document.createElement('button');shortcut.className='secondary';shortcut.textContent=shortcutSummary(filter)||'⌨ Shortcuts';shortcut.onclick=()=>openTriggerEditor('@filter:'+filter.ID);actions.append(shortcut);
  for(const [text,delta] of [['↑',-1],['↓',1]]){const button=document.createElement('button');button.className='secondary filter-move '+(delta<0?'move-up':'move-down');button.textContent=text;button.setAttribute('aria-label','Move '+filterTitle(filter)+' '+(delta<0?'up':'down'));button.disabled=index+delta<0||index+delta>=micFilters.length;button.onclick=()=>{[micFilters[index],micFilters[index+delta]]=[micFilters[index+delta],micFilters[index]];renderFilters();renderGroups();configure();};actions.append(button);}
  const remove=document.createElement('button');remove.className='secondary icon-close';remove.textContent='×';remove.setAttribute('aria-label','Remove '+filterTitle(filter));remove.onclick=()=>{micFilters.splice(index,1);for(const group of filterGroups)group.FilterIDs=group.FilterIDs.filter(id=>id!==filter.ID);renderFilters();renderGroups();configure();};actions.append(remove);top.append(label,actions);card.append(top);
  const controls=renderEffectParameters(filter);
  card.append(controls);$('filterCards').append(card);
 });
 if(patchGraph)renderPatch();
}
function renderEffectParameters(filter,inline=false){
  const controls=document.createElement('div');controls.className='filter-controls';
  const range=(field,title,min,max,step,unit)=>{
   const wrap=document.createElement('div'),head=document.createElement('div');head.className='rangehead';const name=document.createElement('label'),readout=document.createElement('output');name.textContent=title;
   const slider=document.createElement('input');slider.type='range';slider.min=min;slider.max=max;slider.step=step;slider.value=filter[field];slider.setAttribute('aria-label',filterTitle(filter)+' '+title);readout.textContent=filter[field]+unit;
   slider.dataset.effectParameter=filter.ID+':'+field;
   slider.oninput=()=>{
    filter[field]=Number(slider.value);
    readout.textContent=slider.value+unit;
    for(const input of document.querySelectorAll('[data-effect-parameter]')){
     if(input===slider||input.dataset.effectParameter!==slider.dataset.effectParameter)continue;
     input.value=slider.value;input.previousElementSibling.querySelector('output').textContent=readout.textContent;
    }
    configure(true);
   };head.append(name,readout);wrap.append(head,slider);controls.append(wrap);
  };
  if(filter.Kind==='vst3')renderVSTFilter(filter,controls,inline);
  if(['reverb','echo','chorus','distortion','robot'].includes(filter.Kind))range('Mix','Effect mix',0,100,1,'%');
  if(filter.Kind==='reverb'){range('RoomSize','Room size',0,100,1,'%');range('Damping','Damping',0,100,1,'%');}
  if(filter.Kind==='echo'){range('DelayMS','Delay',20,1000,10,' ms');range('Feedback','Feedback',0,85,1,'%');}
  if(filter.Kind==='chorus'){range('RateHz','Rate',.1,5,.1,' Hz');range('DepthMS','Depth',0,15,.5,' ms');}
  if(filter.Kind==='distortion'){range('DriveDB','Drive',0,36,1,' dB');range('OutputDB','Effect output',-24,0,1,' dB');}
  if(filter.Kind==='robot')range('Frequency','Robot frequency',20,2000,10,' Hz');
  if(filter.Kind==='addnoise'){range('NoiseLevelDB','White noise level',-80,-6,1,' dB');const hint=document.createElement('p');hint.className='hint';hint.textContent='Mixes white noise into your microphone. Move it before or after other filters to change how they affect the noise.';controls.append(hint);}
  if(filter.Kind==='noise'){range('Strength','Reduction',0,100,1,'%');range('NoiseDB','Background noise level',-80,-15,1,' dB');const hint=document.createElement('p');hint.className='hint';hint.textContent='Raise the background noise level until hiss fades. Lower it if your voice sounds thin. Adds about 11 ms of delay.';controls.append(hint);}
  if(filter.Kind==='pitch')range('Semitones','Pitch',-12,12,.5,' semitones');
  if(filter.Kind==='eq')controls.append(renderEqualizer(filter));
  if(filter.Kind==='gain')range('GainDB','Gain',-24,24,.5,' dB');
  if(filter.Kind==='highpass')range('Frequency','Cutoff',20,1000,10,' Hz');
  if(filter.Kind==='lowpass')range('Frequency','Cutoff',1000,20000,100,' Hz');
  if(filter.Kind==='gate')range('Threshold','Threshold',-90,-5,1,' dB');
  if(filter.Kind==='expander'){range('Threshold','Threshold',-90,0,1,' dB');range('Ratio','Expansion ratio',1,10,.1,':1');}
  if(filter.Kind==='compressor'){range('Threshold','Threshold',-60,0,1,' dB');range('Ratio','Ratio',1,20,.5,':1');}
 return controls;
}
function renderGroups(){
 const selected=new Set([...$('groupMembers').querySelectorAll('input:checked')].map(x=>x.value));$('groupMembers').replaceChildren();$('filterGroups').replaceChildren();
 micFilters.forEach((filter,index)=>{const label=document.createElement('label');label.className='toggle';const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.value=filter.ID;checkbox.checked=selected.has(filter.ID);label.append(checkbox,document.createTextNode((index+1)+'. '+filterTitle(filter)));$('groupMembers').append(label);});
 for(const group of filterGroups){
  const row=document.createElement('div');row.className='filter-group-row';const name=document.createElement('div');const title=document.createElement('strong');title.textContent=group.Name;const count=document.createElement('small');count.textContent=group.FilterIDs.length+' filters';name.append(title,count);
  const actions=document.createElement('div');actions.className='filter-actions';
  const toggle=document.createElement('button');toggle.className='secondary';toggle.textContent='Toggle group';toggle.disabled=!group.FilterIDs.length;toggle.onclick=async()=>{await save();try{const p=await nativeToggleFilters('@group:'+group.ID);syncFilterState(p.Options);}catch(e){message(errorText(e));}};
  const key=document.createElement('button');key.className='secondary';key.textContent='⌨ Shortcuts';key.onclick=()=>openTriggerEditor('@group:'+group.ID);
  const remove=document.createElement('button');remove.className='secondary icon-close';remove.textContent='×';remove.setAttribute('aria-label','Remove filter group '+group.Name);remove.onclick=()=>{filterGroups=filterGroups.filter(x=>x.ID!==group.ID);renderGroups();save();};
  actions.append(toggle,key,remove);row.append(name,actions);$('filterGroups').append(row);
 }
}
$('filterKind').onchange=()=>{const kind=$('filterKind').value;$('filterKind').value='';if(!filterDefaults[kind])return;micFilters.push({ID:crypto.randomUUID(),Kind:kind,Enabled:true,Hotkeys:[],MIDIs:[],...filterDefaults[kind]});renderFilters();renderGroups();configure();};
$('filtersActive').onchange=()=>{bypassFilters=!$('filtersActive').checked;renderFilters();configure();};
$('allFilterShortcuts').onclick=()=>openTriggerEditor('@all');
$('createGroup').onclick=async()=>{
 const ids=[...$('groupMembers').querySelectorAll('input:checked')].map(x=>x.value),name=$('groupName').value.trim();
 if(!name||!ids.length){message('Name the group and select its filters.');return;}
 filterGroups.push({ID:crypto.randomUUID(),Name:name,FilterIDs:ids,Hotkeys:[],MIDIs:[]});$('groupName').value='';for(const input of $('groupMembers').querySelectorAll('input'))input.checked=false;renderGroups();await save();
};
function syncFilterState(o){
 if(!o)return;const states=new Map((o.Filters||[]).map(f=>[f.ID,f.Enabled]));
 for(const f of micFilters)if(states.has(f.ID))f.Enabled=states.get(f.ID);
 bypassFilters=!!o.BypassFilters;renderFilters();
}
window.addEventListener('filter-state',event=>syncFilterState(event.detail));
let editorID=null,pendingKey=false,pendingMIDI=false,lastMIDISequence=0,lastTriggerMessage='',triggerPollBusy=false,lastFilterRevision=0,committing=false,learningEpoch=0;
function targetData(){
 if(editorID==='@all')return {Name:'Entire filter chain',Keys:allFilterHotkeys,MIDIs:allFilterMIDIs,Description:'Toggle bypass for the whole chain. Individual filter states are preserved.'};
 if(editorID?.startsWith('@filter:')){const f=micFilters.find(f=>f.ID===editorID.slice(8));if(f)return {Name:filterTitle(f),Keys:f.Hotkeys||[],MIDIs:f.MIDIs||[],Description:'Each assigned trigger toggles this filter on or off.'};}
 if(editorID?.startsWith('@group:')){const g=filterGroups.find(g=>g.ID===editorID.slice(7));if(g)return {Name:g.Name,Keys:g.Hotkeys||[],MIDIs:g.MIDIs||[],Description:'Toggle the selected filters together, leaving other filters unchanged.'};}
 for(const board of library.Boards){const s=board.Sounds.find(s=>s.ID===editorID);if(s)return {Name:s.Name,Keys:clipKeys(s),MIDIs:clipMIDIs(s),Sound:s,Description:'Add as many alternate keyboard and MIDI triggers as you need.'};}
 return null;
}
async function openTriggerEditor(id){
 editorID=id;if(!targetData())return;renderTriggerEditor();$('triggerStatus').textContent='Assignments are saved immediately.';$('triggerDialog').showModal();await populateTriggerMIDI();
}
function renderTriggerEditor(){
 const target=targetData();if(!target)return;
 $('triggerTitle').textContent=target.Name;$('triggerDescription').textContent=target.Description;$('exportAudio').hidden=!target.Sound;
 $('keyList').replaceChildren();$('midiList').replaceChildren();
 const chip=(text,remove)=>{const item=document.createElement('span');item.className='trigger-chip';const label=document.createElement('span');label.textContent=text;const button=document.createElement('button');button.textContent='×';button.setAttribute('aria-label','Remove '+text);button.onclick=remove;item.append(label,button);return item;};
 target.Keys.forEach((k,index)=>$('keyList').append(chip(k.Label,()=>commitTarget(target.Keys.filter((_,i)=>i!==index),target.MIDIs))));
 target.MIDIs.forEach((m,index)=>$('midiList').append(chip((m.Kind==='note'?'Note ':'CC ')+m.Number+' · channel '+m.Channel,()=>commitTarget(target.Keys,target.MIDIs.filter((_,i)=>i!==index)))));
 if(!target.Keys.length){const empty=document.createElement('p');empty.className='hint';empty.textContent='No keyboard shortcuts yet.';$('keyList').append(empty);}
 if(!target.MIDIs.length){const empty=document.createElement('p');empty.className='hint';empty.textContent='No MIDI triggers yet.';$('midiList').append(empty);}
}
async function commitTarget(keys,midis){
 if(committing||!editorID)return;committing=true;
 try{
  const result=await nativeSetTriggerTarget(editorID,keys,midis);library=result.Library;preferences=result.Preferences;loadFilterModels(preferences);renderPads();renderSamplerBoard();renderTriggerEditor();if(typeof renderShortcuts==='function')renderShortcuts();$('triggerStatus').textContent='Saved. All assigned triggers remain active.';
 }catch(e){$('triggerStatus').textContent=errorText(e);}
 finally{committing=false;}
}
function showLearningButtons(){const learning=pendingKey||pendingMIDI;$('cancelLearn').hidden=!learning;$('addKey').textContent=pendingKey?'Listening for shortcut…':'＋ Add shortcut';$('addMIDI').textContent=pendingMIDI?'Listening for MIDI…':'＋ Learn MIDI';}
async function cancelLearning(){learningEpoch++;pendingKey=false;pendingMIDI=false;showLearningButtons();const results=await Promise.allSettled([nativeLearnMIDI(false),nativeSuspendShortcuts(false)]);for(const result of results)if(result.status==='rejected')$('triggerStatus').textContent=errorText(result.reason);}
async function closeEditor(){await cancelLearning();$('triggerDialog').close();editorID=null;}
async function populateTriggerMIDI(){try{const devices=await nativeMIDIDevices(),select=$('triggerMIDIInput'),previous=select.value||$('midiInput').value;select.replaceChildren(new Option(devices.length?'Choose a controller…':'No MIDI controllers connected',''));for(const device of devices)select.add(new Option(device.Name,device.Name));if(devices.some(d=>d.Name===previous))select.value=previous;else if(devices.length===1)select.value=devices[0].Name;}catch(e){$('triggerStatus').textContent='Could not list MIDI controllers: '+errorText(e);}}
$('refreshTriggerMIDI').onclick=populateTriggerMIDI;
$('closeTriggerDialog').onclick=closeEditor;
$('triggerDialog').addEventListener('cancel',event=>{event.preventDefault();if(pendingKey||pendingMIDI)cancelLearning();else closeEditor();});
$('cancelLearn').onclick=()=>cancelLearning();
$('clearAllTriggers').onclick=()=>commitTarget([],[]);
$('addKey').onclick=async()=>{const epoch=++learningEpoch;pendingMIDI=false;pendingKey=false;$('triggerStatus').textContent='Preparing keyboard capture…';try{await nativeLearnMIDI(false);await nativeSuspendShortcuts(true);if(epoch!==learningEpoch){await nativeSuspendShortcuts(false);return;}pendingKey=true;showLearningButtons();$('triggerStatus').textContent='Press a shortcut, for example Ctrl+Alt+1. Escape cancels.';}catch(e){pendingKey=false;showLearningButtons();$('triggerStatus').textContent='Could not start shortcut learning: '+errorText(e);}};
$('addMIDI').onclick=async()=>{const epoch=++learningEpoch;pendingKey=false;pendingMIDI=false;showLearningButtons();const device=$('triggerMIDIInput').value;if(!device){$('triggerStatus').textContent='Connect a MIDI controller and choose it above, then press Learn MIDI. Use Refresh controllers after connecting.';$('triggerMIDIInput').focus?.();await nativeLearnMIDI(false);await nativeSuspendShortcuts(false).catch(()=>{});return;}
 try{$('triggerStatus').textContent='Opening MIDI controller…';await nativeLearnMIDI(false);await nativeSuspendShortcuts(true);await nativeSelectMIDIInput(device);if(epoch!==learningEpoch){await nativeSuspendShortcuts(false);return;}
  const settings=$('midiInput');if(![...settings.options].some(option=>option.value===device))settings.add(new Option(device,device));settings.value=device;if(preferences)preferences.MIDIInput=device;
  const status=await nativeTriggerStatus();lastMIDISequence=status.Sequence;if(epoch!==learningEpoch){await nativeSuspendShortcuts(false);return;}pendingMIDI=true;showLearningButtons();await nativeLearnMIDI(true);if(epoch!==learningEpoch){await nativeLearnMIDI(false);await nativeSuspendShortcuts(false);return;}$('triggerStatus').textContent='Press a MIDI note or controller. Escape cancels.';
 }catch(e){pendingMIDI=false;showLearningButtons();await nativeSuspendShortcuts(false).catch(()=>{});$('triggerStatus').textContent='Could not start MIDI learning: '+errorText(e);}};
function windowsKeyForEvent(event){if(event.keyCode>0&&event.keyCode<=255)return event.keyCode;if(/^F([1-9]|1[0-9]|2[0-4])$/.test(event.key))return 111+Number(event.key.slice(1));if(event.key.length===1&&/[a-z0-9]/i.test(event.key))return event.key.toUpperCase().charCodeAt(0);const codes={Space:32,Enter:13,Tab:9,Backspace:8,Delete:46,Insert:45,Home:36,End:35,PageUp:33,PageDown:34,ArrowLeft:37,ArrowUp:38,ArrowRight:39,ArrowDown:40};return codes[event.code]||0;}
document.addEventListener('keydown',async event=>{
 if(!pendingKey&&!pendingMIDI)return;
 if(event.key==='Escape'){event.preventDefault();event.stopImmediatePropagation();cancelLearning().catch(e=>message(errorText(e)));return;}
 if(!pendingKey)return;event.preventDefault();event.stopImmediatePropagation();
 if(['Control','Alt','Shift','Meta'].includes(event.key))return;
 if(event.metaKey||!windowsKeyForEvent(event)||(!event.ctrlKey&&!event.altKey&&!event.shiftKey&&!/^F\d+$/.test(event.key))){$('triggerStatus').textContent='Use Ctrl, Alt, Shift or a function key.';return;}
 const parts=[];if(event.ctrlKey)parts.push('Ctrl');if(event.altKey)parts.push('Alt');if(event.shiftKey)parts.push('Shift');parts.push(event.key.length===1?event.key.toUpperCase():event.key);
 const key={Key:windowsKeyForEvent(event),Modifiers:(event.altKey?1:0)|(event.ctrlKey?2:0)|(event.shiftKey?4:0),Label:parts.join('+')};
 pendingKey=false;showLearningButtons();
 try{await nativeSuspendShortcuts(false);const target=targetData();if(target)await commitTarget([...target.Keys,key],target.MIDIs);}catch(e){$('triggerStatus').textContent=errorText(e);}
},true);
$('exportAudio').onclick=async()=>{try{const target=targetData();if(target?.Sound){const path=await withLoading('Exporting original audio…',()=>nativeExportAudio(target.Sound.ID),true);if(path)$('triggerStatus').textContent='Original '+target.Sound.File.split('.').pop().toUpperCase()+' audio exported.';}}catch(e){$('triggerStatus').textContent=errorText(e);}};
async function refreshMIDI(preferred){
 const selected=preferred??$('midiInput').value,list=await withLoading('Finding MIDI devices…',()=>nativeMIDIDevices());$('midiInput').replaceChildren(new Option('Off',''));
 for(const d of list)$('midiInput').add(new Option(d.Name,d.Name));if(selected&&!list.some(d=>d.Name===selected))$('midiInput').add(new Option(selected+' (disconnected)',selected));$('midiInput').value=selected||'';
}
$('refreshMIDI').onclick=()=>refreshMIDI().catch(e=>message(errorText(e)));
$('globalKeys').onchange=save;$('midiInput').onchange=save;
async function pollTriggers(){
 if(triggerPollBusy)return;triggerPollBusy=true;
 try{
  const status=await nativeTriggerStatus();
  if(status.FilterRevision!==lastFilterRevision){lastFilterRevision=status.FilterRevision;syncFilterState(status.FilterOptions);refreshVSTCards();}
  if(status.Sequence!==lastMIDISequence){
   lastMIDISequence=status.Sequence;
   if(pendingMIDI&&status.MIDI){pendingMIDI=false;showLearningButtons();try{await nativeLearnMIDI(false);await nativeSuspendShortcuts(false);const target=targetData();if(target)await commitTarget(target.Keys,[...target.MIDIs,status.MIDI]);}finally{await nativeLearnMIDI(false);}}
  }
  if(status.Message&&status.Message!==lastTriggerMessage&&!pendingMIDI&&!pendingKey){lastTriggerMessage=status.Message;message(status.Message);}
 }catch(e){if(pendingMIDI){pendingMIDI=false;nativeLearnMIDI(false).catch(()=>{});$('triggerStatus').textContent=errorText(e);}}
 finally{triggerPollBusy=false;}
}
// Keep the menu open while using its controls, dismiss it elsewhere.
document.addEventListener('click',event=>{
 for(const menu of document.querySelectorAll('.board-menu[open]')){
  if(!menu.contains(event.target))menu.open=false;
 }
});
document.addEventListener('keydown',event=>{
 if(event.key==='Escape')for(const menu of document.querySelectorAll('.board-menu[open]'))menu.open=false;
});
setInterval(pollTriggers,200);
init();
