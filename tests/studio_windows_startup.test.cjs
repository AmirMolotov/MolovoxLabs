const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('web/js/studio.js','utf8');
function setup(){
 const controls={windowsStartup:{checked:false,disabled:true},saveStatus:{}};
 const h={$:id=>controls[id],errorText:e=>e.message,nativeLoadWindowsStartup:async()=>true,nativeSaveWindowsStartup:async enabled=>{h.saved=enabled;}};
 vm.createContext(h);vm.runInContext(source.slice(source.indexOf("$('windowsStartup').onchange"),source.indexOf('function wave(')),h);
 return {h,control:controls.windowsStartup,status:controls.saveStatus};
}
test('Windows startup reads registration and saves both toggle states',async()=>{
 const {h,control}=setup();await h.initWindowsStartup();assert.equal(control.checked,true);assert.equal(control.disabled,false);
 for(const enabled of [false,true]){control.checked=enabled;const pending=control.onchange();assert.equal(control.disabled,true);await pending;assert.equal(h.saved,enabled);assert.equal(control.checked,enabled);assert.equal(control.disabled,false);}
});
test('Windows startup failures preserve the previous state and announce the error',async()=>{
 const {h,control,status}=setup();h.nativeLoadWindowsStartup=async()=>{throw Error('Access denied');};await h.initWindowsStartup();assert.equal(control.disabled,true);assert.match(status.textContent,/Access denied/);
 control.disabled=false;control.checked=true;h.nativeSaveWindowsStartup=async()=>{throw Error('Access denied');};await control.onchange();assert.equal(control.checked,false);assert.equal(control.disabled,false);assert.match(status.textContent,/Access denied/);
});
