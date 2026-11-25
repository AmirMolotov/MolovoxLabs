'use strict';
let patchMarquee=null;
function refreshPatchSelectionUI(){
 for(const card of $('patchNodes').querySelectorAll('.patch-node')){
  const id=card.dataset.patchNode,group=patchGroupByID(id),ids=group?group.NodeIDs:[id],any=ids.some(id=>patchSelectedNodes.has(id)),all=ids.every(id=>patchSelectedNodes.has(id));
  card.classList.toggle('patch-selected',any);const check=card.querySelector('.patch-node-select');if(check){check.checked=all;check.indeterminate=any&&!all;}
 }
 const count=patchSelectedNodes.size;$('patchSelectionCount').textContent=count?count+' nodes selected':'Drag empty space to select · Shift-drag adds';$('patchGroupSelection').disabled=!count;$('patchClearSelection').hidden=!count;
}
function replacePatchSelection(ids){patchSelectedNodes.clear();for(const id of ids)patchSelectedNodes.add(id);refreshPatchSelectionUI();}
function patchSelectionIntersects(a,b){return a.left<=b.right&&a.right>=b.left&&a.top<=b.bottom&&a.bottom>=b.top;}
let patchEffectSource=null,patchMenuPoint=null,patchMenuFocus=null;
function showPatchEffectChoices(open){
 $('patchContextChoices').hidden=!open;
 $('patchContextEffect').setAttribute('aria-expanded',String(open));
}
function closePatchSelectionMenu(restoreFocus=false){
 $('patchSelectionMenu').hidden=true;patchEffectSource=null;showPatchEffectChoices(false);
 if(restoreFocus)patchMenuFocus?.focus({preventScroll:true});
 patchMenuFocus=null;
}
function positionPatchSelectionMenu(){
 const menu=$('patchSelectionMenu'),{x,y}=patchMenuPoint;
 menu.style.left=Math.max(12,Math.min(x,innerWidth-(menu.offsetWidth||210)-12))+'px';
 menu.style.top=Math.max(12,Math.min(y,innerHeight-(menu.offsetHeight||48)-12))+'px';
}
function beginPatchSelection(e){
 closePatchSelectionMenu();if(patchMarquee||patchSpace||e.button!==0)return;
 const viewport=$('patchViewport'),bounds=viewport.getBoundingClientRect(),initial=[...patchSelectedNodes],base=e.shiftKey||e.ctrlKey||e.metaKey?initial:[];
 const cards=[...$('patchNodes').querySelectorAll('.patch-node')].map(card=>({id:card.dataset.patchNode,rect:card.getBoundingClientRect()}));
 const box=document.createElement('div');box.className='patch-marquee';box.hidden=true;box.setAttribute('aria-hidden','true');viewport.append(box);
 patchMarquee={viewport,bounds,pointerID:e.pointerId,x:e.clientX,y:e.clientY,initial,base,cards,box,moved:false};replacePatchSelection(base);viewport.tabIndex=-1;viewport.focus({preventScroll:true});viewport.setPointerCapture(e.pointerId);
}
function movePatchSelection(e){
 const d=patchMarquee;if(!d||e.pointerId!==d.pointerID)return;e.preventDefault();
 const x=Math.max(d.bounds.left,Math.min(d.bounds.right,e.clientX)),y=Math.max(d.bounds.top,Math.min(d.bounds.bottom,e.clientY));
 if(!d.moved&&Math.hypot(x-d.x,y-d.y)<4)return;d.moved=true;
 const rect={left:Math.min(d.x,x),top:Math.min(d.y,y),right:Math.max(d.x,x),bottom:Math.max(d.y,y)};d.box.hidden=false;
 Object.assign(d.box.style,{left:rect.left-d.bounds.left-(d.viewport.clientLeft||0)+'px',top:rect.top-d.bounds.top-(d.viewport.clientTop||0)+'px',width:rect.right-rect.left+'px',height:rect.bottom-rect.top+'px'});
 const ids=new Set(d.base);for(const card of d.cards)if(patchSelectionIntersects(rect,card.rect)){const group=patchGroupByID(card.id);for(const id of group?.NodeIDs||[card.id])ids.add(id);}
 replacePatchSelection(ids);
}
function finishPatchSelection(cancelled=false){
 const d=patchMarquee;if(!d)return;patchMarquee=null;d.box.remove();if(d.viewport.hasPointerCapture(d.pointerID))d.viewport.releasePointerCapture(d.pointerID);if(cancelled)replacePatchSelection(d.initial);
}
function cancelPatchSelection(){
 if(patchMarquee){finishPatchSelection(true);closePatchSelectionMenu();return true;}
 if(!$('patchSelectionMenu').hidden){closePatchSelectionMenu(true);return true;}return false;
}
function openPatchSelectionMenu(e){
 if(patchPending||patchDragging||patchPan){closePatchSelectionMenu();return false;}
 if(patchMarquee){finishPatchSelection(true);return true;}
 const id=e.target?.closest?.('[data-patch-node]')?.dataset.patchNode;
 const node=id&&patchGraph?.Nodes.find(n=>n.ID===id&&n.Kind!=='output');
 const group=id&&patchGroupByID(id);
 if(!node&&!group&&!patchSelectedNodes.size){closePatchSelectionMenu();return false;}
 const menu=$('patchSelectionMenu'),button=$('patchSelectionGroup'),effect=$('patchContextEffect');
 button.hidden=!patchSelectedNodes.size;button.textContent='Group selected ('+patchSelectedNodes.size+')';
 patchEffectSource=node||group||null;effect.hidden=!patchEffectSource;
 showPatchEffectChoices(false);
 patchMenuPoint={x:e.clientX,y:e.clientY};patchMenuFocus=document.activeElement;
 menu.hidden=false;positionPatchSelectionMenu();
 (patchEffectSource?effect:button).focus({preventScroll:true});return true;
}
$('patchContextEffect').onclick=()=>{
 const source=patchEffectSource,choices=$('patchContextChoices');
 if(!source)return;
 if(!choices.hidden){showPatchEffectChoices(false);positionPatchSelectionMenu();return;}
 const addChoice=(name,choose)=>{
  const button=document.createElement('button');button.type='button';button.textContent=name;
  button.onclick=()=>{closePatchSelectionMenu(true);choose();};
  choices.append(button);
 };
 choices.replaceChildren();
 for(const [Kind,name] of Object.entries(filterNames)){
  addChoice(name,()=>source.NodeIDs?patchGroupAddEffect(source.ID,Kind):insertPatchEffect(source.ID,{Kind,Enabled:true,...filterDefaults[Kind]}));
 }
 if(!source.NodeIDs)for(const filter of micFilters.filter(f=>f.Kind==='vst3')){
  addChoice(filterTitle(filter)+' (new instance)',()=>insertPatchEffect(source.ID,filter));
 }
 showPatchEffectChoices(true);positionPatchSelectionMenu();
 choices.firstElementChild?.focus({preventScroll:true});
};
$('patchSelectionGroup').onclick=()=>{closePatchSelectionMenu();if(patchSelectedNodes.size)openPatchGroupName();};
window.addEventListener('pointermove',movePatchSelection,true);
window.addEventListener('pointerup',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID&&e.button===0){movePatchSelection(e);finishPatchSelection();}},true);
window.addEventListener('pointercancel',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID)finishPatchSelection(true);},true);
$('patchViewport').addEventListener('lostpointercapture',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID)finishPatchSelection(true);});
window.addEventListener('blur',()=>{finishPatchSelection(true);closePatchSelectionMenu();});
document.addEventListener('pointerdown',e=>{if(!$('patchSelectionMenu').contains(e.target))closePatchSelectionMenu();},true);
$('patchSelectionMenu').onkeydown=e=>{
 const choices=$('patchContextChoices');
 if(e.key==='ArrowLeft'&&!choices.hidden){
  e.preventDefault();showPatchEffectChoices(false);positionPatchSelectionMenu();$('patchContextEffect').focus();return;
 }
 if(e.key==='ArrowRight'&&e.target===$('patchContextEffect')&&choices.hidden){e.preventDefault();$('patchContextEffect').click();return;}
 if(e.key!=='ArrowUp'&&e.key!=='ArrowDown')return;
 const controls=[...$('patchSelectionMenu').querySelectorAll('button')].filter(button=>!button.hidden&&!button.closest('[hidden]'));
 if(!controls.length)return;
 e.preventDefault();
 controls[(controls.indexOf(e.target)+(e.key==='ArrowDown'?1:-1)+controls.length)%controls.length].focus();
};

function deleteSelectedPatchNodes(){
 if(!patchGraph||!patchSelectedNodes.size)return false;
 const removed=new Set(patchGraph.Nodes.filter(n=>patchSelectedNodes.has(n.ID)).map(n=>n.ID));
 if(!removePatchNodes(removed))return false;
 patchPending=null;patchPointer=null;patchSelectedNodes.clear();closePatchSelectionMenu();
 commitPatch();patchStatus(removed.size+' '+(removed.size===1?'node removed':'nodes removed')+' from Patch bay.');return true;
}
document.addEventListener('keydown',e=>{
 if(e.key!=='Delete'||e.defaultPrevented||e.repeat||e.isComposing||e.ctrlKey||e.altKey||e.metaKey||e.shiftKey||currentTab!=='patch'||!patchSelectedNodes.size)return;
 if(document.querySelector('dialog[open]')||patchDragging||patchPan||patchMarquee)return;
 if(e.target.closest?.('input:not(.patch-node-select),textarea,select,[contenteditable]:not([contenteditable="false"]),[role="textbox"],[role="slider"],[role="spinbutton"],[role="combobox"],.patch-cable'))return;
 e.preventDefault();e.stopImmediatePropagation();deleteSelectedPatchNodes();
},true);
