'use strict';
async function openPlaybackRoutes(context, source, targets, isCurrent=()=>true) {
 const players=[];let destination=null, stopped=false;
 const stop=()=>{
  if(stopped)return;stopped=true;
  for(const player of players){player.pause();player.srcObject=null;}
  if(destination){source.disconnect(destination);destination.stream.getTracks().forEach(t=>t.stop());}
  else {try{source.disconnect(context.destination);}catch{}}
 };
 const assertCurrent=()=>{if(!isCurrent())throw Error('Playback setup cancelled.');};
 try {
  if(!targets.length)throw Error('Select at least one playback device.');
  if(new Set(targets.map(t=>t.id)).size!==targets.length)throw Error('Select each playback device only once.');
  if(targets.length===1 && (!targets[0].id || context.setSinkId)){
   if(targets[0].id)await context.setSinkId(targets[0].id);
   assertCurrent();source.connect(context.destination);
   return {stop,mode:'direct',count:1};
  }
  if(typeof HTMLMediaElement.prototype.setSinkId!=='function')throw Error('Multiple outputs require a browser supporting audio device selection.');
  destination=context.createMediaStreamDestination();source.connect(destination);
  for(const target of targets){
   assertCurrent();const player=new Audio();players.push(player);
   player.srcObject=destination.stream;player.volume=1;
   await player.setSinkId(target.id);assertCurrent();
   await player.play();assertCurrent();
  }
  return {stop,mode:'multi-device',count:players.length};
 }catch(error){stop();throw error;}
}
if(typeof module!=='undefined')module.exports={openPlaybackRoutes};
