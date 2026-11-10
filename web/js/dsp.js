'use strict';
function createPolarityStage(context, inverted=true){
 const stage=context.createGain();
 stage.gain.value=inverted?-1:1;
 return stage;
}
async function verifyPolarity(){
 const length=4096,rate=48000;
 const context=new OfflineAudioContext(1,length,rate);
 const input=context.createBuffer(1,length,rate),samples=input.getChannelData(0);
 for(let i=0;i<length;i++)samples[i]=.3*Math.sin(i*.13)+.15*Math.cos(i*.043);
 const source=context.createBufferSource();source.buffer=input;
 source.connect(createPolarityStage(context,true)).connect(context.destination);source.start();
 const rendered=await context.startRendering(),output=rendered.getChannelData(0);
 let maxError=0,energy=0;
 for(let i=0;i<length;i++){maxError=Math.max(maxError,Math.abs(output[i]+samples[i]));energy+=output[i]*output[i];}
 return {passed:maxError<1e-6&&energy>1,maxError,samples:length,sampleRate:rate};
}
async function showPolarityTest(){
 const label=document.getElementById('polarityTest');
 try{
  const result=await verifyPolarity();
  label.textContent=result.passed?'Inversion self-test passed: output = −input (4,096 samples).':'Inversion self-test FAILED.';
  if(window.reportPolarityTest)await window.reportPolarityTest(result);
 }catch(e){
  label.textContent='Inversion self-test unavailable: '+e.message;
  if(window.reportPolarityTest)await window.reportPolarityTest({passed:false,error:e.message});
 }
}
