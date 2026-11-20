'use strict';
let trayOpening=false;
function showTraySoundboard(){return showTrayWorkspace(false);}
function showTrayInstruments(){return showTrayWorkspace(true);}
async function showTrayWorkspace(instruments){
 if(trayOpening)return;trayOpening=true;window.MOLOVOX_OPEN_SOUNDBOARD=false;window.MOLOVOX_OPEN_INSTRUMENTS=false;
 try{
  if($('triggerDialog').open)await closeEditor();
  if($('audioTrimDialog').open)closeAudioTrimmer();
  for(const dialog of document.querySelectorAll('dialog[open]')){const cancel=new Event('cancel',{cancelable:true});if(dialog.dispatchEvent(cancel))dialog.close();}
  if(instruments){
   $('instrumentKeyboard').checked=true;openTab('instruments');
   window.focus();$('instrumentKeyboard').focus({preventScroll:true});return;
  }
  if(typeof updateInstrumentInput==='function')$('instrumentKeyboard').checked=false;
  openTab('board');
  if(typeof clearBoardSearch==='function')clearBoardSearch();
  // Focus the board itself so typing immediately searches rather than editing a field.
  window.focus();$('board').tabIndex=-1;$('board').focus({preventScroll:true});
 }finally{trayOpening=false;}
}
window.addEventListener('molovox-open-soundboard',showTraySoundboard);
window.addEventListener('molovox-open-instruments',showTrayInstruments);
if(window.MOLOVOX_OPEN_INSTRUMENTS)showTrayInstruments();
else if(window.MOLOVOX_OPEN_SOUNDBOARD)showTraySoundboard();

function updateAppFocusBorder(focused){document.body.classList.toggle('app-has-focus',focused);}
window.addEventListener('focus',()=>updateAppFocusBorder(true));
window.addEventListener('blur',()=>updateAppFocusBorder(false));
if(typeof document.hasFocus==='function')updateAppFocusBorder(document.hasFocus());
