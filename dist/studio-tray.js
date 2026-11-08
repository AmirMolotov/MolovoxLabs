'use strict';
let trayOpening=false;
async function showTraySoundboard(){
 if(trayOpening)return;trayOpening=true;window.MOLOVOX_OPEN_SOUNDBOARD=false;
 try{
  if($('triggerDialog').open)await closeEditor();
  if($('audioTrimDialog').open)closeAudioTrimmer();
  for(const dialog of document.querySelectorAll('dialog[open]')){const cancel=new Event('cancel',{cancelable:true});if(dialog.dispatchEvent(cancel))dialog.close();}
  openTab('board');
  if(typeof clearBoardSearch==='function')clearBoardSearch();
  // Focus the board itself so typing immediately searches rather than editing a field.
  window.focus();$('board').tabIndex=-1;$('board').focus({preventScroll:true});
 }finally{trayOpening=false;}
}
window.addEventListener('molovox-open-soundboard',showTraySoundboard);
if(window.MOLOVOX_OPEN_SOUNDBOARD)showTraySoundboard();

function updateAppFocusBorder(focused){document.body.classList.toggle('app-has-focus',focused);}
window.addEventListener('focus',()=>updateAppFocusBorder(true));
window.addEventListener('blur',()=>updateAppFocusBorder(false));
if(typeof document.hasFocus==='function')updateAppFocusBorder(document.hasFocus());
