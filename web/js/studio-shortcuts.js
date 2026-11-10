'use strict';
function shortcutKeyLabel(key){
 if(key.Label)return key.Label;
 const modifiers=[];if(key.Modifiers&2)modifiers.push('Ctrl');if(key.Modifiers&1)modifiers.push('Alt');if(key.Modifiers&4)modifiers.push('Shift');
 const code=Number(key.Key),special={32:'Space',13:'Enter',27:'Escape',9:'Tab',8:'Backspace',46:'Delete'};
 modifiers.push(special[code]||(code>=112&&code<=135?'F'+(code-111):String.fromCharCode(code)));return modifiers.join('+');
}
function shortcutMIDILabel(m){return 'MIDI '+(m.Kind==='note'?'Note ':'CC ')+m.Number+' · Ch '+m.Channel;}
function shortcutSummary(entry){return [...clipKeys(entry).map(shortcutKeyLabel),...clipMIDIs(entry).map(shortcutMIDILabel)].join(' · ');}
function collectShortcutSections(boards,filters,groups,keys,midis){
 const sections=[];
 for(const b of boards){const rows=(b.Sounds||[]).filter(s=>clipKeys(s).length||clipMIDIs(s).length).map(s=>({id:s.ID,name:s.Name,keys:clipKeys(s),midis:clipMIDIs(s)}));if(rows.length)sections.push({name:(b.Kind==='video'?'Video sampler · ':'Soundboard · ')+b.Name,board:b,rows});}
 const effects=(filters||[]).filter(f=>clipKeys(f).length||clipMIDIs(f).length).map(f=>({id:'@filter:'+f.ID,name:filterTitle(f),keys:clipKeys(f),midis:clipMIDIs(f)}));if(effects.length)sections.push({name:'Effects',rows:effects});
 const grouped=(groups||[]).filter(g=>clipKeys(g).length||clipMIDIs(g).length).map(g=>({id:'@group:'+g.ID,name:g.Name,keys:clipKeys(g),midis:clipMIDIs(g)}));if(grouped.length)sections.push({name:'Filter groups',rows:grouped});
 if(keys.length||midis.length)sections.push({name:'Whole filter chain',rows:[{id:'@all',name:'Toggle all filters',keys,midis}]});
 return sections;
}
function renderShortcuts(){
 const root=$('shortcutList');root.replaceChildren();
 const sections=collectShortcutSections(library.Boards,micFilters,filterGroups,allFilterHotkeys,allFilterMIDIs);
 $('shortcutScope').textContent=$('globalKeys').checked?'Global shortcuts are enabled. Sound and video shortcuts follow the selected board in each tab.':'Global keyboard shortcuts are off. Enable them in Settings.';
 if(!sections.length){const p=document.createElement('p');p.className='card hint';p.textContent='No assignments yet. Add shortcuts or MIDI triggers from a pad or effect.';root.append(p);}
 for(const section of sections){const card=document.createElement('section');card.className='card shortcut-section';const heading=document.createElement('h2');heading.textContent=section.name;card.append(heading);
  if(section.board){const selected=section.board.ID===$(section.board.Kind==='video'?'samplerBoard':'boardSelect').value;const scope=document.createElement('p');scope.className='hint';scope.textContent=selected?'Selected board · triggers are active':'Select this board to use its triggers';card.append(scope);}
  for(const row of section.rows){const line=document.createElement('div');line.className='shortcut-row';const name=document.createElement('strong');name.textContent=row.name;const badges=document.createElement('div');badges.className='shortcut-badges';for(const text of [...row.keys.map(shortcutKeyLabel),...row.midis.map(shortcutMIDILabel)]){const badge=document.createElement('span');badge.textContent=text;badges.append(badge);}const edit=document.createElement('button');edit.className='secondary';edit.textContent='Edit';edit.setAttribute('aria-label','Edit shortcuts for '+row.name);edit.onclick=()=>openTriggerEditor(row.id);line.append(name,badges,edit);card.append(line);}root.append(card);
 }
 const controls=$('workspaceShortcutList');controls.replaceChildren();
 for(const [key,action] of [
  ['Alt, Alt','From anywhere in Windows: open Molovox Labs on the Soundboard tab.'],
  ['Type · Backspace · Escape','Soundboard: search, edit the search, clear it. A unique match plays automatically.'],
  ['Space','Trimmer: toggle looping preview. Arrangement: play or pause.'],
  ['Delete','Patch bay: remove selected nodes and their cables. Text fields keep normal editing behavior.'],
  ['Space + drag','Pan the Patch bay or Arrangement.'],
  ['Wheel · Ctrl + wheel','Zoom or move sideways in trimmers, Patch bay and Arrangement.'],
  ['← / → · Shift + ← / →','Focused trim handle: adjust by 0.01 seconds or 1 second. Home / End: jump to the source edge.'],
  ['Arrow keys on a grip','Reorder a sound pad or group.'],
  ['Escape','Clear soundboard search, cancel a pending cable or close a dialog. With no dialog or search open: stop playback and listening.'],
 ]){const line=document.createElement('div');line.className='shortcut-row workspace-shortcut';const label=document.createElement('strong');label.textContent=key;const detail=document.createElement('span');detail.textContent=action;line.append(label,detail);controls.append(line);}
}
