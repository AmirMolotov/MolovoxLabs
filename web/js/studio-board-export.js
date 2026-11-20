'use strict';
let exportBoardID='',boardExporting=false;
const exportBoardOptions=() => Array.from($('exportBoardChoices').querySelectorAll('input[type="checkbox"]'));

function updateBoardExport(){
 const boxes=exportBoardOptions(),groups=new Set(boxes.filter(box=>box.dataset.kind==='Groups'&&box.checked).map(box=>box.value));
 for(const box of boxes)if(box.dataset.kind==='Sounds')box.disabled=groups.has(box.dataset.group);
 $('saveBoardExport').disabled=boardExporting||!$('exportBoardName').value.trim()||!boxes.some(box=>box.checked&&!box.disabled);
}

$('exportBoard').onclick=()=>{
 const board=currentSoundboard();if(!board)return;
 exportBoardID=board.ID;$('exportBoardName').value=board.Name;$('exportBoardStatus').textContent='';
 const choices=$('exportBoardChoices');choices.replaceChildren();
 const add=(parent,item,kind,checked,group='')=>{
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.value=item.ID;box.checked=checked;box.dataset.kind=kind;box.dataset.group=group;label.append(box,document.createTextNode(item.Name));parent.append(label);
 };
 for(const group of board.Groups||[]){
  const section=document.createElement('fieldset');add(section,group,'Groups',true);
  const pads=document.createElement('div');pads.className='export-group-pads';
  for(const sound of board.Sounds)if(sound.GroupID===group.ID)add(pads,sound,'Sounds',false,group.ID);
  section.append(pads);choices.append(section);
 }
 for(const sound of board.Sounds)if(!(board.Groups||[]).some(group=>group.ID===sound.GroupID))add(choices,sound,'Sounds',true);
 document.querySelector('.board-menu').open=false;
 updateBoardExport();$('exportBoardDialog').showModal();$('exportBoardName').select();
};
$('exportBoardName').oninput=updateBoardExport;
$('exportBoardChoices').onchange=updateBoardExport;
$('clearBoardExport').onclick=()=>{for(const box of exportBoardOptions())box.checked=false;updateBoardExport();};
$('selectBoardExport').onclick=()=>{for(const box of exportBoardOptions())box.checked=box.dataset.kind==='Groups'||!box.dataset.group;updateBoardExport();};
$('closeBoardExport').onclick=()=>{if(!boardExporting)$('exportBoardDialog').close();};
$('exportBoardDialog').addEventListener('cancel',event=>{if(boardExporting)event.preventDefault();});
$('saveBoardExport').onclick=async()=>{
 if(boardExporting||$('saveBoardExport').disabled)return;
 const selection={Name:$('exportBoardName').value.trim(),Groups:[],Sounds:[]};
 for(const box of exportBoardOptions())if(box.checked&&!box.disabled)selection[box.dataset.kind].push(box.value);
 boardExporting=true;updateBoardExport();$('exportBoardStatus').textContent='Choose where to save the package.';
 try{
  if(typeof boardLayoutPending!=='undefined'&&boardLayoutPending)await boardLayoutPending;
  const path=await withLoading('Exporting selected sounds…',()=>nativeExportBoard(exportBoardID,selection),true);
  if(path){$('exportBoardDialog').close();$('boardStatus').textContent='Exported “'+selection.Name+'” as a .qlboard package.';}
  else $('exportBoardStatus').textContent='No file selected.';
 }catch(e){$('exportBoardStatus').textContent=errorText(e);}
 finally{boardExporting=false;updateBoardExport();}
};
