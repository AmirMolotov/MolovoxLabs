const {test}=require('node:test');
const assert=require('node:assert/strict');
const {openPlaybackRoutes}=require('../web/js/routing.js');
test('output routing: direct, multiple, failure and cancellation',async()=>{
 const players=[];let stoppedTracks=0;
 global.HTMLMediaElement=class {setSinkId(){}};
 global.Audio=class {
  constructor(){players.push(this);this.paused=false;}
  async setSinkId(id){this.id=id;if(id==='bad')throw Error('Device denied');}
  async play(){this.played=true;}
  pause(){this.paused=true;}
 };
 const connections=new Set();
 const source={connect:n=>connections.add(n),disconnect:n=>connections.delete(n)};
 const context={destination:{},async setSinkId(id){this.sinkId=id;},createMediaStreamDestination:()=>({stream:{getTracks:()=>[{stop(){stoppedTracks++;}}]}})};
 let route=await openPlaybackRoutes(context,source,[{id:'headphones'}]);
 assert.equal(route.mode,'direct');assert.equal(context.sinkId,'headphones');assert.ok(connections.has(context.destination));
 route.stop();route.stop();assert.equal(connections.size,0);
 route=await openPlaybackRoutes(context,source,[{id:'speaker'},{id:'headphones'}]);
 assert.equal(route.count,2);assert.deepEqual(players.map(p=>p.id),['speaker','headphones']);
 assert.ok(players.every(p=>p.played));assert.equal(players[0].srcObject,players[1].srcObject);
 route.stop();assert.ok(players.every(p=>p.paused&&p.srcObject===null));assert.equal(stoppedTracks,1);assert.equal(connections.size,0);
 await assert.rejects(openPlaybackRoutes(context,source,[{id:'speaker'},{id:'bad'}]),/denied/);
 assert.ok(players.every(p=>p.paused));assert.equal(connections.size,0);assert.equal(stoppedTracks,2);
 let valid=true;
 global.Audio=class {async setSinkId(){valid=false;}pause(){this.paused=true;}async play(){throw Error('Should not play');}};
 await assert.rejects(openPlaybackRoutes(context,source,[{id:'a'},{id:'b'}],()=>valid),/cancelled/);
 assert.equal(connections.size,0);assert.equal(stoppedTracks,3);
 await assert.rejects(openPlaybackRoutes(context,source,[]),/Select at least/);
 await assert.rejects(openPlaybackRoutes(context,source,[{id:'a'},{id:'a'}]),/only once/);
});
