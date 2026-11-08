'use strict';
let audioTrimSaving=false;
let videoTrim=null,audioTrim=null,audioTrimBuffer=null,audioTrimBoard='',audioTrimLoad=0,audioTrimPreviewRange=null,audioTrimSource=null;
function initTrimEditors(){
 videoTrim=new TrimTimeline($('videoTrim'),{label:'Video trim',video:true,play:time=>playTrimFrom('video',time),hover:time=>previewHoverFrame(time),change:(start,end)=>{if(typeof trimLoopMode!=='undefined'&&trimLoopMode==='video')stopTrimLoop();$('samplerIn').value=start.toFixed(2);$('samplerOut').value=end.toFixed(2);},seek:time=>{if(typeof stopTrimLoop==='function')stopTrimLoop();$('samplerEditorVideo').pause();$('samplerEditorVideo').currentTime=time;}});
 for(const [id,edge]of [['samplerIn','start'],['samplerOut','end']])$(id).onchange=()=>videoTrim.setEdge(edge,Number($(id).value));
 $('samplerEditorVideo').addEventListener('seeked',renderHoverFrame);
 $('samplerSetIn').onclick=()=>videoTrim.setEdge('start',$('samplerEditorVideo').currentTime);$('samplerSetOut').onclick=()=>videoTrim.setEdge('end',$('samplerEditorVideo').currentTime);
 audioTrim=new TrimTimeline($('audioTrim'),{label:'Audio trim',play:time=>playTrimFrom('audio',time),change:()=>{stopAudioTrimPreview();},seek:time=>{audioTrim.playhead(time);}});
 $('openAudioTrim').onclick=()=>openAudioTrimmer();$('closeAudioTrim').onclick=closeAudioTrimmer;
 $('audioTrimDialog').addEventListener('cancel',e=>{e.preventDefault();closeAudioTrimmer();});
 $('audioTrimFile').onclick=async()=>{const generation=++audioTrimLoad;try{const source=await withLoading('Preparing audio / video for trimming…',()=>nativeImportAudioMedia(),true);if(source?.ID){if(generation===audioTrimLoad)applyAudioTrimSource(source);else await disposeAudioTrimSource(source.ID);}}catch(e){if(generation===audioTrimLoad)$('audioTrimStatus').textContent=errorText(e);}};
 $('audioTrimPreview').onclick=async()=>{
  stopTrimLoop();const generation=audioTrimLoad,epoch=trimLoopEpoch;
  try{if(!audioTrimBuffer&&!audioTrimSource)return;if(audioTrim.end-audioTrim.start>120)throw new Error('Trim the preview to 2 minutes or less.');if(!active&&!await start())return;if(generation!==audioTrimLoad)return;
   const range={start:audioTrim.start,end:audioTrim.end,seen:false},wav=await audioSelectionWAV();if(generation!==audioTrimLoad)return;
   if(!await postTrimPreview('@audio-preview',wav,audioTrimBoard,false,epoch)||generation!==audioTrimLoad)return;
   audioTrimPreviewRange=range;audioTrim.playhead(range.start);$('audioTrimStatus').textContent='Playing your selection through the selected outputs.';
  }catch(e){if(generation===audioTrimLoad)$('audioTrimStatus').textContent=errorText(e);}
 };
 $('audioTrimStop').onclick=()=>stopAudioTrimPreview();
 $('audioTrimSave').onclick=saveAudioTrimBite;
}
function applyAudioTrimSource(source){
 releaseAudioTrimResources();audioTrimSource=source;audioTrimBuffer=null;stopAudioTrimPreview();audioTrim.set(source.Seconds,0,Math.min(source.Seconds,10));audioTrim.media(source);$('audioTrimName').value=(source.Name+' bite').slice(0,80);$('audioTrimSave').disabled=audioTrimSaving;$('audioTrimPreview').disabled=false;$('audioTrimStatus').textContent=samplerSeconds(source.Seconds)+' source · save a bite, then keep selecting from this track.';
}
function nextSoundBiteName(name){const match=name.match(/^(.*?)(?: (\d+))?$/);const suffix=' '+(match[2]?Number(match[2])+1:2);return match[1].slice(0,80-suffix.length)+suffix;}
async function saveAudioTrimBite(){
 if(audioTrimSaving||(!audioTrimBuffer&&!audioTrimSource))return;
 const generation=audioTrimLoad,board=audioTrimBoard,name=$('audioTrimName').value.trim();
 try{
  if(!name)throw new Error('Name your sound bite.');if(audioTrim.end-audioTrim.start>120)throw new Error('Trim your sound bite to 2 minutes or less.');
  audioTrimSaving=true;$('audioTrimSave').disabled=true;
  await withLoading('Saving sound bite…',async()=>{
   const response=await fetch(new URL('sampler/bite',location.href),{method:'POST',headers:{'Content-Type':'audio/wav','X-Bite-Board':board,'X-Bite-Name':btoa(String.fromCharCode(...new TextEncoder().encode(name)))},body:await audioSelectionWAV()});
   if(!response.ok)throw new Error(await response.text());library=await response.json();renderLibrary(board);$('boardStatus').textContent='Added '+name+'.';
   if(generation===audioTrimLoad){$('audioTrimStatus').textContent='Saved “'+name+'” to your board. Select another part of this track to make the next bite.';$('audioTrimName').value=nextSoundBiteName(name);$('audioTrimSave').textContent='＋ Save next bite';}
  },true);
 }catch(e){if(generation===audioTrimLoad)$('audioTrimStatus').textContent=errorText(e);}
 finally{audioTrimSaving=false;if(audioTrimSource||audioTrimBuffer)$('audioTrimSave').disabled=false;}
}
function isTrimMediaFile(file){return /\.(wav|mp3|flac|ogg|opus|aac|m4a|aiff|aif|wma|mp4|mkv|webm|mov|avi|m4v|mpeg|mpg|ts|m2ts|wmv|flv)$/i.test(file.name);}
async function openDroppedAudioTrim(file){
 if(!isTrimMediaFile(file))throw new Error('Drop an audio or video file.');if(file.size>8*1024*1024*1024)throw new Error('Choose a file up to 8 GB.');
 if(!$('audioTrimDialog').open)await openAudioTrimmer();
 const generation=++audioTrimLoad;stopAudioTrimPreview();$('audioTrimStatus').textContent='Importing '+file.name+'…';$('audioTrimSave').disabled=$('audioTrimPreview').disabled=true;
 try{const source=await withLoading('Preparing '+file.name+'…',async()=>{const response=await fetch(new URL('sampler/audio-import?name='+encodeURIComponent(file.name),location.href),{method:'POST',body:file});if(!response.ok)throw new Error(await response.text());return response.json();},true);if(generation===audioTrimLoad)applyAudioTrimSource(source);else await disposeAudioTrimSource(source.ID);}
 catch(e){if(generation===audioTrimLoad){$('audioTrimStatus').textContent=errorText(e);$('audioTrimSave').disabled=$('audioTrimPreview').disabled=!(audioTrimSource||audioTrimBuffer);}throw e;}
}
function isBoardFileDrag(e){return currentTab==='board'&&Array.from(e.dataTransfer?.types||[]).includes('Files')&&!document.querySelector('dialog[open]:not(#audioTrimDialog)');}
document.addEventListener('dragover',e=>{if(!isBoardFileDrag(e))return;e.preventDefault();e.dataTransfer.dropEffect='copy';$('board').classList.add('media-drop-target');},true);
document.addEventListener('dragleave',e=>{if(!e.relatedTarget)$('board').classList.remove('media-drop-target');},true);
document.addEventListener('drop',e=>{
 $('board').classList.remove('media-drop-target');if(!isBoardFileDrag(e))return;e.preventDefault();e.stopPropagation();
 const files=Array.from(e.dataTransfer.files||[]);if(files.length!==1){message('Drop one audio or video file at a time to trim it.');return;}
 openDroppedAudioTrim(files[0]).catch(e=>message(errorText(e)));
},true);
window.addEventListener('dragend',()=>$('board').classList.remove('media-drop-target'));
async function sendTrimPreview(wav,board){await postTrimPreview('@audio-preview',wav,board);}
async function openAudioTrimmer(sound){stopTrimLoop();const generation=++audioTrimLoad;audioTrimPreviewRange=null;audioTrimBoard=$('boardSelect').value;releaseAudioTrimResources();$('audioTrimName').value='';$('audioTrimFile').value='';$('audioTrimSave').disabled=$('audioTrimPreview').disabled=true;$('audioTrimSave').textContent='＋ Save sound bite';$('audioTrimStatus').textContent='Choose audio or video, then drag IN and OUT to extract a sound bite. The source stays unchanged.';$('audioTrimDialog').showModal();if(sound)try{const response=await fetch(samplerURL('clip',sound.ID));if(!response.ok)throw new Error('Could not open this clip.');const bytes=await response.arrayBuffer();if(generation!==audioTrimLoad)return;await loadAudioTrim(bytes,sound.Name);}catch(e){$('audioTrimStatus').textContent=errorText(e);}}
async function loadAudioTrim(bytes,name){const generation=++audioTrimLoad;releaseAudioTrimResources();$('audioTrimStatus').textContent='Loading audio and drawing waveform…';$('audioTrimSave').disabled=$('audioTrimPreview').disabled=true;await nativeStopSound('@audio-preview');await withLoading('Drawing audio waveform…',async()=>{const context=new OfflineAudioContext(1,1,48000);let buffer;try{buffer=await context.decodeAudioData(bytes);}catch(e){throw new Error('Could not read the audio track. Try WAV, MP3, MP4 with AAC, or WebM with Opus.');}if(generation!==audioTrimLoad)return;if(!buffer.duration||buffer.duration>1800)throw new Error('Choose audio up to 30 minutes long.');audioTrimBuffer=buffer;audioTrim.set(buffer.duration,0,Math.min(buffer.duration,10));audioTrim.waveform(buffer);$('audioTrimName').value=(name+' bite').slice(0,80);$('audioTrimSave').disabled=$('audioTrimPreview').disabled=false;$('audioTrimStatus').textContent=samplerSeconds(buffer.duration)+' source · drag either edge, or zoom in for a precise cut.';},true);}
async function disposeAudioTrimSource(id){
 if(typeof nativeReleaseAudioTrimSource!=='function')return;
 try{await nativeReleaseAudioTrimSource(id);}catch(e){message('Could not release the temporary trim source: '+errorText(e));}
}
function releaseAudioTrimResources(){
 const previous=audioTrimSource;stopTrimLoop();audioTrimPreviewRange=null;audioTrimSource=null;audioTrimBuffer=null;
 audioTrim.waveAbort?.abort();audioTrim.waveAbort=null;audioTrim.waveVersion++;audioTrim.buffer=null;audioTrim.mediaID=null;audioTrim.waveError=null;audioTrim.hoverPopup.hidden=true;audioTrim.set(0);
 if(previous?.ID)disposeAudioTrimSource(previous.ID);
}
function closeAudioTrimmer(){audioTrimLoad++;releaseAudioTrimResources();$('audioTrimDialog').close();}

function previewHoverFrame(time){const video=$('samplerEditorVideo');if(video.paused===false){videoTrim.hoverPopup.hidden=true;return;}if(!samplerSource||video.readyState<2)return;video.pause();video.currentTime=Math.min(time,Math.max(0,samplerDuration-.001));videoTrim.playhead(time);if(!video.seeking)renderHoverFrame();}
function renderHoverFrame(){const video=$('samplerEditorVideo');if(!videoTrim.hoverPopup||videoTrim.hoverPopup.hidden||video.readyState<2)return;drawSamplerCrop(videoTrim.hoverCanvas.getContext('2d'),video,{X:0,Y:0,Width:1,Height:1},240,135);}
async function drawVideoTrimFrames(source){
 if(videoTrim.film?.id===source.ID){videoTrim.onViewport();return;}
 if(videoTrim.film){videoTrim.film.video.pause();videoTrim.film.video.removeAttribute('src');videoTrim.film.video.load();}
 const video=document.createElement('video');video.muted=true;video.preload='auto';const film={id:source.ID,video,version:0,pending:null,busy:false};videoTrim.film=film;
 const loaded=waitVideo(video,'loadeddata');video.src=samplerURL('source',source.ID);video.load();film.ready=loaded;
 videoTrim.onViewport=()=>{if(videoTrim.film!==film)return;film.pending={...videoTrim.viewport(),version:++film.version};renderVideoFilmstrip(film);};videoTrim.onViewport();
}
async function renderVideoFilmstrip(film){if(film.busy)return;film.busy=true;try{await film.ready;while(film.pending&&videoTrim.film===film){const v=film.pending;film.pending=null;const {ctx}=videoTrim.surface(videoTrim.canvas,v);ctx.fillStyle='#0b1711';ctx.fillRect(0,0,v.width,80);const count=Math.max(1,Math.ceil(v.width/142)),cell=v.width/count;for(let i=0;i<count;i++){if(film.version!==v.version||videoTrim.film!==film)break;const time=Math.min(samplerDuration-.001,v.start+(i+.5)/count*(v.end-v.start));await seekSamplerVideo(film.video,time);if(film.version!==v.version||videoTrim.film!==film)break;const sw=film.video.videoWidth,sh=film.video.videoHeight,scale=Math.max(cell/sw,80/sh),cw=cell/scale,ch=80/scale;ctx.drawImage(film.video,(sw-cw)/2,(sh-ch)/2,cw,ch,i*cell,0,cell,80);}}}catch(e){$('samplerWaveStatus').textContent='Frame strip unavailable: '+errorText(e);}finally{film.busy=false;}}
async function mediaSelectionWAV(source,start,end,silent=false){const response=await fetch(samplerURL('media-wav',source.ID)+'?start='+start+'&end='+end+'&silent='+silent);if(!response.ok)throw new Error(await response.text());return response.arrayBuffer();}
async function audioSelectionWAV(){return audioTrimSource?mediaSelectionWAV(audioTrimSource,audioTrim.start,audioTrim.end):samplerWAV(audioTrimBuffer,audioTrim.start,audioTrim.end);}
function stopAudioTrimPreview(){if(typeof trimLoopMode!=='undefined'&&trimLoopMode==='audio')stopTrimLoop();audioTrimPreviewRange=null;nativeStopSound('@audio-preview').catch(()=>{});}
function updateTrimPlayback(items){if(!audioTrimPreviewRange||!audioTrim)return;const r=audioTrimPreviewRange,item=items.filter(i=>i.ID==='@audio-preview').sort((a,b)=>b.Sequence-a.Sequence)[0];if(item){r.seen=true;audioTrim.playhead(Math.min(r.end,r.start+item.Position),true);}else if(r.seen){audioTrim.playhead(r.end);audioTrimPreviewRange=null;}}
$('samplerWaveform').onchange=()=>{videoTrim.showAudio($('samplerWaveform').checked);if($('samplerWaveform').checked&&samplerSource)loadVideoWaveform(samplerSource);};
async function loadVideoWaveform(source){videoTrim.media(source);$('samplerWaveStatus').textContent='Audio waveform · hover to preview frames · align the green edge with the first sound.';}
$('samplerPatch').onclick=()=>{const board=$('samplerBoard').value;openTab('patch');if(!patchGraph.Nodes.some(n=>n.Kind==='board'&&n.BoardID===board))addPatchNode(patchNode('board',{BoardID:board},40,180+patchGraph.Nodes.length*20));else renderPatch();patchStatus('Connect this video sampler to an effect or output, and enable Patch bay.');};
$('samplerCropDetails').ontoggle=()=>{$('samplerCropBox').hidden=!$('samplerCropDetails').open;};
initTrimEditors();
