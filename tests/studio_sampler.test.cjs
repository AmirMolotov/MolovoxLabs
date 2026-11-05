const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
function harness(){const elements=new Map();const h={library:{Boards:[]},location:{href:'http://127.0.0.1/token/studio.html'},URL,Map,DataView,ArrayBuffer,Float32Array,TextEncoder,performance,setTimeout,clearTimeout,
 $:id=>{if(!elements.has(id))elements.set(id,{value:'',textContent:'',classList:{add(){},remove(){}},hidden:false});return elements.get(id);},
 document:{querySelectorAll:()=>[]},window:{addEventListener(){}},nativeStopSound:async()=>{},};vm.createContext(h);vm.runInContext(fs.readFileSync(path.join(__dirname,'../dist/studio-sampler.js'),'utf8'),h);return h;}
test('sampler WAV contains only selected time range at native 48 kHz',()=>{
 const h=harness(),channel=new Float32Array(4800);channel.fill(.25,2400);
 const buffer={sampleRate:48000,numberOfChannels:1,getChannelData:()=>channel};const wav=h.samplerWAV(buffer,.05,.1),view=new DataView(wav);
 assert.equal(wav.byteLength,44+2400*2);assert.equal(view.getUint32(24,true),48000);assert.equal(view.getInt16(44,true),8192);assert.equal(view.getUint32(40,true),4800);
});
test('silent video pads get valid silent native WAV rather than browser audio',()=>{
 const h=harness(),wav=h.samplerWAV(null,0,.1),data=new DataView(wav);assert.equal(wav.byteLength,44+4800*2);for(let i=44;i<wav.byteLength;i+=2)assert.equal(data.getInt16(i,true),0);
});
test('crop remains inside source and sample ranges reject reversed or excessive duration',()=>{
 const h=harness(),crop=h.boundSamplerCrop({X:.9,Y:-.2,Width:.5,Height:2});assert.ok(crop.X+crop.Width<=1);assert.ok(crop.Y+crop.Height<=1);assert.ok(crop.Y>=0);
 vm.runInContext('samplerSource={ID:"source"};samplerDuration=500',h);h.$('samplerIn').value='5';h.$('samplerOut').value='4';assert.throws(()=>h.readSamplerRange());h.$('samplerOut').value='126';assert.throws(()=>h.readSamplerRange());h.$('samplerOut').value='6';assert.equal(h.readSamplerRange().Out,6);
});
test('recording worklet streams queued PCM in order',()=>{
 let Processor;const h={AudioWorkletProcessor:class{constructor(){this.port={};}},registerProcessor(name,klass){Processor=klass;},Float32Array};vm.createContext(h);vm.runInContext(fs.readFileSync(path.join(__dirname,'../dist/sampler-audio-worklet.js'),'utf8'),h);
 const p=new Processor(),output=new Float32Array(4);p.port.onmessage({data:{type:'chunk',samples:new Float32Array([.1,.2,.3])}});p.process([],[[output]]);assert.deepEqual(Array.from(output),[0,0,0,0]);
 p.port.onmessage({data:{type:'chunk',samples:new Float32Array([.4,.5])}});p.port.onmessage({data:{type:'start'}});p.process([],[[output]]);assert.ok(Math.abs(output[0]-.1)<1e-6);assert.ok(Math.abs(output[3]-.4)<1e-6);
 p.process([],[[output]]);assert.ok(Math.abs(output[0]-.5)<1e-6);assert.deepEqual(Array.from(output.slice(1)),[0,0,0]);
});
