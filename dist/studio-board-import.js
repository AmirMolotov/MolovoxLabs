'use strict';
let boardImporting=false;
function openBoardImport(){if(boardImporting)return;$('boardImportStatus').textContent='Choose an option, then select your .qlboard file.';$('importBoardDialog').showModal();}
$('closeBoardImport').onclick=()=>{if(!boardImporting)$('importBoardDialog').close();};
$('importBoardDialog').addEventListener('cancel',e=>{if(boardImporting)e.preventDefault();});
async function importBoardPackage(includeShortcuts){
 if(boardImporting)return;boardImporting=true;
 for(const id of ['importWithShortcuts','importWithoutShortcuts','closeBoardImport','importBoard'])$(id).disabled=true;
 try{
  const count=library.Boards.length,id=$('boardSelect').value;
  const updated=await withLoading('Importing board, audio and layout…',()=>nativeImportBoard(includeShortcuts),true);
  if(updated.Boards.length<=count){$('boardImportStatus').textContent='No file selected.';return;}
  library=updated;renderLibrary(library.Boards.at(-1)?.ID||id);await save();$('importBoardDialog').close();
  $('boardStatus').textContent='Imported audio, groups and pad order'+(includeShortcuts?', with available keyboard and MIDI assignments.':'. Shortcuts were skipped.');
 }catch(e){$('boardImportStatus').textContent=errorText(e);}
 finally{boardImporting=false;for(const id of ['importWithShortcuts','importWithoutShortcuts','closeBoardImport','importBoard'])$(id).disabled=false;}
}
$('importWithShortcuts').onclick=()=>importBoardPackage(true);
$('importWithoutShortcuts').onclick=()=>importBoardPackage(false);
