const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');

test('export picker sends the new name, whole groups and individually selected pads',async()=>{
 class Element{
  constructor(){this.children=[];this.dataset={};this.value='';this.disabled=false;}
  append(...children){this.children.push(...children);}
  replaceChildren(){this.children=[];}
  querySelectorAll(){return this.children.flatMap(child=>child.type==='checkbox'?[child]:child.querySelectorAll?.()||[]);}
  addEventListener(){} showModal(){this.open=true;} close(){this.open=false;} select(){}
 }
 const elements=new Map(),$=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};
 const board={ID:'board',Name:'Original',Groups:[{ID:'g',Name:'Group'},{ID:'h',Name:'Other'}],Sounds:[{ID:'a',Name:'A',GroupID:'g'},{ID:'b',Name:'B',GroupID:'h'},{ID:'c',Name:'C'}]};
 let exported;
 const h={$,currentSoundboard:()=>board,document:{querySelector:()=>({}),createElement:()=>new Element(),createTextNode:text=>text},withLoading:async(_,run)=>run(),nativeExportBoard:async(id,pick)=>{exported={id,pick};return 'chosen.qlboard';},errorText:String};
 vm.createContext(h);vm.runInContext(fs.readFileSync('web/js/studio-board-export.js','utf8'),h);
 $('exportBoard').onclick();
 assert.equal($('exportBoardName').value,'Original');
 const boxes=Object.fromEntries($('exportBoardChoices').querySelectorAll().map(box=>[box.value,box]));
 assert.equal(boxes.a.disabled,true);
 boxes.h.checked=false;$('exportBoardChoices').onchange();assert.equal(boxes.b.disabled,false);boxes.b.checked=true;
 $('exportBoardName').value='Chosen';$('exportBoardName').oninput();await $('saveBoardExport').onclick();
 assert.deepEqual(JSON.parse(JSON.stringify(exported)),{id:'board',pick:{Name:'Chosen',Groups:['g'],Sounds:['b','c']}});
 assert.equal($('exportBoardDialog').open,false);
});
