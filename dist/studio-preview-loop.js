'use strict';
let trimLoopMode=null,trimLoopRange=null,trimLoopEpoch=0,trimClickRange=null,trimPreviewTail=Promise.resolve();
function postTrimPreview(id,wav,board,loop=false,epoch=trimLoopEpoch){const job=trimPreviewTail.catch(()=>{}).then(async()=>{if(epoch!==trimLoopEpoch)return false;await nativeStopSound(id);if(epoch!==trimLoopEpoch)return false;const response=await fetch(new URL('sampler/preview',location.href),{method:'POST',headers:{'Content-Type':'audio/wav','X-Sampler-Board':board,'X-Preview-ID':id,...(loop?{'X-Preview-Loop':'true'}:{})},body:wav});if(!response.ok)throw new Error(await response.text());if(epoch!==trimLoopEpoch){await nativeStopSound(id);return false;}return true;});trimPreviewTail=job.catch(()=>{});return job;}
function stopTrimLoop(){const videoWasPlaying=trimLoopMode==='video'||!!trimClickRange;trimLoopEpoch++;trimLoopMode=null;trimLoopRange=null;trimClickRange=null;audioTrimPreviewRange=null;if(videoWasPlaying)$('samplerEditorVideo').pause();nativeStopSound('@audio-preview').catch(()=>{});nativeStopSound('@video-preview').catch(()=>{});}
async function playTrimFrom(kind,time){
 stopTrimLoop();const epoch=trimLoopEpoch,status=kind==='audio'?'audioTrimStatus':'samplerEditorStatus';
 try{const timeline=kind==='audio'?audioTrim:videoTrim,duration=timeline.duration,startAt=Math.max(0,Math.min(time,duration-.05)),endAt=Math.min(duration,startAt+120,Math.max(startAt+.05,startAt<timeline.end?timeline.end:duration));if(duration<.05)return;
  const wav=kind==='audio'?(audioTrimSource?await mediaSelectionWAV(audioTrimSource,startAt,endAt):samplerWAV(audioTrimBuffer,startAt,endAt)):await mediaSelectionWAV(samplerSource,startAt,endAt,$('samplerSilent').checked);
  if(epoch!==trimLoopEpoch)return;if(!active&&!await start())return;if(epoch!==trimLoopEpoch)return;const id=kind==='audio'?'@audio-preview':'@video-preview';if(!await postTrimPreview(id,wav,kind==='audio'?audioTrimBoard:$('samplerBoard').value,false,epoch))return;
  if(kind==='audio')audioTrimPreviewRange={start:startAt,end:endAt,seen:false};else{trimClickRange={start:startAt,end:endAt,seen:false};await seekSamplerVideo($('samplerEditorVideo'),startAt);if(epoch!==trimLoopEpoch)return;await $('samplerEditorVideo').play();}timeline.playhead(startAt);$(status).textContent='Playing from '+samplerSeconds(startAt)+'. Space loops the trim selection.';
 }catch(e){if(epoch===trimLoopEpoch){stopTrimLoop();$(status).textContent=errorText(e);}}
}
async function beginTrimLoop(kind){
 stopTrimLoop();const epoch=trimLoopEpoch;trimLoopMode=kind;const status=kind==='audio'?'audioTrimStatus':'samplerEditorStatus';
 try{
  let range,wav,board;
  if(kind==='audio'){if(!audioTrimBuffer&&!audioTrimSource)throw new Error('Choose audio or video first.');range={start:audioTrim.start,end:audioTrim.end};board=audioTrimBoard;if(range.end-range.start>120)throw new Error('Trim the preview to 2 minutes or less.');wav=await audioSelectionWAV();}
  else{const selected=readSamplerRange();range={start:selected.In,end:selected.Out};board=$('samplerBoard').value;wav=await mediaSelectionWAV(samplerSource,range.start,range.end,$('samplerSilent').checked);}
  if(epoch!==trimLoopEpoch)return;if(!active&&!await start()){if(epoch===trimLoopEpoch)stopTrimLoop();return;}if(epoch!==trimLoopEpoch)return;
  const id=kind==='audio'?'@audio-preview':'@video-preview';if(!await postTrimPreview(id,wav,board,true,epoch))return;
  trimLoopRange={...range,seen:false};
  if(kind==='audio'){audioTrimPreviewRange={...range,seen:false};audioTrim.playhead(range.start);}
  else{const video=$('samplerEditorVideo');await seekSamplerVideo(video,range.start);if(epoch!==trimLoopEpoch)return;await video.play();videoTrim.playhead(range.start);}
  $(status).textContent='Looping the selection · Space to stop.';
 }catch(e){if(epoch===trimLoopEpoch){stopTrimLoop();$(status).textContent=errorText(e);}}
}
function trimSpaceContext(event){
 if((event.code!=='Space'&&event.key!==' ')||event.repeat||event.ctrlKey||event.altKey||event.metaKey)return null;
 if(event.target?.isContentEditable||event.target?.closest?.('input:not([type="range"]),textarea,select,[contenteditable="true"]'))return null;
 if(document.querySelector('#triggerDialog[open]'))return null;
 if($('audioTrimDialog').open)return 'audio';
 const editor=document.querySelector('[data-sampler-view="editor"]');if(currentTab==='sampler'&&editor&&!editor.hidden)return 'video';return null;
}
document.addEventListener('keydown',event=>{const kind=trimSpaceContext(event);if(!kind)return;event.preventDefault();event.stopImmediatePropagation();if(trimLoopMode===kind){stopTrimLoop();$('samplerEditorVideo').pause();$(kind==='audio'?'audioTrimStatus':'samplerEditorStatus').textContent='Preview stopped.';}else beginTrimLoop(kind);},true);
function updateLoopPreview(items){
 if(trimClickRange){const item=items.find(i=>i.ID==='@video-preview');if(item){trimClickRange.seen=true;const at=trimClickRange.start+item.Position,video=$('samplerEditorVideo');if(Math.abs(video.currentTime-at)>.12)video.currentTime=at;videoTrim.playhead(at,true);}else if(trimClickRange.seen){$('samplerEditorVideo').pause();trimClickRange=null;}}
 if(!trimLoopMode||!trimLoopRange)return;const kind=trimLoopMode,id=kind==='audio'?'@audio-preview':'@video-preview',item=items.filter(i=>i.ID===id).sort((a,b)=>b.Sequence-a.Sequence)[0];
 if(!item){if(trimLoopRange.seen)stopTrimLoop();return;}trimLoopRange.seen=true;
 if(kind==='video'){const video=$('samplerEditorVideo'),at=trimLoopRange.start+item.Position;if(Math.abs(video.currentTime-at)>.12)video.currentTime=at;if(video.paused)video.play().catch(()=>{});videoTrim.playhead(at,true);}
}
const originalTrimTimeUpdate=$('samplerEditorVideo').ontimeupdate;
$('samplerEditorVideo').ontimeupdate=()=>{const video=$('samplerEditorVideo');if(trimLoopMode==='video'&&trimLoopRange){if(video.currentTime>=trimLoopRange.end){video.currentTime=trimLoopRange.start;video.play().catch(()=>{});}videoTrim.playhead(video.currentTime,true);}else if(trimClickRange){videoTrim.playhead(video.currentTime,true);if(video.currentTime>=trimClickRange.end){video.pause();trimClickRange=null;}}else originalTrimTimeUpdate?.();};
$('samplerEditorVideo').onended=()=>{if(trimLoopMode==='video'&&trimLoopRange){$('samplerEditorVideo').currentTime=trimLoopRange.start;$('samplerEditorVideo').play().catch(()=>{});}};
$('audioTrimDialog').addEventListener('close',()=>{if(trimLoopMode==='audio')stopTrimLoop();});
