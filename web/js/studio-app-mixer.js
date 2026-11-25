const setAppMixerVisible=(()=>{
 const mixer=$('appMixer');
 const list=$('appMixerList');
 const status=$('appMixerStatus'),groupList=$('appMixerGroups');
 const panel=$('appMixerGroupPanel'),controls=$('appMixerGroupControls'),newGroup=$('appMixerNewGroup');
 const empty=$('appMixerEmpty'),offline=$('appMixerOffline');
 let groupLayout='',selectedGroup=null,draggedPath='',tabs=[];
 const groups=()=>preferences?.AppMixerGroups||[];
 const samePath=(a,b)=>a.toLowerCase()===b.toLowerCase();
 const baseName=path=>path.split(/[\\/]/).pop();
 const groupFor=path=>groups().find(group=>(group.Paths||[]).some(member=>samePath(member,path)));
 const rows=new Map();
 let timer=null, visible=null, scanning=false, revision=0, writes=0, writeError='';

 function updateStatus(){
  if(status.textContent!==writeError)status.textContent=writeError;
 }

 function updateGroupRows(){
  const group=groups().find(item=>item.ID===selectedGroup);
  let visibleRows=0;
  for(const [path,row] of rows){
   row.element.hidden=selectedGroup!==null&&(groupFor(path)?.ID||'')!==selectedGroup;
   if(!row.element.hidden)visibleRows++;
  }
  empty.hidden=visibleRows>0;
  const text=group?(group.Paths?.length?'No recent audio in '+group.Name+'.':'Drag an app icon onto this group to add it.')
   :selectedGroup===''?'No ungrouped apps.':'No recent app audio yet.';
  if(empty.textContent!==text)empty.textContent=text;
  const unavailable=(group?.Paths||[]).filter(path=>![...rows.keys()].some(recent=>samePath(path,recent)));
  offline.hidden=!unavailable.length;
  const members=unavailable.length?'Other members: '+unavailable.map(baseName).join(', '):'';
  if(offline.textContent!==members)offline.textContent=members;
 }

 function shortcutPicker(target,label,prefix=''){
  const select=document.createElement('select');
  select.setAttribute('aria-label','Shortcuts and MIDI for '+label);
  select.add(new Option('Shortcuts & MIDI…',''));
  select.options[0].disabled=true;
  for(const [action,name] of Object.entries(appMixerActions))select.add(new Option(name,action));
  select.onchange=()=>{
   const action=select.value;select.value='';
   if(action)openTriggerEditor('@app:'+prefix+action+':'+target);
  };
  return select;
 }

 function saveGroups(update,afterSave){
  return queueSettings(async()=>{
   const next=update(structuredClone(groups()));
   const value=snapshotPreferences();
   const retained=new Set(next.map(group=>group.ID));
   value.AppMixerGroups=next;
   value.AppMixerShortcuts=value.AppMixerShortcuts.filter(binding=>{
    const id=binding.ID.match(/^@app:group-[^:]+:(.*)$/)?.[1];
    return !id||retained.has(id);
   });
   await nativeSavePreferences(value);
   preferences=value;
   writeError='';
   syncGroups();
   afterSave?.();
   if(currentTab==='shortcuts')renderShortcuts();
  }).catch(error=>{writeError='Could not save mixer groups. '+errorText(error);syncGroups(true);})
   .finally(updateStatus);
 }

 function createGroup(path){
  const id=crypto.randomUUID();
  return saveGroups(next=>{
   let number=1;
   while(next.some(group=>group.Name==='Group '+number))number++;
   if(path)for(const group of next)group.Paths=(group.Paths||[]).filter(member=>!samePath(member,path));
   return [...next,{ID:id,Name:'Group '+number,Paths:path?[path]:[]}];
  },()=>{
   selectGroup(id);
   const name=controls.firstElementChild;
   if(visible){name.focus();name.select();}
  });
 }

 function moveApp(path,groupID){
  if(groupID==='new')return createGroup(path);
  if(groupID===(groupFor(path)?.ID||''))return;
  return saveGroups(next=>{
   for(const group of next){
    group.Paths=(group.Paths||[]).filter(member=>!samePath(member,path));
    if(group.ID===groupID)group.Paths.push(path);
   }
   return next;
  },()=>{
   // Keep the source view while organizing All apps; the row badge shows its destination.
   if(selectedGroup!==null)selectGroup(groupID,true);
  });
 }

 function dropTarget(element,drop){
  element.ondragover=e=>{
   if(!draggedPath)return;
   e.preventDefault();e.dataTransfer.dropEffect='move';
   element.classList.toggle('is-drop-target',true);
  };
  element.ondragleave=e=>{if(!element.contains(e.relatedTarget))element.classList.toggle('is-drop-target',false);};
  element.ondrop=e=>{
   if(!draggedPath)return;
   e.preventDefault();
   const path=draggedPath;endDrag();return drop(path);
  };
 }
 function endDrag(){
  draggedPath='';mixer.classList.toggle('is-grouping',false);
  for(const target of mixer.querySelectorAll('.is-drop-target'))target.classList.toggle('is-drop-target',false);
 }
 newGroup.onclick=()=>createGroup();
 dropTarget(newGroup,createGroup);

 function updateRowGroup(path,row){
  const group=groupFor(path);
  row.badge.textContent=group?.Name||'Ungrouped';
  row.badge.title=row.badge.textContent;
  row.badge.classList.toggle('is-ungrouped',!group);
  row.groupMenu.replaceChildren(new Option('Move to group\u2026','choose'),new Option('New group\u2026','new'));
  row.groupMenu.options[0].disabled=true;
  if(group)row.groupMenu.add(new Option('Ungrouped',''));
  for(const destination of groups()){
   if(destination.ID!==group?.ID)row.groupMenu.add(new Option(destination.Name,destination.ID));
  }
  row.groupMenu.value='choose';
 }

 function selectGroup(id,focus=false){
  selectedGroup=id;
  for(const tab of tabs){
   const selected=tab.group===id;
   tab.button.setAttribute('aria-selected',String(selected));
   tab.button.tabIndex=selected?0:-1;
   if(selected){
    panel.setAttribute('aria-labelledby',tab.button.id);
    if(focus&&visible)tab.button.focus();
   }
  }
  const group=groups().find(item=>item.ID===id);
  controls.replaceChildren();controls.hidden=!group;
  if(group){
   const name=document.createElement('input');
   name.type='text';name.value=group.Name;name.maxLength=50;
   name.setAttribute('aria-label','Group name');
   name.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();name.blur();}};
   name.onchange=()=>{
    const title=name.value.trim();
    if(!title||title===group.Name){name.value=group.Name;return;}
    saveGroups(next=>{const item=next.find(item=>item.ID===group.ID);if(item)item.Name=title;return next;});
   };
   const remove=document.createElement('button');
   remove.type='button';remove.className='text-button app-mixer-delete';remove.textContent='Delete group';
   remove.setAttribute('aria-label','Delete '+group.Name);
   remove.onclick=()=>saveGroups(next=>next.filter(item=>item.ID!==id),()=>selectGroup('',true));
   controls.append(name,shortcutPicker(group.ID,group.Name,'group-'),remove);
  }
  updateGroupRows();
 }

 function syncGroups(force=false){
  const layout=JSON.stringify(groups());
  if(!force&&layout===groupLayout)return;
  groupLayout=layout;
  const focusTab=groupList.contains(document.activeElement);
  groupList.replaceChildren();
  tabs=[{ID:null,Name:'All apps'},{ID:'',Name:'Ungrouped'},...groups()].map((group,index)=>{
   const button=document.createElement('button');
   button.type='button';button.id='appMixerGroupTab'+index;button.className='app-mixer-pill';
   button.setAttribute('role','tab');button.setAttribute('aria-controls',panel.id);
   const name=document.createElement('span');name.textContent=group.Name;button.append(name);
   if(group.ID){
    const paths=group.Paths||[];
    const count=document.createElement('span');count.className='app-mixer-count';count.textContent=paths.length;
    button.append(count);button.title=paths.map(baseName).join(', ');
   }
   button.onclick=()=>selectGroup(group.ID);
   button.onkeydown=e=>{
    const next={ArrowLeft:index-1,ArrowRight:index+1,Home:0,End:tabs.length-1}[e.key];
    if(next===undefined)return;
    e.preventDefault();selectGroup(tabs[(next+tabs.length)%tabs.length].group,true);
   };
   if(group.ID!==null)dropTarget(button,path=>moveApp(path,group.ID));
   groupList.append(button);
   return {group:group.ID,button};
  });
  for(const [path,row] of rows)updateRowGroup(path,row);
  selectGroup(tabs.some(tab=>tab.group===selectedGroup)?selectedGroup:null,focusTab);
 }

 function addRow(app){
  const element=document.createElement('div');
  element.className='app-mixer-row';
  const identity=document.createElement('div');
  identity.className='app-mixer-identity';
  const icon=document.createElement('span');
  icon.className='app-mixer-icon';
  icon.setAttribute('aria-hidden','true');
  if(app.Icon){
   const image=document.createElement('img');
   image.alt='';
   image.onerror=()=>image.remove();
   icon.append(image);
   image.src='data:image/x-icon;base64,'+app.Icon;
  }
  const name=document.createElement('span');
  name.className='app-mixer-name';
  const input=document.createElement('input');
  const output=document.createElement('output');
  const scale=document.createElement('div');
  scale.className='app-mixer-scale';
  scale.setAttribute('aria-hidden','true');
  input.type='range';
  input.min=0;
  input.max=100;
  input.step=10;
  input.setAttribute('aria-label',app.Name+' volume');
  for(let volume=0;volume<=100;volume+=10){
   const mark=document.createElement('span');
   mark.textContent=volume+'%';
   scale.append(mark);
  }
  name.textContent=app.Name;
  name.title=app.Path;
  output.setAttribute('aria-hidden','true');
  const badge=document.createElement('span');badge.className='app-mixer-membership';
  identity.append(icon,name,badge);
  const activity=document.createElement('span');
  activity.className='app-mixer-activity';
  const shortcut=shortcutPicker(app.Path,app.Name);
  const groupMenu=document.createElement('select');
  groupMenu.setAttribute('aria-label','Move '+app.Name+' to a mixer group');
  groupMenu.onchange=()=>{const destination=groupMenu.value;groupMenu.value='choose';return moveApp(app.Path,destination);};
  icon.draggable=true;icon.title='Drag to a group pill';
  for(const image of icon.children)image.draggable=false;
  icon.ondragstart=e=>{
   draggedPath=app.Path;e.dataTransfer.effectAllowed='move';e.dataTransfer.setData('text/plain',app.Path);
   mixer.classList.toggle('is-grouping',true);
  };
  icon.ondragend=endDrag;
  const actions=document.createElement('div');
  actions.className='app-mixer-actions';
  actions.append(activity,groupMenu,shortcut);
  element.append(identity,output,input,scale,actions);
  function showVolume(volume){
   const percent=volume+'%';
   output.textContent=percent;
   input.setAttribute('aria-valuetext',percent);
   input.style.setProperty('--app-mixer-volume',input.value+'%');
  }
  const row={element,input,showVolume,activity,groupMenu,badge,lastPeak:0,saving:false,queued:null};
  input.oninput=async()=>{
   const volume=Number(input.value);
   showVolume(volume);
   row.queued=volume;
   if(row.saving)return;
   row.saving=true;
   writes++;
   revision++;
   while(row.queued!==null){
    const volume=row.queued;
    row.queued=null;
    try{
     await nativeSetAppMixerVolume(app.Path,volume);
     writeError='';
    }catch(error){
     writeError='Could not set '+app.Name+' volume. '+errorText(error);
    }
   }
   row.saving=false;
   writes--;
   if(!mixer.hidden)updateStatus();
  };
  rows.set(app.Path,row);list.append(element);updateRowGroup(app.Path,row);
  return row;
 }

 async function refresh(){
  if(scanning||writes||draggedPath)return;
  syncGroups();
  scanning=true;
  const started=revision;
  try{
   const apps=await nativeListAppMixer([...rows.keys()]);
   if(started!==revision||draggedPath)return;
   const now=performance.now(),sessions=new Map(apps.map(app=>[app.Path,app]));
   const recent=new Map([...rows].map(([path,row])=>[path,row.lastPeak]));
   for(const app of apps)if(app.Playing)recent.set(app.Path,now);
   const retained=[...recent].sort((a,b)=>b[1]-a[1]).slice(0,10);
   const keep=new Set(retained.map(([path])=>path));
   for(const [path,row] of rows){
    if(keep.has(path))continue;
    if(row.element.contains(document.activeElement))mixer.focus();
    row.element.remove();
    rows.delete(path);
   }
   for(const [path,lastPeak] of retained){
    const app=sessions.get(path),row=rows.get(path)||addRow(app);
    row.lastPeak=lastPeak;
    // Keep short pauses in speech or music from flickering between colors.
    const playing=!!app&&(app.Playing||now-lastPeak<=750);
    row.element.classList.toggle('is-recent',!playing);
    row.activity.textContent=playing?'Playing':app?'Idle':'Session closed';
    row.input.disabled=!app;
    if(app&&!row.input.matches(':active')){
     row.input.value=app.Volume;
     row.showVolume(app.Volume);
    }
   }
   updateGroupRows();updateStatus();
  }catch(error){
   if(started!==revision)return;
   const text='Could not read app audio. '+errorText(error);
   if(status.textContent!==text)status.textContent=text;
  }finally{
   scanning=false;
  }
 }

 return open=>{
  if(open===visible)return;
  visible=open;
  if(timer!==null)clearInterval(timer);
  if(open&&!rows.size)status.textContent='Checking for app audio…';
  refresh();
  timer=setInterval(refresh,open?250:1000);
 };
})();
