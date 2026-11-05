'use strict';
const $=id=>document.getElementById(id);
let devices=[],ctx=null,streams=[],nodes=[],mix,inverter,delay,master,analyser,frame,starting=false,generation=0,hotSince=0,outputAnalyser,playbackRoutes=null;
const message=t=>$('status').textContent=t;
const supported=!!navigator.mediaDevices?.getUserMedia;
if(!supported){message('Microphone access requires HTTPS or localhost and a supported browser.');$('refresh').disabled=$('start').disabled=true;}
function controls(active){for(const el of document.querySelectorAll('#inputs input,#aec,#bassFilter,#sinks input,#refresh'))el.disabled=active;$('start').disabled=active;$('stop').disabled=!active;}
async function enumerate(){
 const old=new Map([...document.querySelectorAll('.mic')].map(row=>[row.dataset.id,{checked:row.querySelector('input').checked,gain:row.querySelector('[type=range]').value}]));
 devices=await navigator.mediaDevices.enumerateDevices();$('inputs').replaceChildren();$('inputs').className='';
 const seen=new Set();
 for(const d of devices.filter(d=>d.kind==='audioinput'&&d.deviceId!=='communications')){
  // Avoid aliases that would record the default physical device twice.
  if(d.deviceId==='default'&&devices.some(x=>x.kind==='audioinput'&&x.deviceId!=='default'&&x.groupId===d.groupId))continue;
  if(seen.has(d.deviceId))continue;seen.add(d.deviceId);
  const row=document.createElement('div');row.className='mic';row.dataset.id=d.deviceId;
  const label=document.createElement('label'),box=document.createElement('input');box.type='checkbox';box.value=d.deviceId;box.checked=old.get(d.deviceId)?.checked??seen.size===1;
  label.append(box,document.createTextNode(d.label||'Microphone '+seen.size));
  const gain=document.createElement('input');gain.type='range';gain.min=0;gain.max=100;gain.value=old.get(d.deviceId)?.gain??100;gain.setAttribute('aria-label',(d.label||'Microphone')+' mix level');
  const readout=document.createElement('output');readout.textContent=gain.value+'%';
  gain.oninput=()=>readout.textContent=gain.value+'%';
  row.append(label,gain,readout);$('inputs').append(row);
 }
 if(!seen.size)$('inputs').textContent='No microphones found.';

 const previous=new Set([...document.querySelectorAll('#sinks input:checked')].map(el=>el.value));
 const canRoute=!!(window.AudioContext?.prototype.setSinkId||HTMLMediaElement.prototype.setSinkId);
 const canMulti=!!HTMLMediaElement.prototype.setSinkId;
 $('sinks').replaceChildren();
 const outputs=[{deviceId:'',label:'System default'}];
 if(canRoute)outputs.push(...devices.filter(d=>d.kind==='audiooutput'&&!['default','communications'].includes(d.deviceId)));
 for(const d of outputs){
  const label=document.createElement('label');label.className='switch';
  const box=document.createElement('input');box.type='checkbox';box.value=d.deviceId;box.checked=previous.has(d.deviceId)||(!previous.size&&!d.deviceId);
  box.onchange=()=>{
   const boxes=[...document.querySelectorAll('#sinks input')];
   if(box.checked)for(const other of boxes){if(other!==box&&(!canMulti||!box.value||!other.value))other.checked=false;}
  };
  label.append(box,document.createTextNode(d.label||'Speaker / headphone'));$('sinks').append(label);
 }
 $('sinkNote').textContent=canMulti?'Select multiple devices to mirror audio. System default is used alone to avoid duplicate playback. Devices may drift or play with different delays.':canRoute?'This browser supports one output device at a time.':'This browser uses the system default output. Change it in system sound settings.';

}
$('refresh').onclick=async()=>{let probe;try{$('refresh').disabled=true;probe=await navigator.mediaDevices.getUserMedia({audio:true});await enumerate();message('Devices connected. Select your microphones, then start listening.');}catch(e){message('Could not connect microphones: '+e.message);}finally{probe?.getTracks().forEach(t=>t.stop());$('refresh').disabled=false;}};
function update(){
 $('polarityState').textContent=$('invert').checked?'Polarity: inverted (× −1)':'Polarity: normal (× +1)';
 $('volumeValue').textContent=$('volume').value+'%';$('delayValue').textContent=$('delay').value+' ms';
 if(ctx&&master){message('Listening. '+($('consent').checked?'Experimental playback enabled.':'Output muted until enabled.'));master.gain.setTargetAtTime($('consent').checked?Number($('volume').value)/100:0,ctx.currentTime,.03);inverter.gain.setTargetAtTime($('invert').checked?-1:1,ctx.currentTime,.01);delay.delayTime.setTargetAtTime(Number($('delay').value)/1000,ctx.currentTime,.01);}
}
['volume','delay','invert','consent'].forEach(id=>$(id).oninput=update);
async function stop(reason='Stopped. Microphones released.'){
 generation++;starting=false;tonePlaying=false;cancelAnimationFrame(frame);playbackRoutes?.stop();playbackRoutes=null;if(master&&ctx)master.gain.value=0;
 streams.forEach(s=>s.getTracks().forEach(t=>t.stop()));streams=[];nodes=[];const previous=ctx;ctx=null;master=null;if(previous)await previous.close().catch(()=>{});
 $('outputLevel').textContent='— dBFS';$('consent').checked=false;$('badge').textContent='IDLE';$('count').textContent='0';$('level').innerHTML='— <em>dBFS</em>';$('latency').innerHTML='— <em>ms</em>';$('details').textContent='';controls(false);message(reason);drawIdle();
}
$('stop').onclick=()=>stop();document.addEventListener('keydown',e=>{if(e.key==='Escape')stop();});
$('start').onclick=async()=>{
 if(ctx||starting)return;
 const selected=[...document.querySelectorAll('.mic')].filter(row=>row.querySelector('input').checked);
 if(!selected.length){message('Connect devices and select at least one microphone.');return;}

 const targets=[...document.querySelectorAll('#sinks input:checked')].map(el=>({id:el.value}));
 if(!targets.length){message('Select at least one playback device.');return;}
 starting=true;controls(true);const token=++generation;
 try{
 ctx=new AudioContext({latencyHint:'interactive'});await ctx.resume();if(token!==generation)return;
 if(token!==generation)return;
 mix=ctx.createGain();mix.gain.value=1/selected.length;
 const hp=ctx.createBiquadFilter();hp.type='highpass';hp.frequency.value=40;
 const lp=ctx.createBiquadFilter();lp.type='lowpass';lp.frequency.value=800;
 analyser=ctx.createAnalyser();analyser.fftSize=2048;
 inverter=createPolarityStage(ctx,$('invert').checked);delay=ctx.createDelay(.1);master=ctx.createGain();master.gain.value=0;
 const compressor=ctx.createDynamicsCompressor();compressor.threshold.value=-1;compressor.knee.value=0;compressor.ratio.value=20;compressor.attack.value=.003;
 outputAnalyser=ctx.createAnalyser();outputAnalyser.fftSize=2048;
 if($('bassFilter').checked)mix.connect(hp).connect(lp).connect(analyser);
 else mix.connect(analyser);
 analyser.connect(inverter).connect(delay).connect(master).connect(compressor).connect(outputAnalyser);

 const routes=await openPlaybackRoutes(ctx,outputAnalyser,targets,()=>token===generation);
 if(token!==generation){routes.stop();return;}playbackRoutes=routes;
 const settings=[];
 for(const row of selected){
  const stream=await navigator.mediaDevices.getUserMedia({audio:{deviceId:{exact:row.dataset.id},echoCancellation:$('aec').checked,noiseSuppression:false,autoGainControl:false}});
  if(token!==generation){stream.getTracks().forEach(t=>t.stop());return;}
  streams.push(stream);
  stream.getAudioTracks()[0].onended=()=>stop('An input disconnected. Session stopped.');
  const source=ctx.createMediaStreamSource(stream),gain=ctx.createGain();gain.gain.value=Number(row.querySelector('[type=range]').value)/100;
  source.connect(gain).connect(mix);nodes.push(source,gain);
  const state=stream.getAudioTracks()[0].getSettings().echoCancellation;settings.push(state===true?'on':state===false?'off':'not reported');
 }
 starting=false;update();$('badge').textContent='LISTENING';$('count').textContent=streams.length;
 const ms=playbackRoutes.mode==='direct'?ctx.outputLatency:NaN;if(Number.isFinite(ms))$('latency').innerHTML=(ms*1000).toFixed(1)+' <em>ms</em>';
 $('details').textContent='Echo cancellation by input: '+settings.join(', ')+'. Playback: '+($('bassFilter').checked?'40–800 Hz filter':'full-band')+'. Outputs: '+playbackRoutes.count+(playbackRoutes.mode==='multi-device'?' (independent device timing; latency unavailable).':'.');
 message('Listening. '+($('consent').checked?'Experimental playback enabled.':'Output muted until enabled.'));
 hotSince=0;paint();
 }catch(e){if(token===generation)await stop('Could not start: '+e.message);}
};

const micCanvas=$('micScope'),outputCanvas=$('outputScope');
function wave(canvas,samples,color){
 const pen=canvas.getContext('2d'),ratio=window.devicePixelRatio||1;
 const w=canvas.width=Math.max(1,Math.round(canvas.clientWidth*ratio)),h=canvas.height=Math.max(1,Math.round(canvas.clientHeight*ratio));
 pen.clearRect(0,0,w,h);pen.strokeStyle='#ffffff09';pen.lineWidth=1;
 for(let x=0;x<w;x+=w/12){pen.beginPath();pen.moveTo(x,0);pen.lineTo(x,h);pen.stroke();}
 for(let y=0;y<=h;y+=h/4){pen.beginPath();pen.moveTo(0,y);pen.lineTo(w,y);pen.stroke();}
 pen.strokeStyle=color;pen.lineWidth=1.5*ratio;pen.beginPath();
 if(samples){for(let i=0;i<samples.length;i++){const x=i/(samples.length-1)*w,y=h/2+samples[i]*h*.45;i?pen.lineTo(x,y):pen.moveTo(x,y);}}
 else {pen.moveTo(0,h/2);pen.lineTo(w,h/2);}
 pen.stroke();
}
function drawIdle(){wave(micCanvas,null,'#b9e78d55');wave(outputCanvas,null,'#cf968155');}
function paint(){
 if(!ctx)return;
 const data=new Float32Array(analyser.fftSize);analyser.getFloatTimeDomainData(data);
 const rms=Math.sqrt(data.reduce((sum,v)=>sum+v*v,0)/data.length),db=20*Math.log10(Math.max(rms,1e-6));
 $('level').innerHTML=db.toFixed(1)+' <em>dBFS</em>';
 const output=new Float32Array(outputAnalyser.fftSize);outputAnalyser.getFloatTimeDomainData(output);
 const outputRms=Math.sqrt(output.reduce((sum,v)=>sum+v*v,0)/output.length);
 $('outputLevel').textContent=(20*Math.log10(Math.max(outputRms,1e-6))).toFixed(1)+' dBFS';
 wave(micCanvas,data,'#b9e78d');wave(outputCanvas,output,'#cf9681');
 if($('consent').checked&&db>-6){if(!hotSince)hotSince=performance.now();if(performance.now()-hotSince>400){stop('High input level detected. Playback stopped. Lower your speaker volume before restarting.');return;}}else hotSince=0;
 frame=requestAnimationFrame(paint);
}
navigator.mediaDevices?.addEventListener('devicechange',()=>{if(ctx||starting)stop('Audio devices changed. Reconnect and restart.');});
window.addEventListener('pagehide',()=>{playbackRoutes?.stop();if(master)master.gain.value=0;streams.forEach(s=>s.getTracks().forEach(t=>t.stop()));ctx?.close();});
window.addEventListener('resize',()=>{if(!ctx)drawIdle();});drawIdle();

let tonePlaying=false;
$('testTone').onclick=async()=>{
 if(!ctx||starting){message('Start listening first, then test the selected output.');return;}
 if(tonePlaying)return;
 const audio=ctx;
 try{
  await audio.resume();
  if(ctx!==audio)return;
  const oscillator=audio.createOscillator(),gain=audio.createGain();
  oscillator.frequency.value=440;gain.gain.value=0;
  oscillator.connect(gain).connect(outputAnalyser);
  const now=audio.currentTime;
  gain.gain.setValueAtTime(0,now);
  gain.gain.linearRampToValueAtTime(.02,now+.03);
  gain.gain.setValueAtTime(.02,now+.35);
  gain.gain.linearRampToValueAtTime(0,now+.45);
  tonePlaying=true;
  oscillator.onended=()=>{tonePlaying=false;oscillator.disconnect();gain.disconnect();};
  oscillator.start(now);oscillator.stop(now+.5);
  message('Quiet test tone sent to the selected output. It bypasses the microphone playback mute.');
 }catch(e){tonePlaying=false;message('Output test failed: '+e.message);}
};


update();showPolarityTest();
if(window.QUIET_DESKTOP){document.querySelector('.tag').textContent='GO DESKTOP';}
