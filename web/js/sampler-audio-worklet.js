'use strict';
class QuietSamplerAudio extends AudioWorkletProcessor {
 constructor(){super();this.chunks=[];this.offset=0;this.playing=false;this.port.onmessage=event=>{if(event.data.type==='chunk')this.chunks.push(event.data.samples);if(event.data.type==='start')this.playing=true;};}
 process(inputs,outputs){const out=outputs[0]?.[0];if(!out)return true;out.fill(0);if(!this.playing)return true;
  for(let i=0;i<out.length;i++){while(this.chunks.length&&this.offset>=this.chunks[0].length){this.chunks.shift();this.offset=0;}if(this.chunks.length)out[i]=this.chunks[0][this.offset++];}
  return true;
 }
}
registerProcessor('quiet-sampler-audio',QuietSamplerAudio);
