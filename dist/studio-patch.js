'use strict';
let patchGraph=null,patchPending=null,patchPointer=null,patchDragging=null;
let patchFocusView=null,patchFocusPrior=null;
let patchCameraX=0,patchCameraY=0;
let patchZoom=1,patchSpace=false,patchPan=null,patchSpaceUsed=false;
const patchExpanded=new Set();
const patchResize=typeof ResizeObserver==='function'?new ResizeObserver(sizePatchStage):null;
const patchWidth=210,patchPortY=74;
function patchNode(kind,extra={},x=40,y=60){return {ID:crypto.randomUUID(),Kind:kind,X:x,Y:y,...extra};}
function defaultPatch(mode='mic',enabled=false){
 const mic=patchNode('mic',{},40,60),board=patchNode('board',{BoardID:''},40,290),out=patchNode('output',{DeviceID:''},Math.max(680,330+micFilters.length*260),180);
 const nodes=[mic,board,out],cables=[];let last=mode==='board'?board:mic;
 micFilters.filter(f=>!f.ClipID).forEach((f,index)=>{const node=patchNode('effect',{FilterID:f.ID},310+index*260,mode==='board'?290:60);nodes.push(node);cables.push({From:last.ID,To:node.ID});last=node;});
 cables.push({From:last.ID,To:out.ID},{From:(mode==='board'?mic:board).ID,To:out.ID});
 const clips=new Set(library.Boards.flatMap(b=>b.Sounds).map(s=>s.ID));let row=0;
 for(const clipID of new Set(micFilters.filter(f=>f.ClipID&&clips.has(f.ClipID)).map(f=>f.ClipID))){const source=patchNode('clip',{ClipID:clipID},40,450+row*160);nodes.push(source);let cursor=source;micFilters.filter(f=>f.ClipID===clipID).forEach((filter,index)=>{const effect=patchNode('effect',{FilterID:filter.ID},310+index*260,source.Y);nodes.push(effect);cables.push({From:cursor.ID,To:effect.ID});cursor=effect;});cables.push({From:cursor.ID,To:out.ID});row++;}
 return {Version:1,Enabled:enabled,Preset:mode,Nodes:nodes,Cables:cables,Templates:patchGraph?.Templates||[]};
}
function inferPatchPreset(graph){
 const mic=graph.Nodes.filter(n=>n.Kind==='mic'),board=graph.Nodes.filter(n=>n.Kind==='board'&&!n.BoardID),out=graph.Nodes.filter(n=>n.Kind==='output'&&!n.DeviceID),effects=graph.Nodes.filter(n=>n.Kind==='effect');
 if(mic.length!==1||board.length!==1||out.length!==1||!effects.length||graph.Nodes.length!==effects.length+3||graph.Cables.length!==effects.length+2)return '';
 for(const mode of ['board','mic']){let current=(mode==='board'?board[0]:mic[0]).ID;const seen=new Set();for(let i=0;i<effects.length;i++){const next=graph.Cables.filter(c=>c.From===current);if(next.length!==1)break;const fx=effects.find(n=>n.ID===next[0].To);if(!fx||seen.has(fx.ID))break;seen.add(fx.ID);current=fx.ID;}
  const dry=(mode==='board'?mic[0]:board[0]).ID;
  if(seen.size===effects.length&&graph.Cables.some(c=>c.From===current&&c.To===out[0].ID)&&graph.Cables.some(c=>c.From===dry&&c.To===out[0].ID))return mode;
 }return '';
}
function syncPatchPreset(){
 if(!['mic','board'].includes(patchGraph.Preset))return;
 const desired=defaultPatch(patchGraph.Preset,patchGraph.Enabled),old=patchGraph.Nodes;
 const key=n=>n.Kind==='effect'?'effect:'+n.FilterID:n.Kind==='clip'?'clip:'+n.ClipID:n.Kind;
 const mapping=new Map();
 for(const node of desired.Nodes){const previous=old.find(n=>key(n)===key(node));mapping.set(node.ID,previous?.ID||node.ID);if(previous){node.ID=previous.ID;node.X=previous.X;node.Y=previous.Y;}}
 desired.Cables=desired.Cables.map(c=>({From:mapping.get(c.From),To:mapping.get(c.To)}));patchGraph=desired;
}
function initPatch(p){const saved=p.Options?.Patch;patchGraph=saved?.Nodes?.length?structuredClone(saved):defaultPatch();if(!patchGraph.Version){patchGraph.Preset=inferPatchPreset(patchGraph);patchGraph.Version=1;}renderPatch();}
function patchOptions(){
 if(!patchGraph)return {Enabled:false,Nodes:[],Cables:[]};
 syncPatchPreset();
 const clips=new Set(library.Boards.flatMap(b=>b.Sounds).map(s=>s.ID)),effects=new Set(micFilters.filter(f=>!f.ClipID||clips.has(f.ClipID)).map(f=>f.ID));patchGraph.Nodes=patchGraph.Nodes.filter(n=>(n.Kind!=='effect'||effects.has(n.FilterID))&&(n.Kind!=='clip'||clips.has(n.ClipID)));
 const nodes=new Set(patchGraph.Nodes.map(n=>n.ID));patchGraph.Cables=patchGraph.Cables.filter(c=>nodes.has(c.From)&&nodes.has(c.To));
 if(typeof prunePatchGroups==='function')prunePatchGroups();
 return structuredClone(patchGraph);
}
function patchName(node){
 if(node.Kind==='group')return node.Name;
 if(node.Kind==='mic')return 'Microphone mix';
 if(node.Kind==='clip')return library.Boards.flatMap(b=>b.Sounds).find(s=>s.ID===node.ClipID)?.Name||'Missing pad';
 if(node.Kind==='board')return node.BoardID?(library.Boards.find(b=>b.ID===node.BoardID)?.Name||'Missing board'):'All boards + video samplers';
 if(node.Kind==='effect'){const f=micFilters.find(f=>f.ID===node.FilterID);return f?filterTitle(f):'Missing effect';}
 return node.DeviceID?(devices?.Outputs?.find(d=>d.ID===node.DeviceID)?.Name||'Output device'):'Selected outputs';
}
function patchStatus(text){$('patchStatus').textContent=text||(!patchGraph?.Enabled?'Patch bay is off. Your standard microphone effects and dry soundboard routing are active.':patchGraph.Cables.length+' cables connected · changes apply automatically.');}
function placePatchNode(node){
 const viewport=$('patchViewport');
 if(viewport.clientWidth){
  node.X=Math.round((viewport.clientWidth/2-patchCameraX)/patchZoom-patchWidth/2);
  node.Y=Math.round((viewport.clientHeight/2-patchCameraY)/patchZoom-50);
 }
 return node;
}
function addPatchNode(node){node=placePatchNode(node);patchGraph.Preset='';patchGraph.Nodes.push(node);commitPatch();}
function addPatchEffectInstance(template,x,y){

 const filter={...template,ID:crypto.randomUUID(),Hotkeys:[],MIDIs:[]};
 if(filter.Kind==='vst3')filter.VSTState='';
 const node=patchNode('effect',{FilterID:filter.ID},x,y);
 micFilters.push(filter);patchGraph.Preset='';patchGraph.Nodes.push(node);patchExpanded.add(node.ID);
 renderFilters();renderGroups();commitPatch();
}
function openClipEffects(clipID){
 let node=patchGraph.Nodes.find(n=>n.Kind==='clip'&&n.ClipID===clipID);if(!node){node=patchNode('clip',{ClipID:clipID},40,80+patchGraph.Nodes.length*25);patchGraph.Preset='';patchGraph.Nodes.push(node);}
 patchGraph.Enabled=true;openTab('patch');commitPatch();patchStatus('Add an effect on this pad’s node, or wire this pad → effects → output. Create separate effect instances for independent chains.');document.querySelector('[data-patch-node="'+node.ID+'"]')?.scrollIntoView({block:'center',inline:'center',behavior:'smooth'});
}
function attachClipEffect(sourceID,kind){
 if(!filterDefaults[kind])return;
 const source=patchGraph.Nodes.find(n=>n.ID===sourceID&&n.Kind==='clip');if(!source)return;
 let output=patchGraph.Nodes.find(n=>n.Kind==='output'&&!n.DeviceID);
 const filter={ID:crypto.randomUUID(),ClipID:source.ClipID,Kind:kind,Enabled:true,Hotkeys:[],MIDIs:[],...filterDefaults[kind]};micFilters.push(filter);
 let cursor=source;const seen=new Set();while(!seen.has(cursor.ID)){seen.add(cursor.ID);const edges=patchGraph.Cables.filter(c=>c.From===cursor.ID);if(edges.length!==1)break;const next=patchGraph.Nodes.find(n=>n.ID===edges[0].To);if(next?.Kind!=='effect'||patchGraph.Cables.filter(c=>c.To===next.ID).length!==1)break;cursor=next;}
 const effect=patchNode('effect',{FilterID:filter.ID},cursor.X+260,cursor.Y);patchGraph.Nodes.push(effect);const targets=patchGraph.Cables.filter(c=>c.From===cursor.ID);patchGraph.Cables=patchGraph.Cables.filter(c=>c.From!==cursor.ID);patchGraph.Cables.push({From:cursor.ID,To:effect.ID});
 if(targets.length)for(const cable of targets)patchGraph.Cables.push({From:effect.ID,To:cable.To});else{if(!output){output=patchNode('output',{DeviceID:''},effect.X+270,effect.Y);patchGraph.Nodes.push(output);}patchGraph.Cables.push({From:effect.ID,To:output.ID});}
 patchGraph.Enabled=true;patchGraph.Preset='';renderFilters();renderGroups();commitPatch();patchStatus(filterNames[kind]+' added only to '+patchName(source)+'.');
}
async function commitPatch(){renderPatch();await configure();}
function cancelPatchInteraction(){
 if(patchDragging?.group){cancelPatchGroupMove();return;}
 const move=patchDragging,hadCable=!!patchPending;patchDragging=null;patchPending=null;patchPointer=null;
 if(move){const node=patchGraph.Nodes.find(n=>n.ID===move.node.ID);if(node){node.X=move.startX;node.Y=move.startY;}move.card.style.left=move.startX+'px';move.card.style.top=move.startY+'px';if(move.header?.hasPointerCapture(move.pointerID))move.header.releasePointerCapture(move.pointerID);renderPatch();}
 else drawPatchCables();
 if(move||hadCable)patchStatus(move?'Node move cancelled.':'Cable connection cancelled.');
}
function patchCableAllowed(from,to){
 if(typeof patchGroupByID==='function'&&(patchGroupByID(from)||patchGroupByID(to)))return patchGroupConnectionAllowed(from,to);
 return patchRawCableAllowed(from,to);
}
function patchRawCableAllowed(from,to){
 if(from===to)return false;const a=patchGraph.Nodes.find(n=>n.ID===from),b=patchGraph.Nodes.find(n=>n.ID===to);
 if(!a||!b||a.Kind==='output'||b.Kind==='mic'||b.Kind==='board'||b.Kind==='clip')return false;
 const visit=(id,seen=new Set())=>{if(id===from)return true;if(seen.has(id))return false;seen.add(id);return patchGraph.Cables.filter(c=>c.From===id).some(c=>visit(c.To,seen));};
 return !visit(to)&&!patchGraph.Cables.some(c=>c.From===from&&c.To===to);
}
function connectPatch(from,to){
 if(typeof patchGroupByID==='function'&&(patchGroupByID(from)||patchGroupByID(to))){connectPatchGroups(from,to);return;}
 if(!patchCableAllowed(from,to)){patchStatus('That cable would duplicate a connection or create a feedback loop.');patchPending=null;drawPatchCables();return;}
 patchGraph.Preset='';patchGraph.Cables.push({From:from,To:to});patchPending=null;patchPointer=null;commitPatch();
}
function drawPatchCables(){
 const svg=$('patchWires');svg.replaceChildren();if(!patchGraph)return;
 const path=(from,to,ghost=false,cable=null)=>{
  const element=document.createElementNS('http://www.w3.org/2000/svg','path'),bend=Math.max(75,Math.abs(to.x-from.x)*.45);
  element.setAttribute('d',`M ${from.x} ${from.y} C ${from.x+bend} ${from.y}, ${to.x-bend} ${to.y}, ${to.x} ${to.y}`);
  element.setAttribute('class','patch-cable'+(ghost?' ghost':''));
  if(cable){element.setAttribute('tabindex','0');element.setAttribute('role','button');element.setAttribute('aria-label','Unplug '+patchName(cable.a)+' from '+patchName(cable.b));const unplug=()=>{patchGraph.Preset='';patchGraph.Cables=patchGraph.Cables.filter(c=>!(cable.edges||[cable.edge]).includes(c));commitPatch();};element.onclick=unplug;element.onkeydown=e=>{if(['Enter',' ','Delete','Backspace'].includes(e.key)){e.preventDefault();unplug();}};}
  svg.append(element);
 };
 const visible=typeof patchVisibleCables==='function'?patchVisibleCables():patchGraph.Cables.map(edge=>({a:patchGraph.Nodes.find(n=>n.ID===edge.From),b:patchGraph.Nodes.find(n=>n.ID===edge.To),edges:[edge]}));
 for(const cable of visible){const {a,b}=cable;if(a&&b)path({x:a.X+patchWidth,y:a.Y+patchPortY},{x:b.X,y:b.Y+patchPortY},false,cable);}
 if(patchPending){const a=typeof patchVisualNode==='function'?patchVisualNode(patchPending):patchGraph.Nodes.find(n=>n.ID===patchPending);if(a)path({x:a.X+patchWidth,y:a.Y+patchPortY},patchPointer||{x:a.X+patchWidth+100,y:a.Y+patchPortY},true);}

}
function sizePatchStage(){
 const stage=$('patchStage');
 if(!stage.offsetWidth)return;
 const cards=[...$('patchNodes').children];
 stage.style.width=Math.max(1100,...cards.map(card=>card.offsetLeft+patchWidth+120))+'px';
 stage.style.height=Math.max(560,...cards.map(card=>card.offsetTop+card.offsetHeight+70))+'px';
}
function renderPatch(){
 if(!patchGraph||patchDragging)return;patchOptions();$('patchEnabled').checked=patchGraph.Enabled;
 for(const [id,mode] of [['patchBoardPreset','board'],['patchMicPreset','mic']]){$(id).classList.toggle('is-selected',patchGraph.Preset===mode);$(id).setAttribute('aria-pressed',String(patchGraph.Preset===mode));}
 const source=$('patchSource');source.replaceChildren(new Option('Add a source…',''));source.options[0].disabled=true;source.add(new Option('Microphone mix','mic'));source.add(new Option('All boards + video samplers','board:'));for(const kind of ['audio','video']){const group=document.createElement('optgroup');group.label=kind==='video'?'Video samplers':'Soundboards';for(const b of library.Boards.filter(b=>(b.Kind==='video')===(kind==='video')))group.append(new Option(b.Name,'board:'+b.ID));source.append(group);}for(const board of library.Boards){if(!board.Sounds.length)continue;const group=document.createElement('optgroup');group.label=(board.Kind==='video'?'Video pads · ':'Sound pads · ')+board.Name;for(const sound of board.Sounds)group.append(new Option(sound.Name,'clip:'+sound.ID));source.append(group);}source.value='';
 const effects=$('patchEffect');effects.replaceChildren(new Option('Add an effect…',''));effects.options[0].disabled=true;const used=new Set(patchGraph.Nodes.filter(n=>n.Kind==='effect').map(n=>n.FilterID));
 for(const [kind,name] of Object.entries(filterNames))effects.add(new Option(name,'new:'+kind));
 for(const f of micFilters)if(!used.has(f.ID))effects.add(new Option(filterTitle(f)+' (saved)',f.ID));
 effects.value='';
 const output=$('patchOutput');output.replaceChildren(new Option('Add an output…',''));output.options[0].disabled=true;output.add(new Option('All selected outputs','all'));for(const id of selections().Outputs){const d=devices?.Outputs?.find(d=>d.ID===id);output.add(new Option(d?.Name||'Selected output',id));}output.value='';
 const nodes=$('patchNodes');patchResize?.disconnect();nodes.replaceChildren();
 for(const id of patchExpanded)if(!patchGraph.Nodes.some(n=>n.ID===id))patchExpanded.delete(id);
 for(const node of patchGraph.Nodes){
  if(typeof patchNodeHidden==='function'&&patchNodeHidden(node.ID))continue;
  const card=document.createElement('article');const clip=library.Boards.flatMap(b=>b.Sounds).find(s=>s.ID===node.ClipID),isVideo=node.Kind==='board'&&library.Boards.some(b=>b.ID===node.BoardID&&b.Kind==='video');card.className='patch-node '+node.Kind+(isVideo?' video-source':'');card.style.left=node.X+'px';card.style.top=node.Y+'px';card.dataset.patchNode=node.ID;
  const header=document.createElement('div');header.className='patch-node-header';const kind=document.createElement('span');kind.className='patch-node-kind';kind.textContent=node.Kind==='clip'?(clip?.Video?'VIDEO PAD':'SOUND PAD'):isVideo?'VIDEO SAMPLER':{mic:'INPUT',board:'SOUNDBOARD',effect:'EFFECT',output:'OUTPUT'}[node.Kind];
  const remove=document.createElement('button');remove.className='icon-close patch-remove';remove.setAttribute('aria-label','Remove '+patchName(node)+' from patch bay');remove.onclick=()=>{patchGraph.Preset='';patchGraph.Nodes=patchGraph.Nodes.filter(n=>n.ID!==node.ID);patchGraph.Cables=patchGraph.Cables.filter(c=>c.From!==node.ID&&c.To!==node.ID);commitPatch();};header.append(kind,remove);if(typeof decoratePatchGroupSelection==='function')decoratePatchGroupSelection(node,header,card);
  header.onpointerdown=e=>{
   if(e.button!==0||e.target.closest('button,input,select'))return;
   const current=patchGraph.Nodes.find(n=>n.ID===node.ID);
   if(!current)return;
   patchDragging={node:current,card,header,pointerID:e.pointerId,x:e.clientX,y:e.clientY,startX:current.X,startY:current.Y};
   header.setPointerCapture(e.pointerId);e.preventDefault();
  };
  const title=document.createElement('strong');title.textContent=patchName(node);card.append(header,title);
  const note=document.createElement('small');note.textContent=node.Kind==='clip'?'Only this pad · separate effect chain':node.Kind==='mic'?($('monitorMic').checked?'Microphone → outputs is on':'Muted by the microphone switch'):node.Kind==='board'?(isVideo?'Video pad audio · keyboard / MIDI':'Clips + keyboard / MIDI triggers'):node.Kind==='output'?'Choose devices in Settings':'';if(note.textContent)card.append(note);
  if(node.Kind==='clip'){const select=document.createElement('select');select.className='clip-add-effect';select.setAttribute('aria-label','Add an effect to '+patchName(node));select.append(new Option('Add effect to this pad…',''));select.options[0].disabled=true;for(const [kind,name]of Object.entries(filterNames))if(filterDefaults[kind])select.add(new Option(name,kind));select.value='';select.onchange=()=>{const kind=select.value;select.value='';attachClipEffect(node.ID,kind);};card.append(select);}
  if(node.Kind==='effect'){
   const f=micFilters.find(f=>f.ID===node.FilterID),row=document.createElement('div');row.className='patch-node-actions';
   const enabled=document.createElement('button');enabled.className='patch-effect-toggle'+(f.Enabled?' on':'');enabled.textContent=f.Enabled?'On':'Bypassed';enabled.onclick=()=>{f.Enabled=!f.Enabled;renderFilters();configure();};
   const duplicate=document.createElement('button');duplicate.className='text-button';duplicate.textContent=f.Kind==='vst3'?'New instance':'Duplicate';duplicate.title=f.Kind==='vst3'?'Add another instance with default plugin settings':'Copy this effect’s settings into a new node';duplicate.onclick=()=>addPatchEffectInstance(f,node.X+260,node.Y);
   row.append(enabled,duplicate);card.append(row);
   const details=document.createElement('details');details.className='patch-effect-settings';details.open=patchExpanded.has(node.ID);
   const summary=document.createElement('summary');summary.textContent='Settings';details.append(summary);
   details.ontoggle=()=>{if(!details.isConnected)return;if(details.open)patchExpanded.add(node.ID);else patchExpanded.delete(node.ID);};
   const shortcut=document.createElement('button');shortcut.className='text-button';shortcut.textContent='Shortcuts & MIDI';shortcut.onclick=()=>openTriggerEditor('@filter:'+f.ID);
   details.append(renderEffectParameters(f,true),shortcut);card.append(details);
  }
  const port=(direction)=>{const button=document.createElement('button');button.className='patch-port '+direction;button.dataset.direction=direction;button.dataset.node=node.ID;button.setAttribute('aria-label',(direction==='out'?'Connect output of ':'Connect input of ')+patchName(node));
   if(direction==='out'){button.onpointerdown=e=>{if(e.button!==0)return;e.preventDefault();patchPending=node.ID;patchPointer=null;drawPatchCables();};button.onclick=e=>{if(e.detail===0){patchPending=patchPending===node.ID?null:node.ID;patchPointer=null;drawPatchCables();}};}
   else button.onclick=e=>{if(e.detail===0&&patchPending)connectPatch(patchPending,node.ID);};card.append(button);
  };
  if(node.Kind==='effect'||node.Kind==='output')port('in');if(node.Kind!=='output')port('out');nodes.append(card);patchResize?.observe(card);
 }
 if(typeof renderPatchGroups==='function')renderPatchGroups();
 sizePatchStage();drawPatchCables();patchStatus();
}
window.addEventListener('pointermove',e=>{
 if(patchPan&&patchPan.pointerID===e.pointerId){const p=patchPan;patchCameraX=p.left+e.clientX-p.x;patchCameraY=p.top+e.clientY-p.y;applyPatchCamera();return;}
 if(patchDragging?.group){movePatchGroup(e);return;}
 if(patchDragging){const d=patchDragging,node=patchGraph.Nodes.find(n=>n.ID===d.node.ID);if(!node)return;d.node=node;d.node.X=Math.round(d.startX+(e.clientX-d.x)/patchZoom);d.node.Y=Math.round(d.startY+(e.clientY-d.y)/patchZoom);d.card.style.left=d.node.X+'px';d.card.style.top=d.node.Y+'px';if(typeof updatePatchGroupFrames==='function')updatePatchGroupFrames();drawPatchCables();}
 else if(patchPending){const rect=$('patchStage').getBoundingClientRect();patchPointer={x:(e.clientX-rect.left)/patchZoom,y:(e.clientY-rect.top)/patchZoom};drawPatchCables();}
});
window.addEventListener('pointerup',e=>{
 if(e.button!==0)return;
 if(patchPan&&patchPan.pointerID===e.pointerId){endPatchPan();return;}
 if(patchDragging){patchDragging=null;sizePatchStage();scheduleSave();return;}
 if(patchPending){const port=document.elementFromPoint(e.clientX,e.clientY)?.closest('.patch-port.in');if(port)connectPatch(patchPending,port.dataset.node);}
});
$('patchEnabled').onchange=()=>{patchGraph.Enabled=$('patchEnabled').checked;commitPatch();};
$('patchSource').onchange=()=>{const choice=$('patchSource').value;if(!choice)return;if(choice.startsWith('clip:')){addPatchNode(patchNode('clip',{ClipID:choice.slice(5)},40,80+patchGraph.Nodes.length*30));return;}addPatchNode(patchNode(choice==='mic'?'mic':'board',choice==='mic'?{}:{BoardID:choice.slice(6)},40,80+patchGraph.Nodes.length*30));};
$('patchEffect').onchange=()=>{
 const id=$('patchEffect').value;if(!id)return;$('patchEffect').value='';
 const position=placePatchNode(patchNode('effect',{},350,120+patchGraph.Nodes.length*25)),x=position.X,y=position.Y;
 if(id.startsWith('new:')){const Kind=id.slice(4);addPatchEffectInstance({Kind,Enabled:true,...filterDefaults[Kind]},x,y);}
 else addPatchNode(patchNode('effect',{FilterID:id},x,y));
};
$('patchOutput').onchange=()=>{const id=$('patchOutput').value;if(!id)return;addPatchNode(patchNode('output',{DeviceID:id==='all'?'':id},900,120+patchGraph.Nodes.length*25));};
$('patchBoardPreset').onclick=()=>{patchGraph=defaultPatch('board',true);commitPatch();};
$('patchMicPreset').onclick=()=>{patchGraph=defaultPatch('mic',true);commitPatch();};
$('patchClear').onclick=()=>{patchGraph.Preset='';patchGraph.Cables=[];patchPending=null;commitPatch();};
$('patchDefine').onclick=()=>$('patchEffectsLibrary').scrollIntoView({behavior:'smooth',block:'start'});
function applyPatchCamera(){
 const stage=$('patchStage');
 stage.style.transform='translate('+patchCameraX+'px, '+patchCameraY+'px) scale('+patchZoom+')';
 $('patchZoomLabel').textContent=Math.round(patchZoom*100)+'%';
}
function patchWheel(event){
 if(!event.deltaY||event.target?.closest?.('.patch-vst-params'))return;
 event.preventDefault();if(patchDragging||patchPan)return;
 const viewport=$('patchViewport'),stage=$('patchStage');
 if(event.ctrlKey){patchCameraX-=event.deltaY*2;}
 else{
  const rect=viewport.getBoundingClientRect(),x=event.clientX-rect.left-(viewport.clientLeft||0),y=event.clientY-rect.top-(viewport.clientTop||0);
  const next=Math.max(.03,Math.min(3,patchZoom*Math.exp(-Math.max(-300,Math.min(300,event.deltaY))*.003)));
  const ratio=next/patchZoom;
  patchCameraX=x-(x-patchCameraX)*ratio;patchCameraY=y-(y-patchCameraY)*ratio;patchZoom=next;
 }
 applyPatchCamera();
 if(patchPending){const rect=stage.getBoundingClientRect();patchPointer={x:(event.clientX-rect.left)/patchZoom,y:(event.clientY-rect.top)/patchZoom};drawPatchCables();}
}
$('patchViewport').addEventListener?.('wheel',patchWheel,{passive:false});
$('patchStage').oncontextmenu=e=>{e.preventDefault();e.stopPropagation();cancelPatchInteraction();};
document.addEventListener('keydown',e=>{
 if(e.key!=='Escape'||document.querySelector('dialog[open]'))return;
 if(patchPending||patchDragging||patchPan){e.preventDefault();e.stopImmediatePropagation();cancelPatchInteraction();endPatchPan();}
 else if(patchFocusView){e.preventDefault();e.stopImmediatePropagation();togglePatchFocus(false);}
},true);

function setPatchSpace(held){
 patchSpace=held;$('patchViewport').classList.toggle('pan-ready',held);
}
function endPatchPan(){
 if(!patchPan)return;
 const {viewport,pointerID}=patchPan;patchPan=null;viewport.classList.remove('panning');
 if(viewport.hasPointerCapture(pointerID))viewport.releasePointerCapture(pointerID);
}
document.addEventListener('keydown',e=>{
 if(e.code!=='Space'||e.ctrlKey||e.altKey||e.shiftKey||e.metaKey||currentTab!=='patch'||document.querySelector('dialog[open]')||e.target.closest?.('input,textarea,select,[contenteditable]'))return;
 if(!patchSpace)patchSpaceUsed=false;
 if(patchSpaceUsed||!e.target.closest?.('button,summary,[role=button]'))e.preventDefault();
 setPatchSpace(true);
},true);
document.addEventListener('keyup',e=>{if(e.code==='Space'&&patchSpace){if(patchSpaceUsed)e.preventDefault();setPatchSpace(false);}},true);
window.addEventListener('blur',()=>{if(patchSpace)setPatchSpace(false);endPatchPan();});
$('patchViewport').addEventListener?.('pointerdown',e=>{
 const background=e.target===$('patchViewport')||e.target===$('patchStage')||e.target===$('patchWires');
 if((!patchSpace&&!background)||e.button!==0||patchDragging||patchPan)return;
 e.preventDefault();e.stopPropagation();patchSpaceUsed=true;
 if(patchPending)cancelPatchInteraction();
 const viewport=e.currentTarget;
 patchPan={viewport,pointerID:e.pointerId,x:e.clientX,y:e.clientY,left:patchCameraX,top:patchCameraY};
 viewport.classList.add('panning');viewport.setPointerCapture(e.pointerId);
},true);
$('patchViewport').addEventListener?.('lostpointercapture',e=>{if(patchPan&&patchPan.pointerID===e.pointerId)endPatchPan();});

function patchViewBounds(){
 const cards=[...$('patchNodes').children];
 return {
  left:Math.min(...cards.map(card=>card.offsetLeft))-28,
  top:Math.min(...cards.map(card=>card.offsetTop))-28,
  right:Math.max(...cards.map(card=>card.offsetLeft+card.offsetWidth+28)),
  bottom:Math.max(...cards.map(card=>card.offsetTop+card.offsetHeight+28)),
 };
}
function fitPatchView(){
 if(!patchGraph?.Nodes.length)return;
 const viewport=$('patchViewport'),bounds=patchViewBounds();
 if(!viewport.clientWidth||!viewport.clientHeight)return;
 patchZoom=Math.max(.03,Math.min(1.2,(viewport.clientWidth-24)/(bounds.right-bounds.left),(viewport.clientHeight-24)/(bounds.bottom-bounds.top)));
 patchCameraX=viewport.clientWidth/2-(bounds.left+bounds.right)/2*patchZoom;
 patchCameraY=viewport.clientHeight/2-(bounds.top+bounds.bottom)/2*patchZoom;
 applyPatchCamera();
}
function togglePatchFocus(enabled=!patchFocusView){
 if(enabled===!!patchFocusView)return;
 const viewport=$('patchViewport'),workspace=$('patchWorkspace'),button=$('patchFocus');
 cancelPatchInteraction();endPatchPan();setPatchSpace(false);
 if(enabled){
  patchFocusView={zoom:patchZoom,left:patchCameraX,top:patchCameraY};
  patchFocusPrior=document.activeElement;
  // Move the workspace out of any parent stacking contexts.
  const marker=document.createComment('patch workspace');
  workspace.before(marker);patchFocusView.marker=marker;
  const signals=document.querySelector('.signal-dock');
  if(signals){
   const signalMarker=document.createComment('shared signals');
   signals.before(signalMarker);
   workspace.querySelector('.patch-workspace-bar').after(signals);
   patchFocusView.signals=signals;patchFocusView.signalMarker=signalMarker;
  }
  document.body.append(workspace);
  document.body.classList.add('patch-focused');
  patchFocusView.background=[...document.body.children].filter(el=>el!==workspace&&el.tagName!=='SCRIPT'&&el.tagName!=='DIALOG'&&el.id!=='appLoader').map(el=>({el,inert:el.inert}));
  for(const item of patchFocusView.background)item.el.inert=true;
  button.textContent='Exit full screen';button.setAttribute('aria-pressed','true');button.title='Exit full screen patch bay';button.setAttribute('aria-label',button.title);button.focus();
  requestAnimationFrame(()=>{if(patchFocusView)fitPatchView();});
 }else{
  const previous=patchFocusView;patchFocusView=null;
  document.body.classList.remove('patch-focused');
  if(previous.signals)previous.signalMarker.replaceWith(previous.signals);
  previous.marker.replaceWith(workspace);
  for(const item of previous.background)item.el.inert=item.inert;
  patchZoom=previous.zoom;patchCameraX=previous.left;patchCameraY=previous.top;applyPatchCamera();
  button.textContent='Full screen';button.setAttribute('aria-pressed','false');button.title='Full screen patch bay';button.setAttribute('aria-label',button.title);
  if(patchFocusPrior?.isConnected)patchFocusPrior.focus();
 }
}
$('patchFocus').onclick=()=>togglePatchFocus();
$('patchFit').onclick=fitPatchView;
for(const [id,deltaY] of [['patchZoomIn',-70],['patchZoomOut',70]]){
 $(id).onclick=()=>{
  const viewport=$('patchViewport'),rect=viewport.getBoundingClientRect();
  patchWheel({deltaY,clientX:rect.left+viewport.clientWidth/2,clientY:rect.top+viewport.clientHeight/2,preventDefault(){}});
 };
}

$('patchOpenWorkspace').onclick=()=>togglePatchFocus(true);
