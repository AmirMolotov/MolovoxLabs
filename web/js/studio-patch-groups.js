'use strict';
const patchSelectedNodes=new Set();
let patchGroupEditing=null;
function patchGroupByID(id){return patchGraph?.Groups?.find(g=>g.ID===id);}
function patchGroupForNode(id){return patchGraph?.Groups?.find(g=>g.NodeIDs.includes(id));}
function prunePatchGroups(){
 const valid=new Set(patchGraph.Nodes.map(n=>n.ID)),claimed=new Set();
 patchGraph.Groups=(patchGraph.Groups||[]).filter(g=>{g.NodeIDs=(g.NodeIDs||[]).filter(id=>valid.has(id)&&!claimed.has(id));g.NodeIDs.forEach(id=>claimed.add(id));return g.NodeIDs.length;});
 for(const id of patchSelectedNodes)if(!valid.has(id))patchSelectedNodes.delete(id);
}
function patchNodeHidden(id){return !!patchGroupForNode(id)?.Collapsed;}
function patchVisualNode(id){const group=patchGroupByID(id)||patchGroupForNode(id);if(group&&(group.ID===id||group.Collapsed))return {...group,Kind:'group'};return patchGraph.Nodes.find(n=>n.ID===id);}
function patchVisibleCables(){
 const edges=new Map();
 for(const edge of patchGraph.Cables){const a=patchVisualNode(edge.From),b=patchVisualNode(edge.To);if(!a||!b||a.ID===b.ID)continue;const key=a.ID+'>'+b.ID;if(!edges.has(key))edges.set(key,{a,b,edges:[]});edges.get(key).edges.push(edge);}
 return [...edges.values()];
}
function patchGroupBoundary(id,direction){
 const g=patchGroupByID(id);if(!g)return patchGraph.Nodes.some(n=>n.ID===id)?[id]:[];
 const members=new Set(g.NodeIDs);
 return patchGraph.Nodes.filter(n=>members.has(n.ID)&&(direction==='out'?n.Kind!=='output':['effect','output'].includes(n.Kind))).filter(n=>{
  const internal=patchGraph.Cables.some(c=>direction==='out'?c.From===n.ID&&members.has(c.To):c.To===n.ID&&members.has(c.From));
  const external=patchGraph.Cables.some(c=>direction==='out'?c.From===n.ID&&!members.has(c.To):c.To===n.ID&&!members.has(c.From));return !internal||external;
 }).map(n=>n.ID);
}
function patchGroupConnectionPairs(from,to){
 if(from===to)return [];
 const pairs=[];for(const a of patchGroupBoundary(from,'out'))for(const b of patchGroupBoundary(to,'in'))if(!patchGraph.Cables.some(c=>c.From===a&&c.To===b))pairs.push({From:a,To:b});return pairs;
}
function patchGroupConnectionAllowed(from,to){
 const pairs=patchGroupConnectionPairs(from,to);if(!pairs.length)return false;
 const original=patchGraph.Cables;patchGraph.Cables=[...original];
 try{for(const c of pairs){if(!patchRawCableAllowed(c.From,c.To))return false;patchGraph.Cables.push(c);}return true;}finally{patchGraph.Cables=original;}
}
function connectPatchGroups(from,to){
 if(!patchGroupConnectionAllowed(from,to)){patchPending=null;patchPointer=null;drawPatchCables();patchStatus('That group connection has no available ports, duplicates a cable or creates feedback.');return false;}
 patchGraph.Cables.push(...patchGroupConnectionPairs(from,to));patchGraph.Preset='';patchPending=null;patchPointer=null;commitPatch();return true;
}
function patchGroupVisualSave(){renderPatch();scheduleSave();}
function patchGroupCreate(name,ids=[...patchSelectedNodes]){
 ids=[...new Set(ids)].filter(id=>patchGraph.Nodes.some(n=>n.ID===id));if(!ids.length)return null;
 name=name.trim().slice(0,80);if(!name)return null;
 patchGraph.Preset='';const chosen=new Set(ids);
 for(const g of patchGraph.Groups||[])g.NodeIDs=g.NodeIDs.filter(id=>!chosen.has(id));
 const members=patchGraph.Nodes.filter(n=>chosen.has(n.ID));
 const group={ID:crypto.randomUUID(),Name:name,NodeIDs:ids,X:Math.min(...members.map(n=>n.X)),Y:Math.min(...members.map(n=>n.Y))-165,Collapsed:true};
 patchGraph.Groups=(patchGraph.Groups||[]).filter(g=>g.NodeIDs.length);patchGraph.Groups.push(group);patchSelectedNodes.clear();patchGroupVisualSave();return group;
}
function patchGroupUngroup(id){patchGraph.Groups=(patchGraph.Groups||[]).filter(g=>g.ID!==id);patchGroupVisualSave();}
function patchGroupToggle(id){const group=patchGroupByID(id);if(!group)return;group.Collapsed=!group.Collapsed;patchGroupVisualSave();}
function patchGroupToggleSelection(ids,checked){for(const id of ids)if(checked)patchSelectedNodes.add(id);else patchSelectedNodes.delete(id);renderPatch();}
function decoratePatchGroupSelection(node,header,card){
 const selected=patchSelectedNodes.has(node.ID),group=patchGroupForNode(node.ID);card.classList.toggle('patch-selected',selected);if(group){card.classList.add('patch-group-member');card.dataset.patchGroup=group.ID;card.title='Group: '+group.Name;}
 const box=document.createElement('input');box.type='checkbox';box.className='patch-node-select';box.checked=selected;box.setAttribute('aria-label','Select '+patchName(node)+' for grouping');box.onchange=()=>patchGroupToggleSelection([node.ID],box.checked);header.prepend(box);
}
function patchGroupGraphValid(nodes,cables){
 const index=new Map(nodes.map(n=>[n.ID,n])),edges=new Set(),next=new Map();
 for(const c of cables){const a=index.get(c.From),b=index.get(c.To),key=c.From+'>'+c.To;if(!a||!b||a.ID===b.ID||a.Kind==='output'||['mic','board','clip'].includes(b.Kind)||edges.has(key))return false;edges.add(key);if(!next.has(c.From))next.set(c.From,[]);next.get(c.From).push(c.To);}
 const visiting=new Set(),done=new Set();function visit(id){if(visiting.has(id))return false;if(done.has(id))return true;visiting.add(id);for(const to of next.get(id)||[])if(!visit(to))return false;visiting.delete(id);done.add(id);return true;}return nodes.every(n=>visit(n.ID));
}
function patchGroupAddEffect(id,kind){
 const g=patchGroupByID(id);if(!g||!filterDefaults[kind])return false;
 const originalNodes=[...patchGraph.Nodes],originalCables=[...patchGraph.Cables];
 const exits=patchGroupBoundary(id,'out');if(!exits.length){patchStatus('This group has no output to connect an effect.');return false;}
 const members=new Set(g.NodeIDs),outgoing=patchGraph.Cables.filter(c=>members.has(c.From)&&!members.has(c.To)),targets=[...new Set(outgoing.map(c=>c.To))];
 const filter={ID:crypto.randomUUID(),Kind:kind,Enabled:true,Hotkeys:[],MIDIs:[],...structuredClone(filterDefaults[kind])};
 const node=patchNode('effect',{FilterID:filter.ID},Math.max(...patchGraph.Nodes.filter(n=>members.has(n.ID)).map(n=>n.X))+270,g.Y+165);micFilters.push(filter);patchGraph.Nodes.push(node);
 patchGraph.Cables=patchGraph.Cables.filter(c=>!outgoing.includes(c));for(const from of exits)patchGraph.Cables.push({From:from,To:node.ID});for(const to of targets)patchGraph.Cables.push({From:node.ID,To:to});
 if(!targets.length){let output=patchGraph.Nodes.find(n=>n.Kind==='output'&&!n.DeviceID);if(!output){output=patchNode('output',{DeviceID:''},node.X+270,node.Y);patchGraph.Nodes.push(output);}patchGraph.Cables.push({From:node.ID,To:output.ID});}
 if(!patchGroupGraphValid(patchGraph.Nodes,patchGraph.Cables)){patchGraph.Nodes=originalNodes;patchGraph.Cables=originalCables;micFilters.pop();patchStatus('That shared effect would create feedback across this group. Group the complete chain or unplug the crossing cables first.');return false;}
 g.NodeIDs.push(node.ID);patchGraph.Preset='';patchGraph.Enabled=true;patchExpanded.add(node.ID);renderFilters();renderGroups();commitPatch();patchStatus(filterNames[kind]+' now processes the combined output of '+g.Name+'.');return true;
}
async function patchGroupSaveTemplate(id){
 const g=patchGroupByID(id);if(!g)return null;const members=new Set(g.NodeIDs),nodes=patchGraph.Nodes.filter(n=>members.has(n.ID)),filters=new Set(nodes.filter(n=>n.Kind==='effect').map(n=>n.FilterID));
 const template={ID:crypto.randomUUID(),Name:g.Name,Nodes:structuredClone(nodes),Cables:structuredClone(patchGraph.Cables.filter(c=>members.has(c.From)&&members.has(c.To))),Filters:structuredClone(micFilters.filter(f=>filters.has(f.ID)))};
 if(template.Filters.some(f=>f.Kind==='vst3')){try{template.Filters=await withLoading('Saving effect group…',()=>nativeSnapshotEffects(template.Filters),true);}catch(e){patchStatus('Could not save this group: '+errorText(e));return null;}}
 patchGraph.Templates??=[];patchGraph.Templates.push(template);patchGroupVisualSave();patchStatus('Saved '+g.Name+'. Insert it from Saved groups to create an independent copy.');return template;
}
function patchGroupInsertTemplate(id){
 const template=patchGraph.Templates?.find(t=>t.ID===id);if(!template?.Nodes?.length)return null;
 const available=new Set(library.Boards.flatMap(b=>b.Sounds).map(s=>s.ID));if(template.Nodes.some(n=>n.Kind==='clip'&&!available.has(n.ClipID))){patchStatus('A sound or video pad in this saved group has been removed.');return null;}
 const origin=placePatchNode(patchNode('effect')),minX=Math.min(...template.Nodes.map(n=>n.X)),minY=Math.min(...template.Nodes.map(n=>n.Y)),nodeMap=new Map(),filterMap=new Map();
 const filters=structuredClone(template.Filters||[]);for(const f of filters){const id=crypto.randomUUID();filterMap.set(f.ID,id);f.ID=id;f.Hotkeys=[];f.MIDIs=[];delete f.ClipID;}
 const nodes=structuredClone(template.Nodes);for(const n of nodes){const id=crypto.randomUUID();nodeMap.set(n.ID,id);n.ID=id;n.X=origin.X+n.X-minX;n.Y=origin.Y+165+n.Y-minY;if(n.Kind==='effect')n.FilterID=filterMap.get(n.FilterID);}
 if(nodes.some(n=>n.Kind==='effect'&&!n.FilterID)){patchStatus('This saved group is missing an effect definition.');return null;}
 const cables=template.Cables.map(c=>({From:nodeMap.get(c.From),To:nodeMap.get(c.To)}));if(cables.some(c=>!c.From||!c.To)){patchStatus('This saved group has an invalid internal cable.');return null;}
 if(!patchGroupGraphValid(nodes,cables)){patchStatus('This saved group contains invalid or circular cables.');return null;}
 const group={ID:crypto.randomUUID(),Name:template.Name,NodeIDs:nodes.map(n=>n.ID),X:origin.X,Y:origin.Y,Collapsed:true};
 patchGraph.Groups??=[];patchGraph.Groups.push(group);patchGraph.Nodes.push(...nodes);patchGraph.Cables.push(...cables);micFilters.push(...filters);patchGraph.Preset='';renderFilters();renderGroups();commitPatch();patchStatus('Inserted '+group.Name+'. Connect its ports to sources or outputs.');return group;
}
function updatePatchGroupFrames(){for(const g of patchGraph.Groups||[]){if(g.Collapsed)continue;const frame=document.querySelector('[data-patch-frame="'+g.ID+'"]');if(!frame)continue;const cards=g.NodeIDs.map(id=>document.querySelector('[data-patch-node="'+id+'"]')).filter(Boolean);if(!cards.length)continue;const left=Math.min(...cards.map(c=>c.offsetLeft))-14,top=Math.min(...cards.map(c=>c.offsetTop))-14;Object.assign(frame.style,{left:left+'px',top:top+'px',width:Math.max(...cards.map(c=>c.offsetLeft+c.offsetWidth))+14-left+'px',height:Math.max(...cards.map(c=>c.offsetTop+c.offsetHeight))+14-top+'px'});}}
function movePatchGroup(e){
 const d=patchDragging;if(!d?.group||e.pointerId!==d.pointerID)return;
 const dx=Math.round((e.clientX-d.x)/patchZoom),dy=Math.round((e.clientY-d.y)/patchZoom);d.node.X=d.startX+dx;d.node.Y=d.startY+dy;d.card.style.left=d.node.X+'px';d.card.style.top=d.node.Y+'px';
 for(const [id,p]of d.positions){const n=patchGraph.Nodes.find(n=>n.ID===id);if(!n)continue;n.X=p.X+dx;n.Y=p.Y+dy;const card=document.querySelector('[data-patch-node="'+id+'"]');if(card){card.style.left=n.X+'px';card.style.top=n.Y+'px';}}
 const frame=document.querySelector('[data-patch-frame="'+d.group+'"]');if(frame&&d.frame){frame.style.left=d.frame.X+dx+'px';frame.style.top=d.frame.Y+dy+'px';}drawPatchCables();
}
function cancelPatchGroupMove(){const d=patchDragging;if(!d)return;d.node.X=d.startX;d.node.Y=d.startY;for(const [id,p]of d.positions){const n=patchGraph.Nodes.find(n=>n.ID===id);if(n){n.X=p.X;n.Y=p.Y;}}patchDragging=null;patchPending=null;patchPointer=null;if(d.header?.hasPointerCapture?.(d.pointerID))d.header.releasePointerCapture(d.pointerID);renderPatch();patchStatus('Group move cancelled.');}
function patchGroupButton(text,cls,action){const button=document.createElement('button');button.className=cls;button.type='button';button.textContent=text;button.onclick=action;return button;}
function renderPatchGroups(){
 const root=$('patchNodes');
 for(const g of patchGraph.Groups||[]){
  const card=document.createElement('article');card.className='patch-node patch-group-card';card.dataset.patchNode=g.ID;card.style.left=g.X+'px';card.style.top=g.Y+'px';
  const header=document.createElement('div');header.className='patch-node-header';const selected=document.createElement('input');selected.type='checkbox';selected.className='patch-node-select';selected.checked=g.NodeIDs.every(id=>patchSelectedNodes.has(id));selected.setAttribute('aria-label','Select all nodes in '+g.Name);selected.onchange=()=>patchGroupToggleSelection(g.NodeIDs,selected.checked);
  const kind=document.createElement('span');kind.className='patch-node-kind';kind.textContent='GROUP · '+g.NodeIDs.length;const rename=patchGroupButton('✎','patch-group-rename',()=>openPatchGroupName(g.ID));rename.setAttribute('aria-label','Rename '+g.Name);header.append(selected,kind,rename);
  header.onpointerdown=e=>{if(e.button!==0||e.target.closest('button,input'))return;const frame=document.querySelector('[data-patch-frame="'+g.ID+'"]');patchDragging={group:g.ID,node:g,card,header,pointerID:e.pointerId,x:e.clientX,y:e.clientY,startX:g.X,startY:g.Y,positions:new Map(patchGraph.Nodes.filter(n=>g.NodeIDs.includes(n.ID)).map(n=>[n.ID,{X:n.X,Y:n.Y}])),frame:frame?{X:frame.offsetLeft,Y:frame.offsetTop}:null};header.setPointerCapture(e.pointerId);e.preventDefault();};
  const title=patchGroupButton((g.Collapsed?'▸ ':'▾ ')+g.Name,'patch-group-title',()=>patchGroupToggle(g.ID));title.setAttribute('aria-expanded',String(!g.Collapsed));
  const actions=document.createElement('div');actions.className='patch-group-actions';actions.append(patchGroupButton('Save group','text-button',()=>patchGroupSaveTemplate(g.ID)),patchGroupButton('Ungroup','text-button',()=>patchGroupUngroup(g.ID)));
  const effect=document.createElement('select');effect.className='clip-add-effect';effect.setAttribute('aria-label','Add shared effect to '+g.Name);effect.add(new Option('Add shared effect…',''));for(const [kind,name]of Object.entries(filterNames))if(filterDefaults[kind])effect.add(new Option(name,kind));effect.onchange=()=>{patchGroupAddEffect(g.ID,effect.value);};card.append(header,title,actions,effect);
  for(const direction of ['in','out']){if(!patchGroupBoundary(g.ID,direction).length)continue;const port=patchGroupButton('','patch-port '+direction,()=>{});port.dataset.node=g.ID;port.dataset.direction=direction;port.setAttribute('aria-label','Connect '+direction+' of '+g.Name);if(direction==='out'){port.onpointerdown=e=>{if(e.button!==0)return;e.preventDefault();patchPending=g.ID;patchPointer=null;drawPatchCables();};port.onclick=e=>{if(e.detail===0){patchPending=patchPending===g.ID?null:g.ID;drawPatchCables();}};}else port.onclick=e=>{if(e.detail===0&&patchPending)connectPatch(patchPending,g.ID);};card.append(port);}
  if(!g.Collapsed){const members=g.NodeIDs.map(id=>document.querySelector('[data-patch-node="'+id+'"]')).filter(Boolean);if(members.length){const frame=document.createElement('div');frame.className='patch-group-frame';frame.dataset.patchFrame=g.ID;const left=Math.min(...members.map(c=>c.offsetLeft))-14,top=Math.min(...members.map(c=>c.offsetTop))-14;frame.style.left=left+'px';frame.style.top=top+'px';frame.style.width=Math.max(...members.map(c=>c.offsetLeft+c.offsetWidth))+14-left+'px';frame.style.height=Math.max(...members.map(c=>c.offsetTop+c.offsetHeight))+14-top+'px';root.prepend(frame);}}
  root.append(card);patchResize?.observe(card);
 }
 const count=patchSelectedNodes.size;$('patchSelectionCount').textContent=count?count+' nodes selected':'Drag empty space to select · Shift-drag adds';$('patchGroupSelection').disabled=!count;$('patchClearSelection').hidden=!count;
 const list=$('patchTemplateList');list.replaceChildren();for(const template of patchGraph.Templates||[]){const row=document.createElement('div');row.className='patch-template-row';const label=document.createElement('strong');label.textContent=template.Name;const insert=patchGroupButton('Insert','secondary',()=>patchGroupInsertTemplate(template.ID));const remove=patchGroupButton('×','icon-close',()=>{patchGraph.Templates=patchGraph.Templates.filter(t=>t.ID!==template.ID);patchGroupVisualSave();});remove.setAttribute('aria-label','Delete saved group '+template.Name);row.append(label,insert,remove);list.append(row);}if(!list.children.length){const p=document.createElement('p');p.className='hint';p.textContent='Save a group from its card to reuse its nodes, settings and internal cables.';list.append(p);}
}
function openPatchGroupName(id=null){patchGroupEditing=id;const group=patchGroupByID(id);$('patchGroupName').value=group?.Name||'';$('patchGroupTitle').textContent=group?'Rename group':'Group selected nodes';$('patchGroupDescription').textContent=group?'Audio connections stay unchanged.':patchSelectedNodes.size+' nodes will collapse into one group. Connect its output or add shared effects.';$('savePatchGroupName').textContent=group?'Save name':'Create group';$('patchGroupDialog').showModal();$('patchGroupName').focus();}
$('patchGroupSelection').onclick=()=>openPatchGroupName();$('patchClearSelection').onclick=()=>{patchSelectedNodes.clear();renderPatch();};$('closePatchGroupDialog').onclick=()=>$('patchGroupDialog').close();
$('savePatchGroupName').onclick=()=>{const name=$('patchGroupName').value.trim();if(!name){$('patchGroupName').focus();return;}if(patchGroupEditing){const g=patchGroupByID(patchGroupEditing);if(g)g.Name=name;patchGroupVisualSave();}else patchGroupCreate(name);$('patchGroupDialog').close();};
$('patchGroupName').onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();$('savePatchGroupName').click();}};
