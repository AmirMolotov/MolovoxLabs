'use strict';
let trimLoopMode=null,trimLoopRange=null,trimLoopEpoch=0,trimClickRange=null,trimPreviewTail=Promise.resolve(),audioTrimPlaying=false;
function postTrimPreview(id,wav,board,loop=false,epoch=trimLoopEpoch,offset=0){const job=trimPreviewTail.catch(()=>{}).then(async()=>{if(epoch!==trimLoopEpoch)return false;await nativeStopSound(id);if(epoch!==trimLoopEpoch)return false;const response=await fetch(new URL('sampler/preview',location.href),{method:'POST',headers:{'Content-Type':'audio/wav','X-Sampler-Board':board,'X-Preview-ID':id,...(loop?{'X-Preview-Loop':'true'}:{}),...(offset?{'X-Preview-Offset':String(offset)}:{})},body:wav});if(!response.ok)throw new Error(await response.text());if(epoch!==trimLoopEpoch){await nativeStopSound(id);return false;}return true;});trimPreviewTail=job.catch(()=>{});return job;}
function setAudioTrimPlaying(playing){audioTrimPlaying=playing;$('audioTrimPreview').textContent=playing?'Ⅱ Pause':'▶ Play';}
function stopTrimLoop(){const videoWasPlaying=trimLoopMode==='video'||!!trimClickRange;trimLoopEpoch++;trimLoopMode=null;trimLoopRange=null;trimClickRange=null;audioTrimPreviewRange=null;setAudioTrimPlaying(false);if(videoWasPlaying)$('samplerEditorVideo').pause();nativeStopSound('@audio-preview').catch(()=>{});nativeStopSound('@video-preview').catch(()=>{});}
async function playAudioTrim(time){
 if(!audioTrimSource&&!audioTrimBuffer)return;
 stopTrimLoop();const epoch=trimLoopEpoch,loop=$('audioTrimLoop').checked,duration=audioTrim.duration;
 let cursor=Math.max(0,Math.min(time,duration-.05));
 if(loop&&(cursor<audioTrim.start||cursor>=audioTrim.end-.001))cursor=audioTrim.start;
 const rangeStart=loop?audioTrim.start:cursor,end=loop?audioTrim.end:Math.min(duration,cursor+120,Math.max(cursor+.05,cursor<audioTrim.end?audioTrim.end:duration));
 setAudioTrimPlaying(true);audioTrim.playhead(cursor);
 try{
  if(loop&&end-rangeStart>120)throw new Error('Trim the loop to 2 minutes or less.');
  const wav=audioTrimSource?await mediaSelectionWAV(audioTrimSource,rangeStart,end):samplerWAV(audioTrimBuffer,rangeStart,end);
  if(epoch!==trimLoopEpoch)return;
  if(!active&&!await start()){if(epoch===trimLoopEpoch)stopTrimLoop();return;}
  if(!await postTrimPreview('@audio-preview',wav,audioTrimBoard,loop,epoch,cursor-rangeStart))return;
  audioTrimPreviewRange={start:rangeStart,end,loop,seen:false};
  $('audioTrimStatus').textContent=loop?'Looping the selection.':'Playing from '+samplerSeconds(cursor)+'.';
 }catch(e){if(epoch===trimLoopEpoch){stopTrimLoop();$('audioTrimStatus').textContent=errorText(e);}}
}
function toggleAudioTrimPlayback(){
 if(audioTrimPlaying){stopTrimLoop();$('audioTrimStatus').textContent='Paused at '+samplerSeconds(audioTrim.position)+'.';return;}
 return playAudioTrim(audioTrim.position>=audioTrim.duration-.001?audioTrim.start:audioTrim.position);
}
async function playTrimFrom(kind,time){
 if(kind==='audio')return playAudioTrim(time);
 stopTrimLoop();const epoch=trimLoopEpoch;
 try{
  const duration=videoTrim.duration,startAt=Math.max(0,Math.min(time,duration-.05)),endAt=Math.min(duration,startAt+120,Math.max(startAt+.05,startAt<videoTrim.end?videoTrim.end:duration));if(duration<.05)return;
  const wav=await mediaSelectionWAV(samplerSource,startAt,endAt,$('samplerSilent').checked);
  if(epoch!==trimLoopEpoch)return;if(!active&&!await start())return;if(epoch!==trimLoopEpoch)return;
  if(!await postTrimPreview('@video-preview',wav,$('samplerBoard').value,false,epoch))return;
  trimClickRange={start:startAt,end:endAt,seen:false};await seekSamplerVideo($('samplerEditorVideo'),startAt);
  if(epoch!==trimLoopEpoch)return;await $('samplerEditorVideo').play();videoTrim.playhead(startAt);
  $('samplerEditorStatus').textContent='Playing from '+samplerSeconds(startAt)+'. Tap Space to loop the selection.';
 }catch(e){if(epoch===trimLoopEpoch){stopTrimLoop();$('samplerEditorStatus').textContent=errorText(e);}}
}
async function beginTrimLoop(){
 stopTrimLoop();const epoch=trimLoopEpoch;trimLoopMode='video';
 try{
  const selected=readSamplerRange(),range={start:selected.In,end:selected.Out};
  const wav=await mediaSelectionWAV(samplerSource,range.start,range.end,$('samplerSilent').checked);
  if(epoch!==trimLoopEpoch)return;if(!active&&!await start()){if(epoch===trimLoopEpoch)stopTrimLoop();return;}
  if(!await postTrimPreview('@video-preview',wav,$('samplerBoard').value,true,epoch))return;
  trimLoopRange={...range,seen:false};const video=$('samplerEditorVideo');
  await seekSamplerVideo(video,range.start);if(epoch!==trimLoopEpoch)return;await video.play();videoTrim.playhead(range.start);
  $('samplerEditorStatus').textContent='Looping the selection. Tap Space to stop.';
 }catch(e){if(epoch===trimLoopEpoch){stopTrimLoop();$('samplerEditorStatus').textContent=errorText(e);}}
}
function trimShortcutContext(event,allowCheckbox=false){
 if(event.repeat||event.ctrlKey||event.altKey||event.metaKey)return null;
 if(event.target?.isContentEditable)return null;
 if(event.target!==$('audioTrimLoop')&&event.target?.closest?.(allowCheckbox?'input:not([type="range"]):not([type="checkbox"]),textarea,select,[contenteditable="true"]':'input:not([type="range"]),textarea,select,[contenteditable="true"]'))return null;
 if(document.querySelector('#triggerDialog[open]'))return null;
 if($('audioTrimDialog').open)return 'audio';
 const editor=document.querySelector('[data-sampler-view="editor"]');if(currentTab==='sampler'&&editor&&!editor.hidden)return 'video';return null;
}
let trimSpace=null;
document.addEventListener('keydown',event=>{
 const space=event.code==='Space'||event.key===' ';
 if(space&&trimSpace){event.preventDefault();event.stopImmediatePropagation();return;}
 const kind=trimShortcutContext(event,!space),timeline=kind==='audio'?audioTrim:kind==='video'?videoTrim:null;
 if(!timeline?.duration)return;
 if(event.key==='['||event.key===']'||(event.key==='Escape'&&timeline.trimTool)){
  event.preventDefault();event.stopImmediatePropagation();timeline.setTrimTool(event.key==='['?'start':event.key===']'?'end':null);return;
 }
 if(event.code==='KeyL'&&kind==='audio'){
  event.preventDefault();event.stopImmediatePropagation();const loop=$('audioTrimLoop');if(!loop.disabled){loop.checked=!loop.checked;loop.onchange();}return;
 }
 if(!space)return;
 event.preventDefault();event.stopImmediatePropagation();trimSpace={kind,timeline,pressedAt:event.timeStamp};timeline.setSpacePan(true);
},true);
document.addEventListener('keyup',event=>{
 if((event.code!=='Space'&&event.key!==' ')||!trimSpace)return;
 event.preventDefault();event.stopImmediatePropagation();const {kind,timeline,pressedAt}=trimSpace;releaseTrimSpace();
 if(timeline.panUsed||event.timeStamp-pressedAt>=300||trimShortcutContext(event)!==kind)return;
 if(kind==='audio')toggleAudioTrimPlayback();
 else if(trimLoopMode==='video'){stopTrimLoop();$('samplerEditorStatus').textContent='Preview stopped.';}else beginTrimLoop();
},true);
function releaseTrimSpace(){trimSpace?.timeline.setSpacePan(false);trimSpace=null;}
globalThis.addEventListener?.('blur',releaseTrimSpace);
function updateLoopPreview(items){
 if(trimClickRange){const item=items.find(i=>i.ID==='@video-preview');if(item){trimClickRange.seen=true;const at=trimClickRange.start+item.Position,video=$('samplerEditorVideo');if(Math.abs(video.currentTime-at)>.12)video.currentTime=at;videoTrim.playhead(at,true);}else if(trimClickRange.seen){$('samplerEditorVideo').pause();trimClickRange=null;}}
 if(!trimLoopMode||!trimLoopRange)return;const item=items.filter(i=>i.ID==='@video-preview').sort((a,b)=>b.Sequence-a.Sequence)[0];
 if(!item){if(trimLoopRange.seen)stopTrimLoop();return;}trimLoopRange.seen=true;
 const video=$('samplerEditorVideo'),at=trimLoopRange.start+item.Position;if(Math.abs(video.currentTime-at)>.12)video.currentTime=at;if(video.paused)video.play().catch(()=>{});videoTrim.playhead(at,true);
}
const originalTrimTimeUpdate=$('samplerEditorVideo').ontimeupdate;
$('samplerEditorVideo').ontimeupdate=()=>{const video=$('samplerEditorVideo');if(trimLoopMode==='video'&&trimLoopRange){if(video.currentTime>=trimLoopRange.end){video.currentTime=trimLoopRange.start;video.play().catch(()=>{});}videoTrim.playhead(video.currentTime,true);}else if(trimClickRange){videoTrim.playhead(video.currentTime,true);if(video.currentTime>=trimClickRange.end){video.pause();trimClickRange=null;}}else originalTrimTimeUpdate?.();};
$('samplerEditorVideo').onended=()=>{if(trimLoopMode==='video'&&trimLoopRange){$('samplerEditorVideo').currentTime=trimLoopRange.start;$('samplerEditorVideo').play().catch(()=>{});}};
$('audioTrimDialog').addEventListener('close',()=>{if(trimSpace?.kind==='audio')releaseTrimSpace();});
