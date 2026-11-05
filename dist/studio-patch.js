'use strict';
let patchGraph=null,patchPending=null,patchPointer=null,patchDragging=null;
const patchWidth=210,patchPortY=74;
function patchNode(kind,extra={},x=40,y=60){return {ID:crypto.randomUUID(),Kind:kind,X:x,Y:y,...extra};}
function defaultPatch(mode='mic',enabled=false){
 const mic=patchNode('mic',{},40,60),board=patchNode('board',{BoardID:''},40,290),out=patchNode('output',{DeviceID:''},Math.max(680,330+micFilters.length*260),180);
 const nodes=[mic,board,out],cables=[];let last=mode==='board'?board:mic;
 micFilters.forEach((f,index)=>{const node=patchNode('effect',{FilterID:f.ID},310+index*260,mode==='board'?290:60);nodes.push(node);cables.push({From:last.ID,To:node.ID});last=node;});
 cables.push({From:last.ID,To:out.ID},{From:(mode==='board'?mic:board).ID,To:out.ID});
 return {Version:1,Enabled:enabled,Preset:mode,Nodes:nodes,Cables:cables};
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
 const key=n=>n.Kind==='effect'?'effect:'+n.FilterID:n.Kind;
 const mapping=new Map();
 for(const node of desired.Nodes){const previous=old.find(n=>key(n)===key(node));mapping.set(node.ID,previous?.ID||node.ID);if(previous){node.ID=previous.ID;node.X=previous.X;node.Y=previous.Y;}}
 desired.Cables=desired.Cables.map(c=>({From:mapping.get(c.From),To:mapping.get(c.To)}));patchGraph=desired;
}
function initPatch(p){const saved=p.Options?.Patch;patchGraph=saved?.Nodes?.length?structuredClone(saved):defaultPatch();if(!patchGraph.Version){patchGraph.Preset=inferPatchPreset(patchGraph);patchGraph.Version=1;}renderPatch();}
function patchOptions(){
 if(!patchGraph)return {Enabled:false,Nodes:[],Cables:[]};
 syncPatchPreset();
 const effects=new Set(micFilters.map(f=>f.ID));patchGraph.Nodes=patchGraph.Nodes.filter(n=>n.Kind!=='effect'||effects.has(n.FilterID));
 const nodes=new Set(patchGraph.Nodes.map(n=>n.ID));patchGraph.Cables=patchGraph.Cables.filter(c=>nodes.has(c.From)&&nodes.has(c.To));
 return structuredClone(patchGraph);
}
function patchName(node){
 if(node.Kind==='mic')return 'Microphone mix';
 if(node.Kind==='board')return node.BoardID?(library.Boards.find(b=>b.ID===node.BoardID)?.Name||'Missing soundboard'):'All soundboards';
 if(node.Kind==='effect'){const f=micFilters.find(f=>f.ID===node.FilterID);return f?filterTitle(f):'Missing effect';}
 return node.DeviceID?(devices?.Outputs?.find(d=>d.ID===node.DeviceID)?.Name||'Output device'):'Selected outputs';
}
function patchStatus(text){$('patchStatus').textContent=text||(!patchGraph?.Enabled?'Patch bay is off. Your standard microphone effects and dry soundboard routing are active.':patchGraph.Cables.length+' cables connected · changes apply automatically.');}
function addPatchNode(node){if(patchGraph.Nodes.length>=32){patchStatus('Maximum 32 patch nodes. Remove an unused node first.');return;}patchGraph.Preset='';patchGraph.Nodes.push(node);commitPatch();}
async function commitPatch(){renderPatch();await configure();}
function cancelPatchInteraction(){
 const move=patchDragging,hadCable=!!patchPending;patchDragging=null;patchPending=null;patchPointer=null;
 if(move){move.node.X=move.startX;move.node.Y=move.startY;move.card.style.left=move.startX+'px';move.card.style.top=move.startY+'px';if(move.header?.hasPointerCapture(move.pointerID))move.header.releasePointerCapture(move.pointerID);renderPatch();}
 else drawPatchCables();
 if(move||hadCable)patchStatus(move?'Node move cancelled.':'Cable connection cancelled.');
}
function patchCableAllowed(from,to){
 if(from===to)return false;const a=patchGraph.Nodes.find(n=>n.ID===from),b=patchGraph.Nodes.find(n=>n.ID===to);
 if(!a||!b||a.Kind==='output'||b.Kind==='mic'||b.Kind==='board')return false;
 const visit=(id,seen=new Set())=>{if(id===from)return true;if(seen.has(id))return false;seen.add(id);return patchGraph.Cables.filter(c=>c.From===id).some(c=>visit(c.To,seen));};
 return !visit(to)&&!patchGraph.Cables.some(c=>c.From===from&&c.To===to);
}
function connectPatch(from,to){
 if(!patchCableAllowed(from,to)){patchStatus('That cable would duplicate a connection or create a feedback loop.');patchPending=null;drawPatchCables();return;}
 patchGraph.Preset='';patchGraph.Cables.push({From:from,To:to});patchPending=null;patchPointer=null;commitPatch();
}
function drawPatchCables(){
 const svg=$('patchWires');svg.replaceChildren();if(!patchGraph)return;
 const path=(from,to,ghost=false,cable=null)=>{
  const element=document.createElementNS('http://www.w3.org/2000/svg','path'),bend=Math.max(75,Math.abs(to.x-from.x)*.45);
  element.setAttribute('d',`M ${from.x} ${from.y} C ${from.x+bend} ${from.y}, ${to.x-bend} ${to.y}, ${to.x} ${to.y}`);
  element.setAttribute('class','patch-cable'+(ghost?' ghost':''));
  if(cable){element.setAttribute('tabindex','0');element.setAttribute('role','button');element.setAttribute('aria-label','Unplug '+patchName(cable.a)+' from '+patchName(cable.b));const unplug=()=>{patchGraph.Preset='';patchGraph.Cables=patchGraph.Cables.filter(c=>c!==cable.edge);commitPatch();};element.onclick=unplug;element.onkeydown=e=>{if(['Enter',' ','Delete','Backspace'].includes(e.key)){e.preventDefault();unplug();}};}
  svg.append(element);
 };
 for(const edge of patchGraph.Cables){const a=patchGraph.Nodes.find(n=>n.ID===edge.From),b=patchGraph.Nodes.find(n=>n.ID===edge.To);if(a&&b)path({x:a.X+patchWidth,y:a.Y+patchPortY},{x:b.X,y:b.Y+patchPortY},false,{a,b,edge});}
 if(patchPending){const a=patchGraph.Nodes.find(n=>n.ID===patchPending);if(a)path({x:a.X+patchWidth,y:a.Y+patchPortY},patchPointer||{x:a.X+patchWidth+100,y:a.Y+patchPortY},true);}
}
function renderPatch(){
 if(!patchGraph)return;patchOptions();$('patchEnabled').checked=patchGraph.Enabled;
 for(const [id,mode] of [['patchBoardPreset','board'],['patchMicPreset','mic']]){$(id).classList.toggle('is-selected',patchGraph.Preset===mode);$(id).setAttribute('aria-pressed',String(patchGraph.Preset===mode));}
 const source=$('patchSource');source.replaceChildren(new Option('Add a source…',''));source.options[0].disabled=true;source.add(new Option('Microphone mix','mic'));source.add(new Option('All soundboards','board:'));for(const b of library.Boards)source.add(new Option(b.Name,'board:'+b.ID));source.value='';
 const effects=$('patchEffect');effects.replaceChildren(new Option('Add an effect…',''));effects.options[0].disabled=true;const used=new Set(patchGraph.Nodes.filter(n=>n.Kind==='effect').map(n=>n.FilterID));for(const f of micFilters)if(!used.has(f.ID))effects.add(new Option(filterTitle(f),f.ID));effects.value='';effects.disabled=effects.options.length===1;
 const output=$('patchOutput');output.replaceChildren(new Option('Add an output…',''));output.options[0].disabled=true;output.add(new Option('All selected outputs','all'));for(const id of selections().Outputs){const d=devices?.Outputs?.find(d=>d.ID===id);output.add(new Option(d?.Name||'Selected output',id));}output.value='';
 const stage=$('patchStage'),nodes=$('patchNodes');nodes.replaceChildren();
 stage.style.width=Math.max(1100,...patchGraph.Nodes.map(n=>n.X+patchWidth+120))+'px';stage.style.height=Math.max(560,...patchGraph.Nodes.map(n=>n.Y+190))+'px';
 for(const node of patchGraph.Nodes){
  const card=document.createElement('article');card.className='patch-node '+node.Kind;card.style.left=node.X+'px';card.style.top=node.Y+'px';card.dataset.patchNode=node.ID;
  const header=document.createElement('div');header.className='patch-node-header';const kind=document.createElement('span');kind.className='patch-node-kind';kind.textContent={mic:'INPUT',board:'SOUNDBOARD',effect:'EFFECT',output:'OUTPUT'}[node.Kind];
  const remove=document.createElement('button');remove.className='icon-close patch-remove';remove.setAttribute('aria-label','Remove '+patchName(node)+' from patch bay');remove.onclick=()=>{patchGraph.Preset='';patchGraph.Nodes=patchGraph.Nodes.filter(n=>n.ID!==node.ID);patchGraph.Cables=patchGraph.Cables.filter(c=>c.From!==node.ID&&c.To!==node.ID);commitPatch();};header.append(kind,remove);
  header.onpointerdown=e=>{if(e.button!==0||e.target.closest('button'))return;patchDragging={node,card,header,pointerID:e.pointerId,x:e.clientX,y:e.clientY,startX:node.X,startY:node.Y};header.setPointerCapture(e.pointerId);e.preventDefault();};
  const title=document.createElement('strong');title.textContent=patchName(node);card.append(header,title);
  const note=document.createElement('small');note.textContent=node.Kind==='mic'?($('monitorMic').checked?'Microphone → outputs is on':'Muted by the microphone switch'):node.Kind==='board'?'Clips + keyboard / MIDI triggers':node.Kind==='output'?'Choose devices in Settings':'Defined in your Effects library';card.append(note);
  if(node.Kind==='effect'){
   const f=micFilters.find(f=>f.ID===node.FilterID),row=document.createElement('div');row.className='patch-node-actions';
   const enabled=document.createElement('button');enabled.className='patch-effect-toggle'+(f?.Enabled?' on':'');enabled.textContent=f?.Enabled?'On':'Bypassed';enabled.onclick=()=>{f.Enabled=!f.Enabled;renderFilters();commitPatch();};
   const edit=document.createElement('button');edit.className='text-button';edit.textContent='Edit effect';edit.onclick=()=>{openTab('filters');document.querySelector(`[data-filter-id="${node.FilterID}"]`)?.scrollIntoView({behavior:'smooth',block:'center'});};row.append(enabled,edit);card.append(row);
  }
  const port=(direction)=>{const button=document.createElement('button');button.className='patch-port '+direction;button.dataset.direction=direction;button.dataset.node=node.ID;button.setAttribute('aria-label',(direction==='out'?'Connect output of ':'Connect input of ')+patchName(node));
   if(direction==='out'){button.onpointerdown=e=>{if(e.button!==0)return;e.preventDefault();patchPending=node.ID;patchPointer=null;drawPatchCables();};button.onclick=e=>{if(e.detail===0){patchPending=patchPending===node.ID?null:node.ID;patchPointer=null;drawPatchCables();}};}
   else button.onclick=e=>{if(e.detail===0&&patchPending)connectPatch(patchPending,node.ID);};card.append(button);
  };
  if(node.Kind==='effect'||node.Kind==='output')port('in');if(node.Kind!=='output')port('out');nodes.append(card);
 }
 drawPatchCables();patchStatus();
}
window.addEventListener('pointermove',e=>{
 if(patchDragging){const d=patchDragging;d.node.X=Math.round(Math.max(16,Math.min(5000,d.startX+e.clientX-d.x)));d.node.Y=Math.round(Math.max(16,Math.min(5000,d.startY+e.clientY-d.y)));d.card.style.left=d.node.X+'px';d.card.style.top=d.node.Y+'px';drawPatchCables();}
 else if(patchPending){const rect=$('patchStage').getBoundingClientRect();patchPointer={x:e.clientX-rect.left,y:e.clientY-rect.top};drawPatchCables();}
});
window.addEventListener('pointerup',e=>{
 if(e.button!==0)return;
 if(patchDragging){patchDragging=null;renderPatch();scheduleSave();return;}
 if(patchPending){const port=document.elementFromPoint(e.clientX,e.clientY)?.closest('.patch-port.in');if(port)connectPatch(patchPending,port.dataset.node);}
});
$('patchEnabled').onchange=()=>{patchGraph.Enabled=$('patchEnabled').checked;commitPatch();};
$('patchSource').onchange=()=>{const choice=$('patchSource').value;if(!choice)return;addPatchNode(patchNode(choice==='mic'?'mic':'board',choice==='mic'?{}:{BoardID:choice.slice(6)},40,80+patchGraph.Nodes.length*30));};
$('patchEffect').onchange=()=>{const id=$('patchEffect').value;if(!id)return;addPatchNode(patchNode('effect',{FilterID:id},350,120+patchGraph.Nodes.length*25));};
$('patchOutput').onchange=()=>{const id=$('patchOutput').value;if(!id)return;addPatchNode(patchNode('output',{DeviceID:id==='all'?'':id},900,120+patchGraph.Nodes.length*25));};
$('patchBoardPreset').onclick=()=>{patchGraph=defaultPatch('board',true);commitPatch();};
$('patchMicPreset').onclick=()=>{patchGraph=defaultPatch('mic',true);commitPatch();};
$('patchClear').onclick=()=>{patchGraph.Preset='';patchGraph.Cables=[];patchPending=null;commitPatch();};
$('patchDefine').onclick=()=>openTab('filters');
$('patchStage').oncontextmenu=e=>{e.preventDefault();e.stopPropagation();cancelPatchInteraction();};
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&(patchPending||patchDragging)){e.preventDefault();e.stopImmediatePropagation();cancelPatchInteraction();}},true);
