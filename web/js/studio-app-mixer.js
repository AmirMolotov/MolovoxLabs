const setAppMixerVisible=(()=>{
 const mixer=$('appMixer');
 const list=$('appMixerList');
 const status=$('appMixerStatus'),groupList=$('appMixerGroups');
 const groupBodies=new Map();
 let groupLayout='',groupSaving=false,draggedPath='';
 const groups=()=>preferences?.AppMixerGroups||[];
 const samePath=(a,b)=>a.toLowerCase()===b.toLowerCase();
 const baseName=path=>path.split(/[\\/]/).pop();
 const groupFor=path=>groups().find(group=>group.Paths.some(member=>samePath(member,path)));
 const rows=new Map();
 let timer=null, visible=null, scanning=false, revision=0, writes=0, writeError='';

 function updateStatus(){
  const text=writeError || (rows.size?'':'No recent app audio yet.');
  if(status.textContent!==text)status.textContent=text;
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

 function saveGroups(update){
  if(groupSaving)return;
  groupSaving=true;
  return queueSettings(async()=>{
   const next=update(structuredClone(groups())).filter(group=>group.Paths.length);
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
   if(currentTab==='shortcuts')renderShortcuts();
  }).catch(error=>{writeError='Could not save mixer groups. '+errorText(error);syncGroups(true);})
   .finally(()=>{groupSaving=false;updateStatus();});
 }

 function moveApp(path,groupID,targetPath){
  if(targetPath&&samePath(path,targetPath))return;
  const prior=groupFor(path);
  if(groupID===prior?.ID||!groupID&&!prior)return;
  return saveGroups(next=>{
   const group=next.find(item=>item.ID===groupID);
   for(const item of next)item.Paths=item.Paths.filter(member=>!samePath(member,path));
   if(group)group.Paths.push(path);
   else if(groupID==='new'){
    next.push({ID:crypto.randomUUID(),Name:'Group '+(next.length+1),Paths:targetPath?[targetPath,path]:[path]});
   }
   return next;
  });
 }

 function dropTarget(element,drop){
  element.ondragover=e=>{
   if(!draggedPath)return;
   e.preventDefault();e.stopPropagation();e.dataTransfer.dropEffect='move';
   element.classList.toggle('is-drop-target',true);
  };
  element.ondragleave=e=>{if(!element.contains(e.relatedTarget))element.classList.toggle('is-drop-target',false);};
  element.ondrop=e=>{
   if(!draggedPath)return;
   e.preventDefault();e.stopPropagation();element.classList.toggle('is-drop-target',false);
   const path=draggedPath;draggedPath='';return drop(path);
  };
 }
 dropTarget(list,path=>moveApp(path,''));

 function placeRow(path,row){
  const group=groupFor(path),parent=groupBodies.get(group?.ID)||list;
  if(row.element.parentNode!==parent)parent.append(row.element);
  row.groupMenu.replaceChildren(new Option('Move to group…','choose'),new Option('Create new group','new'));
  row.groupMenu.options[0].disabled=true;
  if(group)row.groupMenu.add(new Option('Ungroup app',''));
  for(const destination of groups()){
   if(destination.ID!==group?.ID)row.groupMenu.add(new Option(destination.Name,destination.ID));
  }
  row.groupMenu.value='choose';
 }

 function syncGroups(force=false){
  const layout=JSON.stringify(groups());
  if(!force&&layout===groupLayout)return;
  groupLayout=layout;
  const focused=document.activeElement;
  for(const row of rows.values())if(row.element.parentNode!==list)list.append(row.element);
  groupBodies.clear();groupList.replaceChildren();
  for(const group of groups()){
   const section=document.createElement('section'),header=document.createElement('div'),body=document.createElement('div');
   section.className='app-mixer-group';header.className='app-mixer-group-header';
   const name=document.createElement('input');
   name.type='text';name.value=group.Name;name.maxLength=50;
   name.setAttribute('aria-label','Name of '+group.Name);
   name.onchange=()=>{
    const title=name.value.trim();
    if(!title){name.value=group.Name;return;}
    saveGroups(next=>{next.find(item=>item.ID===group.ID).Name=title;return next;});
   };
   const members=document.createElement('small');
   members.textContent=group.Paths.length+(group.Paths.length===1?' app':' apps');
   members.title=group.Paths.map(baseName).join(', ');
   const ungroup=document.createElement('button');
   ungroup.type='button';ungroup.className='text-button';ungroup.textContent='Ungroup';
   ungroup.setAttribute('aria-label','Ungroup all apps in '+group.Name);
   ungroup.onclick=()=>saveGroups(next=>next.filter(item=>item.ID!==group.ID));
   header.append(name,members,shortcutPicker(group.ID,group.Name,'group-'),ungroup);
   section.append(header,body);groupList.append(section);groupBodies.set(group.ID,body);
   dropTarget(section,path=>moveApp(path,group.ID));
  }
  for(const [path,row] of rows)placeRow(path,row);
  if(focused?.isConnected&&mixer.contains(focused)&&document.activeElement!==focused)focused.focus({preventScroll:true});
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
  identity.append(icon,name);
  const activity=document.createElement('span');
  activity.className='app-mixer-activity';
  const shortcut=shortcutPicker(app.Path,app.Name);
  const groupMenu=document.createElement('select');
  groupMenu.setAttribute('aria-label','Move '+app.Name+' to a mixer group');
  groupMenu.onchange=()=>{const destination=groupMenu.value;groupMenu.value='choose';return moveApp(app.Path,destination);};
  identity.draggable=true;identity.title='Drag onto another app to group';
  identity.ondragstart=e=>{
   draggedPath=app.Path;e.dataTransfer.effectAllowed='move';e.dataTransfer.setData('text/plain',app.Path);
   mixer.classList.toggle('is-grouping',true);
  };
  identity.ondragend=()=>{
   draggedPath='';mixer.classList.toggle('is-grouping',false);
   for(const target of mixer.querySelectorAll('.is-drop-target'))target.classList.toggle('is-drop-target',false);
  };
  dropTarget(element,path=>moveApp(path,groupFor(app.Path)?.ID||'new',app.Path));
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
  const row={element,input,showVolume,activity,groupMenu,lastPeak:0,saving:false,queued:null};
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
  rows.set(app.Path,row);placeRow(app.Path,row);
  return row;
 }

 async function refresh(){
  if(scanning||writes)return;
  scanning=true;
  const started=revision;
  try{
   const apps=await nativeListAppMixer([...rows.keys()]);
   if(started!==revision)return;
   syncGroups();
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
   updateStatus();
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
