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
function closePatchSelectionMenu(){$('patchSelectionMenu').hidden=true;}
function beginPatchSelection(e){
 closePatchSelectionMenu();if(patchMarquee||patchSpace||e.button!==0)return;
 const viewport=$('patchViewport'),bounds=viewport.getBoundingClientRect(),initial=[...patchSelectedNodes],base=e.shiftKey||e.ctrlKey||e.metaKey?initial:[];
 const cards=[...$('patchNodes').querySelectorAll('.patch-node')].map(card=>({id:card.dataset.patchNode,rect:card.getBoundingClientRect()}));
 const box=document.createElement('div');box.className='patch-marquee';box.hidden=true;box.setAttribute('aria-hidden','true');viewport.append(box);
 patchMarquee={viewport,bounds,pointerID:e.pointerId,x:e.clientX,y:e.clientY,initial,base,cards,box,moved:false};replacePatchSelection(base);viewport.setPointerCapture(e.pointerId);
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
 if(!$('patchSelectionMenu').hidden){closePatchSelectionMenu();return true;}return false;
}
function openPatchSelectionMenu(e){
 if(patchPending||patchDragging||patchPan){closePatchSelectionMenu();return false;}
 if(patchMarquee){finishPatchSelection(true);return true;}
 if(!patchSelectedNodes.size){closePatchSelectionMenu();return false;}
 const menu=$('patchSelectionMenu'),button=$('patchSelectionGroup');button.textContent='Group selected ('+patchSelectedNodes.size+')';menu.hidden=false;
 menu.style.left=Math.max(12,Math.min(e.clientX,innerWidth-(menu.offsetWidth||210)-12))+'px';menu.style.top=Math.max(12,Math.min(e.clientY,innerHeight-(menu.offsetHeight||48)-12))+'px';button.focus({preventScroll:true});return true;
}
$('patchSelectionGroup').onclick=()=>{closePatchSelectionMenu();if(patchSelectedNodes.size)openPatchGroupName();};
window.addEventListener('pointermove',movePatchSelection,true);
window.addEventListener('pointerup',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID&&e.button===0){movePatchSelection(e);finishPatchSelection();}},true);
window.addEventListener('pointercancel',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID)finishPatchSelection(true);},true);
$('patchViewport').addEventListener('lostpointercapture',e=>{if(patchMarquee&&e.pointerId===patchMarquee.pointerID)finishPatchSelection(true);});
window.addEventListener('blur',()=>{finishPatchSelection(true);closePatchSelectionMenu();});
document.addEventListener('pointerdown',e=>{if(!$('patchSelectionMenu').contains(e.target))closePatchSelectionMenu();},true);
$('patchSelectionMenu').onkeydown=e=>{if(e.key==='ArrowUp'||e.key==='ArrowDown'){e.preventDefault();$('patchSelectionGroup').focus();}};
