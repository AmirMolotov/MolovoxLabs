'use strict';
// Board layout is stored with the library, so grouping travels with exports too.
let boardLayoutSaving=false,boardPadDrag=null,boardSearchTimer=null,boardSearchPlayTimer=null;
let boardSearch={query:'',played:null};
function currentSoundboard(){return library.Boards.find(b=>b.ID===$('boardSelect').value);}
function matchingBoardSounds(board,query){const q=query.trim().toLocaleLowerCase();return (board?.Sounds||[]).filter(s=>s.Name.toLocaleLowerCase().includes(q));}
function moveBoardPad(board,id,groupID,beforeID){
 const at=board.Sounds.findIndex(s=>s.ID===id);if(at<0||id===beforeID)return;
 const [sound]=board.Sounds.splice(at,1);sound.GroupID=groupID;
 const before=board.Sounds.findIndex(s=>s.ID===beforeID);
 if(before>=0)board.Sounds.splice(before,0,sound);else board.Sounds.push(sound);
}
async function persistBoardLayout(board,redraw=true){
 if(boardLayoutSaving)return;boardLayoutSaving=true;$('soundPads').inert=true;$('createPadGroup').disabled=true;
 try{
  const updated=await nativeSaveBoardLayout(board.ID,board.Sounds.map(s=>({ID:s.ID,GroupID:s.GroupID||''})),board.Groups||[]);
  library=updated;if(redraw&&currentSoundboard()?.ID===board.ID)renderPads();
 }catch(e){message(errorText(e));try{library=await nativeLibrary();renderPads();}catch(_){} }
 finally{boardLayoutSaving=false;$('soundPads').inert=false;$('createPadGroup').disabled=false;}
}
function boardButton(label,cls,action){const b=document.createElement('button');b.type='button';b.className=cls;b.textContent=label;b.onclick=action;return b;}
function boardDropZone(node,board,groupID,beforeID){
 node.addEventListener('dragover',e=>{if(!boardPadDrag||boardPadDrag.board!==board.ID||boardLayoutSaving)return;e.preventDefault();e.stopPropagation();e.dataTransfer.dropEffect='move';node.classList.add('pad-drop-target');});
 node.addEventListener('dragleave',e=>{if(!node.contains(e.relatedTarget))node.classList.remove('pad-drop-target');});
 node.addEventListener('drop',e=>{
  if(!boardPadDrag||boardPadDrag.board!==board.ID||boardLayoutSaving)return;e.preventDefault();e.stopPropagation();
  node.classList.remove('pad-drop-target');
  if(boardPadDrag.kind==='group'){if(!groupID||beforeID||groupID===boardPadDrag.id)return;const groups=board.Groups,from=groups.findIndex(g=>g.ID===boardPadDrag.id),to=groups.findIndex(g=>g.ID===groupID);if(from<0||to<0)return;const [g]=groups.splice(from,1);groups.splice(to,0,g);}else moveBoardPad(board,boardPadDrag.id,groupID,beforeID);
  boardPadDrag=null;persistBoardLayout(board);
 });
}
function decorateSoundboard(board){
 const root=$('soundPads');root.classList.add('grouped-pads');
 const pads=new Map([...padViews].map(([id,v])=>[id,v.pad]));root.replaceChildren();
 const groups=board.Groups||[],known=new Set(groups.map(g=>g.ID));
 const makeGrid=groupID=>{
  const grid=document.createElement('div');grid.className='board-pad-grid';grid.dataset.group=groupID;boardDropZone(grid,board,groupID);
  for(const sound of board.Sounds){
   if((known.has(sound.GroupID)?sound.GroupID:'')!==groupID)continue;
   const pad=pads.get(sound.ID);if(!pad)continue;
   const top=pad.querySelector('.sound-pad-top');
   const grip=boardButton('⠿','pad-drag-grip',()=>{});grip.title='Drag to reorder. Arrow keys move this pad.';grip.setAttribute('aria-label','Move '+sound.Name);grip.draggable=true;
   grip.addEventListener('dragstart',e=>{boardPadDrag={board:board.ID,id:sound.ID};e.dataTransfer.setData('text/plain',sound.ID);e.dataTransfer.effectAllowed='move';pad.classList.add('pad-dragging');});
   grip.addEventListener('dragend',()=>{boardPadDrag=null;pad.classList.remove('pad-dragging');document.querySelectorAll('.pad-drop-target').forEach(n=>n.classList.remove('pad-drop-target'));});
   grip.addEventListener('keydown',e=>{if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key)||boardLayoutSaving)return;e.preventDefault();const siblings=board.Sounds.filter(s=>(s.GroupID||'')===groupID),at=siblings.findIndex(s=>s.ID===sound.ID),back=['ArrowLeft','ArrowUp'].includes(e.key),neighbor=siblings[at+(back?-1:1)];if(!neighbor)return;const a=board.Sounds.indexOf(sound),b=board.Sounds.indexOf(neighbor);[board.Sounds[a],board.Sounds[b]]=[board.Sounds[b],board.Sounds[a]];persistBoardLayout(board);});
   top.prepend(grip);
   if(groups.length){const select=document.createElement('select');select.className='pad-group-select';select.setAttribute('aria-label','Group for '+sound.Name);select.add(new Option('Ungrouped',''));for(const g of groups)select.add(new Option(g.Name,g.ID));select.value=groupID;select.onchange=()=>{moveBoardPad(board,sound.ID,select.value);persistBoardLayout(board);};top.insertBefore(select,top.lastElementChild);}
   boardDropZone(pad,board,groupID,sound.ID);grid.append(pad);
  }
  if(!grid.children.length){const hint=document.createElement('p');hint.className='group-drop-hint';hint.textContent='Drop sounds here';grid.append(hint);}
  return grid;
 };
 const loose=document.createElement('section');loose.className='pad-group ungrouped-pads';const label=document.createElement('div');label.className='pad-group-label';label.textContent=groups.length?'Ungrouped':'Your sounds';loose.append(label,makeGrid(''));root.append(loose);
 for(const group of groups){
  const section=document.createElement('section');section.className='pad-group'+(group.Collapsed?' is-collapsed':'');section.dataset.group=group.ID;
  const header=document.createElement('div');header.className='pad-group-header';boardDropZone(header,board,group.ID);
  const toggle=boardButton(group.Collapsed?'▸':'▾','pad-group-toggle',()=>{group.Collapsed=!group.Collapsed;section.classList.toggle('is-collapsed',group.Collapsed);toggle.textContent=group.Collapsed?'▸':'▾';toggle.setAttribute('aria-expanded',String(!group.Collapsed));body.inert=group.Collapsed;persistBoardLayout(board,false);});toggle.setAttribute('aria-expanded',String(!group.Collapsed));toggle.setAttribute('aria-label','Expand or collapse '+group.Name);
  const name=boardButton(group.Name,'pad-group-title',()=>toggle.click());
  const rename=boardButton('✎','pad-group-rename',()=>{const input=document.createElement('input');input.className='pad-group-name';input.value=group.Name;input.maxLength=80;input.setAttribute('aria-label','Group name');name.replaceWith(input);input.focus();input.select();input.onkeydown=e=>{if(e.key==='Enter')input.blur();if(e.key==='Escape'){input.value=group.Name;input.blur();e.stopPropagation();}};input.onblur=()=>{const value=input.value.trim();if(value&&value!==group.Name){group.Name=value;persistBoardLayout(board);}else input.replaceWith(name);};});rename.title='Rename group';rename.setAttribute('aria-label','Rename '+group.Name);
  const count=document.createElement('span');count.className='pad-group-count';count.textContent=String(board.Sounds.filter(s=>s.GroupID===group.ID).length);
  const remove=boardButton('×','pad-group-remove',()=>{board.Groups=groups.filter(g=>g.ID!==group.ID);for(const s of board.Sounds)if(s.GroupID===group.ID)s.GroupID='';persistBoardLayout(board);});remove.title='Remove group and keep its sounds';remove.setAttribute('aria-label','Ungroup '+group.Name);
  const grip=boardButton('⠿','pad-drag-grip',()=>{});grip.draggable=true;grip.title='Drag to reorder groups';grip.setAttribute('aria-label','Move group '+group.Name);grip.addEventListener('dragstart',e=>{boardPadDrag={kind:'group',board:board.ID,id:group.ID};e.dataTransfer.setData('text/plain',group.ID);e.dataTransfer.effectAllowed='move';});grip.addEventListener('dragend',()=>{boardPadDrag=null;document.querySelectorAll('.pad-drop-target').forEach(n=>n.classList.remove('pad-drop-target'));});
  grip.onkeydown=e=>{if(!['ArrowUp','ArrowDown'].includes(e.key)||boardLayoutSaving)return;e.preventDefault();const from=groups.indexOf(group),to=from+(e.key==='ArrowUp'?-1:1);if(to<0||to>=groups.length)return;[groups[from],groups[to]]=[groups[to],groups[from]];persistBoardLayout(board);};
  header.append(grip,toggle,name,count,rename,remove);header.addEventListener('click',e=>{if(e.target===header||e.target===count)toggle.click();});
  const body=document.createElement('div');body.className='pad-group-body';body.inert=!!group.Collapsed;const inner=document.createElement('div');inner.className='pad-group-inner';inner.append(makeGrid(group.ID));body.append(inner);section.append(header,body);root.append(section);
 }
}
$('createPadGroup').onclick=async()=>{const board=currentSoundboard();if(!board||boardLayoutSaving)return;board.Groups??=[];const id=crypto.randomUUID();board.Groups.push({ID:id,Name:'New group',Collapsed:false});await persistBoardLayout(board);const rename=[...$('soundPads').querySelectorAll('.pad-group')].find(n=>n.dataset.group===id)?.querySelector('.pad-group-rename');rename?.click();};
function clearBoardSearch(restore=true){
 clearTimeout(boardSearchTimer);clearTimeout(boardSearchPlayTimer);boardSearch={query:'',played:null};$('boardSearchOverlay').hidden=true;
 if(restore){for(const {pad} of padViews.values())pad.hidden=false;for(const section of $('soundPads').querySelectorAll('.pad-group')){section.hidden=false;section.classList.remove('search-open');const g=currentSoundboard()?.Groups?.find(g=>g.ID===section.dataset.group);const body=section.querySelector('.pad-group-body');if(body)body.inert=!!g?.Collapsed;}}
}
async function playBoardSearchResult(id,boardID,query){
 if(currentTab!=='board'||boardSearch.query!==query||currentSoundboard()?.ID!==boardID||playingSounds.has(id))return;
 try{if(!active&&!await start())return;if(currentTab!=='board'||boardSearch.query!==query||currentSoundboard()?.ID!==boardID)return;await nativePlaySound(id);$('boardStatus').textContent='Playing '+currentSoundboard().Sounds.find(s=>s.ID===id).Name+'.';}catch(e){message(errorText(e));}
}
function updateBoardSearch(){
 const board=currentSoundboard(),query=boardSearch.query,matches=matchingBoardSounds(board,query),ids=new Set(matches.map(s=>s.ID));
 $('boardSearchOverlay').hidden=!query;$('boardSearchText').textContent=query;$('boardSearchCount').textContent=matches.length===1?'1 match · playing':matches.length+' matches · Escape to clear';
 for(const [id,{pad}] of padViews)pad.hidden=!ids.has(id);
 for(const section of $('soundPads').querySelectorAll('.pad-group')){const found=!!section.querySelector('.sound-pad:not([hidden])');section.hidden=!found;section.classList.toggle('search-open',found);const body=section.querySelector('.pad-group-body');if(body)body.inert=false;}
 clearTimeout(boardSearchPlayTimer);const unique=query.trim()&&matches.length===1?matches[0].ID:null;
 if(unique&&boardSearch.played!==unique){boardSearchPlayTimer=setTimeout(()=>{boardSearch.played=unique;playBoardSearchResult(unique,board.ID,query);},280);}
 else if(!unique)boardSearch.played=null;
 clearTimeout(boardSearchTimer);boardSearchTimer=setTimeout(clearBoardSearch,3000);
}
document.addEventListener('keydown',e=>{
 if(currentTab!=='board'||e.isComposing||e.ctrlKey||e.altKey||e.metaKey||e.defaultPrevented||document.querySelector('dialog[open]')||e.target?.closest?.('input,textarea,select,[contenteditable="true"]'))return;
 if(e.key==='Escape'&&boardSearch.query){e.preventDefault();e.stopImmediatePropagation();clearBoardSearch();return;}
 if(e.key==='Backspace'&&boardSearch.query){e.preventDefault();e.stopImmediatePropagation();boardSearch.query=Array.from(boardSearch.query).slice(0,-1).join('');if(!boardSearch.query)clearBoardSearch();else updateBoardSearch();return;}
 if(e.key.length!==1||e.repeat||(!boardSearch.query&&e.key===' '))return;
 e.preventDefault();e.stopImmediatePropagation();boardSearch.query=(boardSearch.query+e.key).slice(0,100);updateBoardSearch();
},true);
window.addEventListener('blur',()=>clearBoardSearch());
