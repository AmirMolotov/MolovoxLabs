'use strict';
let soundboardView='standard';
function initSoundboardView(value){setSoundboardView(value,false);}
function setSoundboardView(value,persist=true){
 soundboardView=value==='compact'?'compact':'standard';
 $('board').classList.toggle('compact-soundboard',soundboardView==='compact');
 $('boardViewStandard').setAttribute('aria-pressed',String(soundboardView==='standard'));
 $('boardViewCompact').setAttribute('aria-pressed',String(soundboardView==='compact'));
 syncSoundboardViewControls();if(persist)scheduleSave();
}
function syncSoundboardViewControls(){
 const compact=soundboardView==='compact';
 for(const view of padViews.values()){
  const pad=view.pad,top=pad.querySelector('.sound-pad-top'),bottom=pad.querySelector('.sound-pad-bottom'),trim=pad.querySelector('.pad-trim'),effects=pad.querySelector('.pad-effects'),time=pad.querySelector('.clip-time');
  if(!top||!bottom)continue;
  let menu=pad.querySelector('.sound-pad-menu');
  if(!menu){menu=document.createElement('details');menu.className='sound-pad-menu';const summary=document.createElement('summary');summary.textContent='⋯';summary.title='Sound options';summary.setAttribute('aria-label','Options for '+view.sound.Name);const content=document.createElement('div');content.className='sound-pad-menu-content';menu.append(summary,content);pad.append(menu);}
  menu.open=false;const content=menu.querySelector('.sound-pad-menu-content'),group=pad.querySelector('.pad-group-select');
  if(compact){if(trim)content.append(trim);if(effects)content.append(effects);if(group)content.append(group);}
  else{if(trim)bottom.insertBefore(trim,time);if(effects)bottom.insertBefore(effects,time);if(group)top.insertBefore(group,top.lastElementChild);}
 }
}
$('boardViewStandard').onclick=()=>setSoundboardView('standard');
$('boardViewCompact').onclick=()=>setSoundboardView('compact');
document.addEventListener('click',e=>{const option=e.target.closest?.('.sound-pad-menu-content button');if(option)option.closest('.sound-pad-menu').open=false;});
document.addEventListener('keydown',e=>{if(e.key!=='Escape'||document.querySelector('dialog[open]'))return;const menus=document.querySelectorAll('.sound-pad-menu[open]');if(!menus.length)return;e.preventDefault();e.stopImmediatePropagation();for(const menu of menus)menu.open=false;},true);
