'use strict';

// Frequencies use a logarithmic axis so octaves occupy equal space.
const eqLimits={Frequency:[20,20000],GainDB:[-24,24],Q:[.2,12]};
const eqColors=['#c6ed98','#8ddcf3','#f3b287','#c7a8ff','#f0d97e','#f0a9cb','#91e1bd','#aabfff'];
const eqPlot={left:35,right:316,top:12,bottom:150};
const eqClamp=(value,[min,max])=>Math.min(max,Math.max(min,value));
const eqX=hz=>eqPlot.left+(eqPlot.right-eqPlot.left)*Math.log10(hz/20)/3;
const eqFrequency=x=>20*1000**eqClamp((x-eqPlot.left)/(eqPlot.right-eqPlot.left),[0,1]);
const eqY=db=>eqPlot.top+(eqPlot.bottom-eqPlot.top)*(24-eqClamp(db,eqLimits.GainDB))/48;
const eqGain=y=>eqClamp(24-48*(y-eqPlot.top)/(eqPlot.bottom-eqPlot.top),eqLimits.GainDB);

function equalizerBands(filter){
 return filter.EQBands??[
  {Frequency:120,GainDB:filter.BassDB??0,Q:1},
  {Frequency:1000,GainDB:filter.MidDB??0,Q:1},
  {Frequency:8000,GainDB:filter.TrebleDB??0,Q:1},
 ];
}

function eqResponse(band,hz){
 if(!band.GainDB)return 0;
 // Magnitude of the same peaking biquad used by the native engine.
 const center=2*Math.PI*band.Frequency/48000,w=2*Math.PI*hz/48000;
 const real=Math.cos(w)-Math.cos(center);
 const imaginary=Math.sin(center)*Math.sin(w)/(2*band.Q);
 const a=10**(band.GainDB/40);
 return 10*Math.log10((real**2+(imaginary*a)**2)/(real**2+(imaginary/a)**2));
}

function eqCombinedResponse(bands,hz){return bands.reduce((sum,band)=>sum+eqResponse(band,hz),0);}
function eqBandGainAtPoint(bands,index,hz,total){return eqClamp(total-eqCombinedResponse(bands.filter((_,i)=>i!==index),hz),eqLimits.GainDB);}

function renderEqualizer(filter){
 const editor=document.createElement('div');
 editor.className='eq-editor';
 editor.dataset.eqFilter=filter.ID;
 editor.innerHTML=`
  <svg class="eq-plot" viewBox="0 0 328 174" role="img" aria-label="Equalizer response by frequency">
   <g class="eq-grid"></g>
   <path class="eq-response"></path>
   <g class="eq-points"></g>
  </svg>
  <p class="eq-help">Click the chart to add a band; click a point to select it. Drag to shape it, wheel to change width, right-click a point to remove it. Points follow the combined response. Up to 8 bands.</p>
  <div class="eq-actions">
   <select aria-label="Select equalizer band"></select>
   <button type="button" class="secondary eq-remove">Remove</button>
  </div>
  <div class="eq-values">
   <label>Hz<input type="number" data-eq-field="Frequency" min="20" max="20000" step="1"></label>
   <label>Band gain (dB)<input type="number" data-eq-field="GainDB" min="-24" max="24" step=".5"></label>
   <label>Width (Q)<input type="number" data-eq-field="Q" min=".2" max="12" step=".1" title="Lower Q affects a wider frequency range"></label>
  </div>`;
 const svg=editor.querySelector('svg'),grid=editor.querySelector('.eq-grid');
 const points=editor.querySelector('.eq-points'),response=editor.querySelector('.eq-response');
 const select=editor.querySelector('select'),remove=editor.querySelector('.eq-remove');
 const inputs=[...editor.querySelectorAll('[data-eq-field]')];
 let selected=0,drag=null;
 const shape=(tag,attrs,parent)=>{
  const element=document.createElementNS('http://www.w3.org/2000/svg',tag);
  for(const [key,value] of Object.entries(attrs))element.setAttribute(key,value);
  parent.append(element);
  return element;
 };
 for(const hz of [20,100,1000,10000,20000]){
  const x=eqX(hz);
  shape('line',{x1:x,y1:eqPlot.top,x2:x,y2:eqPlot.bottom},grid);
  shape('text',{x,y:166,'text-anchor':'middle'},grid).textContent=hz>=1000?hz/1000+'k':hz;
 }
 for(const db of [24,12,0,-12,-24]){
  const y=eqY(db);
  shape('line',{x1:eqPlot.left,y1:y,x2:eqPlot.right,y2:y,class:db===0?'eq-zero':''},grid);
  shape('text',{x:30,y:y+4,'text-anchor':'end'},grid).textContent=(db>0?'+':'')+db;
 }
 const path=bands=>{
  const segments=[];
  for(let i=0;i<=240;i++){
   const x=eqPlot.left+(eqPlot.right-eqPlot.left)*i/240,hz=eqFrequency(x);
   const db=eqCombinedResponse(bands,hz);
   segments.push((i?'L':'M')+x.toFixed(2)+' '+eqY(db).toFixed(2));
  }
  return segments.join(' ');
 };
 function render(){
  const bands=equalizerBands(filter);
  selected=Math.max(0,Math.min(selected,bands.length-1));
  const current=bands[selected];
  select.replaceChildren(...(bands.length?bands.map((band,i)=>new Option('Band '+(i+1),i)): [new Option('No bands','')]));
  select.value=current?String(selected):'';
  select.disabled=remove.disabled=!current;
  for(const input of inputs){
   input.disabled=!current;
   const value=current?.[input.dataset.eqField];input.value=value==null?'':Number(value.toFixed(2));
  }
  response.setAttribute('d',path(bands));
  points.replaceChildren();
  bands.forEach((band,i)=>{
   const group=shape('g',{'data-band':i,transform:`translate(${eqX(band.Frequency)} ${eqY(eqCombinedResponse(bands,band.Frequency))})`},points);
   shape('circle',{r:i===selected?10:8,fill:eqColors[i],class:i===selected?'eq-active':''},group);
   shape('text',{'text-anchor':'middle',y:3.5},group).textContent=i+1;
   shape('title',{},group).textContent=`Band ${i+1} · ${band.Frequency} Hz · ${band.GainDB} dB · Q ${band.Q}`;
   if(i===selected)points.append(group);
  });
 }
 function apply(bands){
  filter.EQBands=bands;
  for(const other of document.querySelectorAll('[data-eq-filter]')){
   if(other.dataset.eqFilter===filter.ID)other.dispatchEvent(new Event('eq-change'));
  }
  configure(true);
 }
 function update(index,values){
  apply(equalizerBands(filter).map((band,i)=>i===index?{...band,...values}:band));
 }
 editor.addEventListener('eq-change',render);
 select.onchange=()=>{selected=Number(select.value);render();};
 remove.onclick=()=>apply(equalizerBands(filter).filter((_,i)=>i!==selected));
 for(const input of inputs){
  input.onchange=()=>{
   const value=input.valueAsNumber,field=input.dataset.eqField;
   if(Number.isFinite(value))update(selected,{[field]:eqClamp(value,eqLimits[field])});
   else render();
  };
 }
 svg.onpointerdown=event=>{
  if(event.button!==0||drag)return;
  const point=event.target.closest('[data-band]');
  const origin=new DOMPoint(event.clientX,event.clientY).matrixTransform(svg.getScreenCTM().inverse());
  if(!point&&(origin.x<eqPlot.left||origin.x>eqPlot.right||origin.y<eqPlot.top||origin.y>eqPlot.bottom))return;
  event.preventDefault();event.stopPropagation();
  if(point)selected=Number(point.dataset.band);
  else {const bands=equalizerBands(filter);if(bands.length>=eqColors.length)return;selected=bands.length;const Frequency=Math.round(eqFrequency(origin.x)),GainDB=eqBandGainAtPoint(bands,selected,Frequency,eqGain(origin.y));apply([...bands,{Frequency,GainDB,Q:1}]);}
  const band=equalizerBands(filter)[selected];if(!band)return;
  drag={id:event.pointerId,index:selected,dx:eqX(band.Frequency)-origin.x,dy:eqY(eqCombinedResponse(equalizerBands(filter),band.Frequency))-origin.y};
  svg.setPointerCapture(event.pointerId);render();
 };
 svg.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();const point=event.target.closest('[data-band]');if(!point)return;const index=Number(point.dataset.band);drag=null;selected=Math.max(0,index-1);apply(equalizerBands(filter).filter((_,i)=>i!==index));render();};
 svg.onpointermove=event=>{
  if(!drag||drag.id!==event.pointerId)return;
  const point=new DOMPoint(event.clientX,event.clientY).matrixTransform(svg.getScreenCTM().inverse());
  const Frequency=Math.round(eqFrequency(point.x+drag.dx));
  update(drag.index,{Frequency,GainDB:eqBandGainAtPoint(equalizerBands(filter),drag.index,Frequency,eqGain(point.y+drag.dy))});
 };
 svg.onpointerup=event=>{
  if(!drag||drag.id!==event.pointerId)return;
  drag=null;
  if(svg.hasPointerCapture(event.pointerId))svg.releasePointerCapture(event.pointerId);
 };
 svg.onpointercancel=()=>{drag=null;};
 svg.onlostpointercapture=()=>{drag=null;};
 svg.addEventListener('wheel',event=>{
  const bands=equalizerBands(filter);
  if(!bands[selected]||!event.deltaY)return;
  event.preventDefault();event.stopPropagation();
  const Q=eqClamp(bands[selected].Q*Math.exp(-Math.max(-100,Math.min(100,event.deltaY))*.005),eqLimits.Q);
  update(selected,{Q:Math.round(Q*100)/100});
 },{passive:false});
 render();
 return editor;
}
