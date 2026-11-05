'use strict';
let vstCatalog=[],vstEditorID=null,vstParameters=[],vstPersistBusy=false;
function filterTitle(filter){const title=filter.Kind==='vst3'?(filter.PluginName||'VST3 plugin'):filterNames[filter.Kind];return filter.ClipID?title+' · '+(library.Boards.flatMap(b=>b.Sounds).find(sound=>sound.ID===filter.ClipID)?.Name||'Pad'):title;}
function vstFolders(){return $('vstFolders').value.split(/\r?\n/).map(x=>x.trim()).filter(Boolean);}
function showVSTCatalog(result){
 vstCatalog=result.Plugins||[];$('vstSelect').replaceChildren(new Option('Choose a VST3 effect to add…',''));
 $('vstSelect').options[0].disabled=true;
 vstCatalog.forEach((plugin,index)=>$('vstSelect').add(new Option(plugin.Name+(plugin.Vendor?' · '+plugin.Vendor:''),String(index))));
 $('vstSelect').value='';$('vstSelect').disabled=!vstCatalog.length;
 $('vstScanStatus').textContent=vstCatalog.length+' audio effect'+(vstCatalog.length===1?'':'s')+' found.'+((result.Errors||[]).length?' '+result.Errors.join(' · '):'');
}
async function initVST(p){$('vstFolders').value=(p.VSTFolders||[]).join('\n');try{showVSTCatalog(await nativeLoadVSTPlugins());}catch(e){$('vstScanStatus').textContent=errorText(e);}}
$('scanVST').onclick=async()=>{
 const finishLoading=beginLoading('Scanning VST3 plugins…');$('scanVST').disabled=true;$('vstScanStatus').textContent='Scanning installed VST3 effects… Each plugin is checked in a separate process.';
 try{await paintLoading();await save();await nativeScanVST(vstFolders());let scan;do{await new Promise(resolve=>setTimeout(resolve,250));scan=await nativeVSTScanStatus();}while(scan.Running);showVSTCatalog(scan.Result);}catch(e){$('vstScanStatus').textContent=errorText(e);}finally{$('scanVST').disabled=false;finishLoading();}
};
$('vstFolders').onchange=save;
$('vstSelect').onchange=async()=>{
 const index=$('vstSelect').value;$('vstSelect').value='';const plugin=vstCatalog[Number(index)];if(index===''||!plugin)return;
 if(micFilters.length>=8){message('Maximum eight microphone filters.');return;}
 const filter={ID:crypto.randomUUID(),Kind:'vst3',Enabled:true,Hotkeys:[],MIDIs:[],PluginPath:plugin.Path,ClassID:plugin.ClassID,PluginName:plugin.Name,VSTState:''};
 await withLoading('Loading '+plugin.Name+'…',async()=>{micFilters.push(filter);renderFilters();renderGroups();await configure();await save();refreshVSTCards();},true);
};
function renderVSTFilter(filter,controls){
 const box=document.createElement('div');box.className='vst-filter';box.dataset.vstId=filter.ID;
 const label=document.createElement('p');label.className='hint';label.dataset.vstStatus='';label.textContent='Checking plugin…';
 const row=document.createElement('div');row.className='vst-buttons';
 const editor=document.createElement('button');editor.className='secondary';editor.textContent='Open plugin editor';editor.onclick=async()=>{try{await save();await withLoading('Opening plugin editor…',()=>nativeVSTEditor(filter.ID),true);}catch(e){label.textContent=errorText(e);}};
 const params=document.createElement('button');params.className='secondary';params.textContent='Parameters';params.onclick=()=>openVSTParameters(filter);
 row.append(editor,params);box.append(row,label);controls.append(box);
 queueMicrotask(()=>refreshVSTCard(filter,box));
}
async function refreshVSTCard(filter,box){
 try{const info=await nativeVSTInfo(filter.ID);if(!box.isConnected)return;
  box.querySelector('[data-vst-status]').textContent=info.Error||('VST3 · '+((128+(info.Latency||0))/48).toFixed(1)+' ms processing delay'+(info.NeedsRestart?' · Plugin requested a restart; remove and add it again.':''));
  box.querySelectorAll('button').forEach(button=>button.disabled=!!info.Error);
 }catch(e){box.querySelector('[data-vst-status]').textContent=errorText(e);}
}
function refreshVSTCards(){for(const box of document.querySelectorAll('[data-vst-id]')){const filter=micFilters.find(f=>f.ID===box.dataset.vstId);if(filter)refreshVSTCard(filter,box);}}
async function openVSTParameters(filter){
 try{await save();const info=await withLoading('Loading plugin parameters…',()=>nativeVSTInfo(filter.ID),true);if(info.Error)throw new Error(info.Error);vstEditorID=filter.ID;vstParameters=(info.Parameters||[]).filter(p=>!p.Hidden);$('vstTitle').textContent=filter.PluginName;$('vstSearch').value='';renderVSTParameters();$('vstDialog').showModal();}catch(e){message(errorText(e));}
}
function renderVSTParameters(){
 const search=$('vstSearch').value.toLowerCase(),list=vstParameters.filter(p=>p.Name.toLowerCase().includes(search));$('vstParamList').replaceChildren();
 for(const param of list){
  const row=document.createElement('label');row.className='vst-param';const title=document.createElement('span');title.textContent=param.Name;
  const value=document.createElement('output');value.textContent=param.Display||Math.round(param.Value*100)+'%';
  const slider=document.createElement('input');slider.type='range';slider.min=0;slider.max=1;slider.step=param.Steps>0?1/param.Steps:.001;slider.value=param.Value;slider.disabled=param.ReadOnly;slider.setAttribute('aria-label',param.Name);
  const target=vstEditorID;let timer;
  const apply=async()=>{clearTimeout(timer);try{await nativeVSTParameter(target,param.ID,Number(slider.value));param.Value=Number(slider.value);$('vstParamStatus').textContent='Applied. Plugin settings are saved automatically.';}catch(e){$('vstParamStatus').textContent=errorText(e);}};
  slider.oninput=()=>{value.textContent=Math.round(Number(slider.value)*100)+'%';clearTimeout(timer);timer=setTimeout(apply,60);};slider.onchange=apply;
  row.append(title,value,slider);$('vstParamList').append(row);
 }
 $('vstParamStatus').textContent=list.length+' parameter'+(list.length===1?'':'s')+'.';
}
$('vstSearch').oninput=renderVSTParameters;
$('closeVSTDialog').onclick=()=>$('vstDialog').close();
setInterval(async()=>{
 if(!ready||vstPersistBusy||!micFilters.some(f=>f.Kind==='vst3'))return;vstPersistBusy=true;
 try{await nativeSaveVSTState();if(currentTab==='patch')refreshVSTCards();}catch(e){message('Plugin settings could not be saved: '+errorText(e));}finally{vstPersistBusy=false;}
},5000);
