'use strict';
const loadingJobs=new Map();let loadingSequence=0,settingsTail=Promise.resolve();
function recordingIsLive(){return (typeof samplerRecording!=='undefined'&&samplerRecording?.recorder?.state==='recording'&&!samplerRecording.stopping)||(typeof boardRecording!=='undefined'&&!!boardRecording&&!boardRecording.stopping);}
function renderLoading(){
 const live=recordingIsLive(),jobs=[...loadingJobs.values()],blocking=jobs.filter(job=>job.blocking),visible=blocking.at(-1)||jobs.at(-1);
 $('appLoader').hidden=live||!blocking.length;$('appActivity').hidden=live||!jobs.length;
 if(visible){$('appLoaderMessage').textContent=visible.label;$('appActivityLabel').textContent=visible.label;}
 document.querySelector('aside').inert=!live&&!!blocking.length;document.querySelector('.workspace').inert=!live&&!!blocking.length;
}
function beginLoading(label,blocking=false){const id=++loadingSequence;loadingJobs.set(id,{label,blocking});renderLoading();let finished=false;return ()=>{if(finished)return;finished=true;loadingJobs.delete(id);renderLoading();};}
function paintLoading(){return new Promise(resolve=>{let done=false;const finish=()=>{if(!done){done=true;resolve();}};requestAnimationFrame(()=>requestAnimationFrame(finish));setTimeout(finish,80);});}
async function withLoading(label,task,blocking=false){if(recordingIsLive())return await task();const finish=beginLoading(label,blocking);try{await paintLoading();return await task();}finally{finish();}}
function queueSettings(task){const next=settingsTail.catch(()=>{}).then(task);settingsTail=next.catch(()=>{});return next;}
