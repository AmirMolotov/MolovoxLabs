'use strict';
const boardRecordLimit=120*48000;
let boardRecording=null;
async function readBoardRecording(job,finish=false){
 const response=await fetch(samplerURL(finish?'mic-finish':'mic-audio',job.id)+'?cursor='+job.cursor,{method:finish?'POST':'GET'});
 if(!response.ok)throw new Error(await response.text());
 if(Number(response.headers.get('X-Audio-Dropped'))>0)throw new Error('Microphone capture fell behind.');
 job.cursor=Number(response.headers.get('X-Audio-Cursor'));const data=new Float32Array(await response.arrayBuffer()),count=Math.min(data.length,boardRecordLimit-job.frames);
 if(count>0){job.chunks.push(data.subarray(0,count));job.frames+=count;}
}
function encodeBoardRecording(job){
 const samples=new Float32Array(job.frames);let at=0;for(const chunk of job.chunks){samples.set(chunk,at);at+=chunk.length;}
 return samplerWAV({sampleRate:48000,numberOfChannels:1,getChannelData:()=>samples},0,job.frames/48000);
}
async function feedBoardRecording(job){
 try{while(!job.stopping){await readBoardRecording(job);if(job.stopping)break;
  $('soundRecordStatus').textContent='● Recording '+samplerSeconds(job.frames/48000)+' · '+job.boardName;
  if(job.frames>=boardRecordLimit){queueMicrotask(stopBoardRecording);break;}
  await new Promise(resolve=>setTimeout(resolve,60));
 }}catch(e){job.error=errorText(e);if(!job.stopping)queueMicrotask(stopBoardRecording);}
}
function stopBoardRecording(){const job=boardRecording;if(!job)return;if(!job.finish)job.finish=finishBoardRecording(job);return job.finish;}
async function finishBoardRecording(job){
 job.stopping=true;$('recordSound').disabled=true;$('soundRecordName').disabled=true;$('soundRecordStatus').textContent='Saving recording…';
 try{await job.reader;if(job.frames<boardRecordLimit){try{await readBoardRecording(job,true);}catch(e){job.error=errorText(e);}}
  if(job.frames<2400)throw new Error(job.error||'No audio was captured. Record for a little longer.');
  const name=$('soundRecordName').value.trim()||job.name,response=await fetch(new URL('sampler/bite',location.href),{method:'POST',headers:{'Content-Type':'audio/wav','X-Bite-Board':job.board,'X-Bite-Name':btoa(String.fromCharCode(...new TextEncoder().encode(name)))},body:encodeBoardRecording(job)});
  if(!response.ok)throw new Error(await response.text());library=await response.json();renderLibrary($('boardSelect').value);$('soundRecordStatus').textContent='Saved to '+job.boardName+'.'+(job.error?' Saved the audio captured before the interruption.':'');
 }catch(e){$('soundRecordStatus').textContent='Could not save recording: '+errorText(e);}
 finally{await nativeAbortMicRecording(job.id).catch(()=>{});boardRecording=null;$('recordSound').disabled=false;$('soundRecordName').disabled=false;$('recordSound').textContent='● Record sound';}
}
$('recordSound').onclick=async()=>{
 if(boardRecording){await stopBoardRecording();return;}
 const board=library.Boards.find(item=>item.ID===$('boardSelect').value);if(!board)return;
 $('recordSound').disabled=true;$('soundRecordPanel').hidden=false;$('soundRecordStatus').textContent='Preparing microphones…';
 try{if(!active&&!await start())throw new Error('Select a microphone and an output in Settings first.');const id=await nativeBeginMicRecording();
  const name='Recording '+new Date().toLocaleTimeString();const job={id,board:board.ID,boardName:board.Name,name,cursor:0,frames:0,chunks:[],stopping:false};boardRecording=job;$('soundRecordName').value=name;$('recordSound').textContent='■ Stop & save';job.reader=feedBoardRecording(job);
 }catch(e){$('soundRecordStatus').textContent=errorText(e);}finally{$('recordSound').disabled=false;}
};
window.addEventListener('beforeunload',event=>{if(boardRecording){event.preventDefault();event.returnValue='';}});
