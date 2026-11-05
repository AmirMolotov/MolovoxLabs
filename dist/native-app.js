'use strict';
const $=id=>document.getElementById(id);
let active=false,busy=false,polling=false,pollTimer=null;
const message=text=>$('status').textContent=text;
document.querySelector('.tag').textContent='NATIVE GO · WASAPI';
$('aec').checked=false;$('aec').disabled=true;
$('aec').parentElement.lastChild.textContent=' Echo cancellation unavailable in native mode';
$('aec').parentElement.nextElementSibling.textContent='Native capture does not implement acoustic echo cancellation. Inversion alone does not remove speaker feedback.';
$('sinkNote').textContent='Connect devices to select native Windows playback endpoints. Multiple devices have independent clocks.';
function options(){return {Volume:Number($('volume').value)/100,Enabled:$('consent').checked,Inverted:$('invert').checked,DelayMS:Number($('delay').value),Filter:$('bassFilter').checked};}
function controls(running){
 for(const el of document.querySelectorAll('#inputs input,#sinks input,#refresh'))el.disabled=running;
 $('start').disabled=running;$('stop').disabled=!running;
}
function wave(id,samples,color){
 const canvas=$(id),pen=canvas.getContext('2d'),ratio=window.devicePixelRatio||1;
 const w=canvas.width=Math.max(1,Math.round(canvas.clientWidth*ratio)),h=canvas.height=Math.max(1,Math.round(canvas.clientHeight*ratio));
 pen.clearRect(0,0,w,h);pen.strokeStyle='#ffffff09';pen.lineWidth=1;
 for(let x=0;x<w;x+=w/12){pen.beginPath();pen.moveTo(x,0);pen.lineTo(x,h);pen.stroke();}
 for(let y=0;y<=h;y+=h/4){pen.beginPath();pen.moveTo(0,y);pen.lineTo(w,y);pen.stroke();}
 pen.strokeStyle=color;pen.lineWidth=1.5*ratio;pen.beginPath();
 if(samples?.length){samples.forEach((v,i)=>{const x=i/(samples.length-1)*w,y=h/2+v*h*.45;i?pen.lineTo(x,y):pen.moveTo(x,y);});}
 else{pen.moveTo(0,h/2);pen.lineTo(w,h/2);}
 pen.stroke();
}
function idle(){wave('micScope',null,'#b9e78d55');wave('outputScope',null,'#cf968155');}
async function connect(){
 $('refresh').disabled=true;
 try{
  const list=await nativeDevices();
  const previousInputs=new Map([...document.querySelectorAll('.mic')].map(row=>[row.dataset.id,{checked:row.querySelector('[type=checkbox]').checked,gain:row.querySelector('[type=range]').value}]));
  const previousOutputs=new Set([...document.querySelectorAll('#sinks input:checked')].map(el=>el.value));
  $('inputs').replaceChildren();$('inputs').className='';$('sinks').replaceChildren();
  for(const d of list.Inputs){
   const row=document.createElement('div');row.className='mic';row.dataset.id=d.ID;
   const label=document.createElement('label'),box=document.createElement('input');
   box.type='checkbox';box.checked=previousInputs.get(d.ID)?.checked??d.Default;
   label.append(box,document.createTextNode(d.Name));
   const gain=document.createElement('input');gain.type='range';gain.min=0;gain.max=100;gain.value=previousInputs.get(d.ID)?.gain??100;gain.setAttribute('aria-label',d.Name+' mix level');
   const value=document.createElement('output');value.textContent=gain.value+'%';gain.oninput=()=>value.textContent=gain.value+'%';
   row.append(label,gain,value);$('inputs').append(row);
  }
  for(const d of list.Outputs){
   const label=document.createElement('label');label.className='switch';
   const box=document.createElement('input');box.type='checkbox';box.value=d.ID;box.checked=previousOutputs.size?previousOutputs.has(d.ID):d.Default;
   label.append(box,document.createTextNode(d.Name));$('sinks').append(label);
  }
  if(!list.Inputs.length)$('inputs').textContent='No native capture devices found.';
  if(!list.Outputs.length)$('sinks').textContent='No native output devices found.';
  message('Native Windows devices connected. Select inputs and outputs, then start.');
 }catch(e){message('Device discovery failed: '+e.message);}
 finally{$('refresh').disabled=false;}
}
async function configure(){
 $('volumeValue').textContent=$('volume').value+'%';$('delayValue').textContent=$('delay').value+' ms';
 $('polarityState').textContent=$('invert').checked?'Native DSP: inverted polarity (× −1)':'Native DSP: normal polarity (× +1)';
 try{await nativeConfigure(options());if(active)message($('consent').checked?'Native playback enabled.':'Listening in Go. Output muted until enabled.');}
 catch(e){message('Could not apply controls: '+e.message);}
}
['volume','delay','invert','consent','bassFilter'].forEach(id=>$(id).oninput=configure);
async function stop(reason='Stopped. Native devices released.'){
 if(busy&&!active)return;
 busy=true;
 try{await nativeStop();}
 catch(e){reason='Stop error: '+e.message;}
 finally{
 active=false;busy=false;clearInterval(pollTimer);pollTimer=null;controls(false);$('consent').checked=false;
 $('badge').textContent='IDLE';$('count').textContent='0';$('level').innerHTML='— <em>dBFS</em>';$('outputLevel').textContent='— dBFS';$('details').textContent='';message(reason);idle();
 }
}
async function poll(){
 if(!active||polling)return;polling=true;
 try{
  const s=await nativeSnapshot();
  if(!active)return;
  if(s.Fault){await stop('Native audio device stopped or disconnected. Reconnect devices before restarting.');return;}
  wave('micScope',s.Mic,'#b9e78d');wave('outputScope',s.Output,'#cf9681');
  $('level').innerHTML=s.InputDB.toFixed(1)+' <em>dBFS</em>';$('outputLevel').textContent=s.OutputDB.toFixed(1)+' dBFS';
  $('details').textContent=s.Backend+' · '+s.Inputs+' inputs · '+s.Outputs+' outputs · buffer underruns: '+s.Underruns+' · clipped samples: '+s.Clipped+'. Requested device period 5 ms plus 20 ms bridge; total hardware latency is not measured.';
 }catch(e){await stop('Native engine error: '+e.message);}
 finally{polling=false;}
}
$('refresh').onclick=connect;
$('start').onclick=async()=>{
 if(active||busy)return;
 const Inputs=[...document.querySelectorAll('.mic')].filter(row=>row.querySelector('[type=checkbox]').checked).map(row=>({ID:row.dataset.id,Gain:Number(row.querySelector('[type=range]').value)/100}));
 const Outputs=[...document.querySelectorAll('#sinks input:checked')].map(el=>el.value);
 if(!Inputs.length||!Outputs.length){message('Select at least one microphone and one output.');return;}
 busy=true;controls(true);
 try{
  await nativeStart({Inputs,Outputs,Options:options()});
  active=true;busy=false;$('badge').textContent='NATIVE LIVE';$('count').textContent=Inputs.length;
  $('latency').innerHTML='— <em>ms</em>';
  await configure();pollTimer=setInterval(poll,66);await poll();
 }catch(e){busy=false;await stop('Could not start native audio: '+e.message);}
};
$('stop').onclick=()=>stop();
$('testTone').onclick=async()=>{try{await nativeTone();message('Go generated a quiet half-second test tone on all selected outputs. This bypasses the mic playback mute.');}catch(e){message(e.message);}};
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!busy)stop();});
window.addEventListener('resize',()=>{if(!active)idle();});
window.addEventListener('pagehide',()=>{nativeStop().catch(()=>{});});
idle();configure();
nativeSelfTest().then(result=>{$('polarityTest').textContent=result.passed?'Native Go inversion test passed: output = −input (4,096 samples).':'Native Go inversion test FAILED.';}).catch(e=>{$('polarityTest').textContent='Native test error: '+e.message;});
