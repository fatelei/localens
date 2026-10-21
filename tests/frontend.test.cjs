const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
async function boot(){
 const elements=new Map(),handlers=new Map(),opened=[];
 function element(id){if(elements.has(id))return elements.get(id);const e={value:'',hidden:true,checked:false,dataset:{},children:[],classList:{toggle(){}},addEventListener(name,fn){handlers.set(id+':'+name,fn);},replaceChildren(...children){this.children=children;},append(...children){this.children.push(...children);},setAttribute(){},getBoundingClientRect(){return {height:68};},focus(){document.activeElement=this;},select(){}};elements.set(id,e);return e;}
 const document={documentElement:{lang:"zh-CN"},getElementById:element,createElement:()=>element(Symbol()),querySelectorAll:()=>[],addEventListener:(name,fn)=>handlers.set(name,fn),activeElement:null};
 const status={roots:[],stats:{files:0,vectors:0},progress:{processed:0,skipped:0,warnings:[]},model:'off',python:'',scanning:false,embedding:false};
 const calls={GetStatus:async()=>status,SetPanelHeight:async()=>{},Search:async()=>({files:[]}),OpenFile:async p=>opened.push(p),HideLauncher:async()=>{}};
 calls.SearchRelated=async(query,kind)=>calls.Search(query,kind,true);
 calls.SearchKeywords=async(query,kind)=>calls.Search(query,kind,true);
 const context=vm.createContext({window:{go:{app:{App:calls}}},document,console,setTimeout:()=>1,clearTimeout(){},setInterval(){}});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/i18n.js'),'utf8'),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/icons.js'),'utf8'),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/evidence.js'),'utf8'),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/thumbnails.js'),'utf8'),context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/app.js'),'utf8'),context);
 await new Promise(resolve=>setImmediate(resolve));
 element('query').value='old';
 vm.runInContext("state.files=[{path:'/indexed/old.txt'}];state.selected=0;state.renderedQuery='old';state.renderedKind='all';",context);
 function key(overrides={}){return {key:'Enter',code:'Enter',isComposing:false,keyCode:13,metaKey:false,ctrlKey:false,preventDefault(){this.prevented=true;},...overrides};}
 return {context,element,handlers,opened,key,calls,status};
}
test('IME candidate confirmation never opens a search result',async()=>{
 const ui=await boot();ui.handlers.get('keydown')(ui.key({isComposing:true}));
 ui.handlers.get('keydown')(ui.key({keyCode:229}));
 ui.handlers.get('query:compositionstart')();ui.handlers.get('keydown')(ui.key());
 await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(ui.opened,[]);
});
test('Enter cannot open results belonging to an earlier query',async()=>{
 const ui=await boot();ui.element('query').value='new';ui.handlers.get('keydown')(ui.key());
 await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(ui.opened,[]);
});
test('Enter opens the selected file when query and results agree',async()=>{
 const ui=await boot();ui.handlers.get('keydown')(ui.key());
 await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(ui.opened,['/indexed/old.txt']);
});

test('English language updates placeholder and dynamic search text',async()=>{
 const ui=await boot();vm.runInContext("applyLanguage('en')",ui.context);
 assert.equal(vm.runInContext("t('modelReady')",ui.context),'Built-in model ready · Offline');
 assert.equal(vm.runInContext("t('results',{count:2})",ui.context),'2 results');
 assert.equal(ui.element('language').value,'en');
});

test('Settings markup does not expose interpreter paths',()=>{
 const html=fs.readFileSync(path.join(__dirname,'../frontend/index.html'),'utf8');
 assert.ok(html.includes('id="language"'));
 assert.ok(!html.includes('id="python"'));
 assert.ok(!html.includes('Python 解释器路径'));
});

test('Returning from settings preserves the results, selection and preview without restarting search',async()=>{
 const ui=await boot();let searches=0;ui.calls.Search=async()=>{searches++;return {files:[]};};
 ui.calls.GetEditors=async()=>[];
 vm.runInContext("state.files=[{path:'/indexed/first.txt'},{path:'/indexed/second.txt'}];state.selected=1;state.renderedContent=true;state.status.model='loading';state.refreshKey=JSON.stringify([state.status.stats.files,state.status.roots,state.status.model,state.status.scanning,state.status.embedding,state.status.stats.contentRevision])",ui.context);
 ui.element('preview').hidden=false;ui.element('results').scrollTop=127;
 vm.runInContext("showSettings();closeSettings()",ui.context);
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(vm.runInContext("state.files.length",ui.context),2);
 assert.equal(vm.runInContext("state.selected",ui.context),1);
 assert.equal(ui.element('preview').hidden,false);
 assert.equal(ui.element('results').scrollTop,127);
 assert.equal(searches,0,'opening settings alone must not trigger an empty filename search');
});

test('File icons safely handle names matching object prototype properties',async()=>{
 const ui=await boot();
 for(const name of ['constructor','foo.__proto__','report.pdf','page.html']){
  const icon=vm.runInContext(`fileIcon({name:${JSON.stringify(name)},kind:'document'})`,ui.context);
  assert.equal(icon.children.length,1);
 }
});

test('Typing a new phrase clears old results and rejects the late old response',async()=>{
 const ui=await boot();let resolve;
 ui.calls.Search=()=>new Promise(r=>resolve=r);
 ui.element('query').value='Out';ui.handlers.get('query:input')();
 const pending=vm.runInContext('search()',ui.context);
 ui.element('query').value='Out Of';ui.handlers.get('query:input')();
 assert.equal(vm.runInContext('state.files.length',ui.context),0);
 resolve({query:'Out',files:[{name:'output.py',path:'/output.py',kind:'document'}],filenameOnly:true});await pending;
 assert.equal(vm.runInContext('state.files.length',ui.context),0);
 ui.calls.Search=async()=>({query:'Out Of',files:[{name:'Out Of Context.mp4',path:'/movie.mp4',kind:'video'}],filenameOnly:true});
 await vm.runInContext('search()',ui.context);
 assert.equal(vm.runInContext('state.renderedQuery',ui.context),'Out Of');
 assert.equal(vm.runInContext('state.files[0].name',ui.context),'Out Of Context.mp4');
});

test('Index progress polling does not invalidate a pending search',async()=>{
 const ui=await boot();let resolve;
 ui.element('query').value='Out Of';ui.calls.Search=()=>new Promise(r=>resolve=r);
 const pending=vm.runInContext('search()',ui.context);
 const version=vm.runInContext('state.searchVersion',ui.context);
 ui.status.stats.files++;await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(vm.runInContext('state.searchVersion',ui.context),version);
 resolve({query:'Out Of',files:[{name:'Out Of Context.mp4',path:'/movie.mp4',kind:'video'}],filenameOnly:true});await pending;
 assert.equal(vm.runInContext('state.files.length',ui.context),1);
});

test('Body evidence shows the passage and highlights safe literal keywords',async()=>{
 const ui=await boot();
 const text=vm.runInContext(`fileEvidence({name:'notes.txt',path:'/notes.txt',match:'正文',snippet:'A story: Out Of context <script>unsafe</script>'},'Out Of')`,ui.context);
 assert.equal(text.children.length,3);
 const excerpt=text.children[1];assert.ok(excerpt.children.some(n=>n.textContent==='Out'));
 assert.ok(excerpt.children.some(n=>n.textContent==='Of'));
 assert.ok(excerpt.children.some(n=>n.textContent.includes('<script>')));
 assert.equal(excerpt.innerHTML,undefined);
});

test('Whitespace changes keep the same pending query without restarting it',async()=>{
 const ui=await boot();let resolve,calls=0;ui.calls.Search=()=>{calls++;return new Promise(r=>resolve=r);};
 ui.element('query').value='Out';const first=vm.runInContext('search()',ui.context);const version=vm.runInContext('state.searchVersion',ui.context);
 ui.element('query').value='Out ';ui.handlers.get('query:input')();
 assert.equal(vm.runInContext('state.searchVersion',ui.context),version);assert.equal(calls,1);
 resolve({query:'Out',files:[{name:'Out.mp4',path:'/Out.mp4',kind:'video',match:'文件名'}]});await first;
 assert.equal(vm.runInContext('state.files.length',ui.context),1);
});

test('Code content opens through selected editor with current evidence',async()=>{
 const ui=await boot();const opened=[];ui.calls.OpenCode=async(...args)=>opened.push(args);
 vm.runInContext("state.status.editor='code';state.files=[{path:'/indexed/sample.go',match:'正文',snippet:'func old() {}'}]",ui.context);
 ui.handlers.get('keydown')(ui.key());await new Promise(resolve=>setImmediate(resolve));
 assert.deepEqual(opened,[['/indexed/sample.go','old','func old() {}']]);assert.deepEqual(ui.opened,[]);
});
test('Code snippet presents line numbers without interpreting markup',async()=>{
 const ui=await boot();const result=vm.runInContext("fileEvidence({name:'sample.go',path:'/sample.go',match:'正文',snippet:'func search() {\\n  <script>\\n}',line:8,snippetLine:8},'search')",ui.context);
 assert.equal(result.children[1].textContent,'第 8 行');
 assert.equal(result.children[2].children[0].children[0].textContent,'8');
 assert.equal(result.children[2].children[1].children[0].textContent,'9');
});

test('Background index refresh preserves visible content hits during the next search',async()=>{
 const ui=await boot();let timer;ui.context.setTimeout=fn=>{timer=fn;return 2;};
 ui.element('query').value='while True';ui.status.model='ready';ui.status.semanticEnabled=true;
 ui.calls.Search=async(query,kind,content)=>({query,semanticEligible:true,files:content?[{path:'/indexed/sample.py',name:'sample.py',kind:'document',match:'正文',snippet:'while True:'}]:[]});
 await vm.runInContext('search()',ui.context);await timer();
 assert.equal(vm.runInContext('state.files.length',ui.context),1);
 ui.status.stats.files++;
 await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(vm.runInContext('state.files.length',ui.context),1,'a metadata refresh must not erase the content results');
 // A completed refresh may legitimately remove deleted/unmatched files.
 ui.calls.Search=async query=>({query,semanticEligible:true,files:[]});await timer();
 assert.equal(vm.runInContext('state.files.length',ui.context),0,'completed content response must replace old results');
});

test('Windows shortcut hints use Ctrl rather than Command',async()=>{
 const ui=await boot();ui.context.window.navigator={platform:'Win32'};
 assert.ok(vm.runInContext("t('keys')",ui.context).includes('Ctrl Y'));
 assert.ok(!vm.runInContext("t('keys')",ui.context).includes('⌘'));
});

test('Shortcut recorder captures a chord without triggering search actions',async()=>{
 const ui=await boot();ui.element('shortcut-edit').onclick();
 const e=ui.key({key:'p',code:'KeyP',ctrlKey:true,altKey:true});ui.handlers.get('keydown')(e);
 assert.equal(e.prevented,true);assert.equal(vm.runInContext('state.shortcutDraft',ui.context),'Ctrl+Alt+P');
 assert.equal(ui.element('shortcut-input').value,'Ctrl + Alt + P');assert.equal(ui.element('shortcut-save').disabled,false);
 assert.deepEqual(ui.opened,[]);
 ui.element('shortcut-edit').onclick();ui.handlers.get('keydown')(ui.key({key:'Escape',code:'Escape'}));
 assert.equal(vm.runInContext('state.shortcutRecording',ui.context),false);assert.equal(vm.runInContext('state.settings',ui.context),false);
});
test('Failed shortcut save retains draft and current binding',async()=>{
 const ui=await boot();ui.status.shortcut='Ctrl + Q';ui.calls.SetShortcut=async()=>{throw new Error('occupied')};
 ui.element('shortcut-edit').onclick();ui.handlers.get('keydown')(ui.key({key:'p',code:'KeyP',ctrlKey:true,altKey:true}));
 await ui.element('shortcut-save').onclick();
 assert.equal(ui.status.shortcut,'Ctrl + Q');assert.equal(vm.runInContext('state.shortcutDraft',ui.context),'Ctrl+Alt+P');assert.equal(ui.element('shortcut-save').disabled,false);
});

test('Returning resumes a paused content search without replacing existing evidence with empty filename hits',async()=>{
 const ui=await boot();let timer;ui.context.setTimeout=fn=>{timer=fn;return 2;};ui.calls.GetEditors=async()=>[];
 ui.element('query').value='while True';ui.status.model='ready';ui.status.semanticEnabled=true;
 vm.runInContext("state.files=[{path:'/existing.py',name:'existing.py',kind:'document',match:'正文',snippet:'while True:'}];state.renderedQuery='while True';state.renderedContent=true;state.semanticInFlight=123;",ui.context);
 ui.calls.Search=async(query,kind,content)=>({query,semanticEligible:true,files:content?[{path:'/new.py',name:'new.py',kind:'document',match:'正文',snippet:'while True:'}]:[]});
 vm.runInContext('showSettings();closeSettings()',ui.context);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(vm.runInContext('state.files[0].path',ui.context),'/existing.py');
 await timer();assert.equal(vm.runInContext('state.files[0].path',ui.context),'/new.py');
});
test('Returning after changing indexed folders refreshes the search scope',async()=>{
 const ui=await boot();ui.calls.GetEditors=async()=>[];let searches=0;
 ui.calls.Search=async()=>{searches++;return {files:[]};};
 vm.runInContext('showSettings()',ui.context);ui.status.roots=[{path:'/new-root',count:0}];
 vm.runInContext('closeSettings()',ui.context);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(searches,1);assert.equal(vm.runInContext('state.files.length',ui.context),0);
});

for(const scenario of [{semantic:false,model:'ready'},{semantic:true,model:'loading'}]){
 test(`Paused settings search applies final filename response: ${JSON.stringify(scenario)}`,async()=>{
  const ui=await boot();ui.calls.GetEditors=async()=>[];
  ui.status.model=scenario.model;ui.status.semanticEnabled=scenario.semantic;
  vm.runInContext(`state.semantic=${scenario.semantic};state.lexicalInFlight=123;`,ui.context);
  let searches=0;ui.calls.Search=async()=>{searches++;return {query:'old',semanticEligible:false,files:[]};};
  vm.runInContext('showSettings();closeSettings()',ui.context);await new Promise(resolve=>setImmediate(resolve));
  assert.equal(searches,1);assert.equal(vm.runInContext('state.files.length',ui.context),0);
  assert.equal(ui.element('search-spinner').hidden,true);
 });
}

test('Keyword content search runs without model inference; related results require a click',async()=>{
 const ui=await boot();let timer;ui.context.setTimeout=fn=>{timer=fn;return 2;};ui.status.model='ready';ui.status.semanticEnabled=true;
 ui.element('query').value='多词查询优先';const requests=[];
 ui.calls.Search=async(q,k,semantic)=>{requests.push(semantic);return {query:q,semanticEligible:true,files:semantic?[{path:'/extra.txt',name:'extra.txt',match:'语义',snippet:'different words'}]:[]};};
 ui.calls.SearchKeywords=async q=>({query:q,files:[{path:'/literal.txt',name:'literal.txt',match:'正文',snippet:q}]});
 await vm.runInContext('search()',ui.context);await timer();
 assert.deepEqual(requests,[false]);assert.equal(vm.runInContext('state.files.length',ui.context),1);
 assert.equal(ui.element('related-toggle').hidden,false);
 ui.element('related-toggle').onclick();await new Promise(resolve=>setImmediate(resolve));
 assert.deepEqual(requests,[false,true]);assert.equal(vm.runInContext('state.files.length',ui.context),2);
 assert.equal(vm.runInContext('state.files[0].path',ui.context),'/literal.txt');assert.equal(vm.runInContext('state.files[1].related',ui.context),true);
 ui.element('related-toggle').onclick();assert.equal(vm.runInContext('state.files.length',ui.context),1);
});
test('Literal body matches remain available when semantic search is off',async()=>{
 const ui=await boot();let timer;ui.context.setTimeout=fn=>{timer=fn;return 2;};ui.status.model='off';ui.status.semanticEnabled=false;
 ui.element('query').value='while True';ui.calls.Search=async q=>({query:q,semanticEligible:true,files:[]});
 ui.calls.SearchKeywords=async q=>({query:q,files:[{path:'/literal.py',name:'literal.py',match:'正文',snippet:q}]});
 await vm.runInContext('search()',ui.context);await timer();
 assert.equal(vm.runInContext('state.files.length',ui.context),1);assert.equal(ui.element('related-toggle').hidden,true);
});

test('Collapsing related results rejects a late model response',async()=>{
 const ui=await boot();let resolve;ui.status.model='ready';
 vm.runInContext("state.keywordFiles=[{path:'/literal.txt',name:'literal.txt'}];state.relatedEligible=true;state.renderedContent=true",ui.context);
 ui.calls.SearchRelated=()=>new Promise(r=>resolve=r);
 ui.element('related-toggle').onclick();ui.element('related-toggle').onclick();
 resolve({query:'old',files:[{path:'/extra.txt',name:'extra.txt',match:'语义'}]});await new Promise(r=>setImmediate(r));
 assert.equal(vm.runInContext('state.files.length',ui.context),1);assert.equal(vm.runInContext('state.files[0].path',ui.context),'/literal.txt');
});

test('Tab cycles file type filters and Shift Tab reverses while keeping query focus',async()=>{
 const ui=await boot();const searched=[];ui.calls.Search=async(q,kind)=>{searched.push(kind);return {files:[],semanticEligible:false};};
 for(const expected of ['document','image','video','audio','other','all']){
  const e=ui.key({key:'Tab',code:'Tab'});ui.handlers.get('keydown')(e);
  assert.equal(e.prevented,true);assert.equal(vm.runInContext('state.kind',ui.context),expected);
  assert.equal(ui.context.document.activeElement,ui.element('query'));await new Promise(r=>setImmediate(r));
 }
 const reverse=ui.key({key:'Tab',code:'Tab',shiftKey:true});ui.handlers.get('keydown')(reverse);await new Promise(r=>setImmediate(r));
 assert.equal(vm.runInContext('state.kind',ui.context),'other');assert.equal(reverse.prevented,true);
 assert.deepEqual(searched,['document','image','video','audio','other','all']);
});
test('Tab remains normal in settings, empty search, IME and modified shortcuts',async()=>{
 const ui=await boot();
 for(const setup of ["state.settings=true","state.settings=false;document.getElementById('query').value=''","document.getElementById('query').value='old';state.composing=true"]){
  vm.runInContext(setup,ui.context);const e=ui.key({key:'Tab',code:'Tab'});ui.handlers.get('keydown')(e);assert.ok(!e.prevented);
 }
 vm.runInContext('state.composing=false',ui.context);
 const e=ui.key({key:'Tab',code:'Tab',ctrlKey:true});ui.handlers.get('keydown')(e);assert.ok(!e.prevented);
});

test('Background refresh keeps the loading indicator hidden with existing results',async()=>{
 const ui=await boot();let resolve;ui.calls.Search=()=>new Promise(r=>resolve=r);
 ui.status.stats.files++;const pending=vm.runInContext('refreshStatus()',ui.context);await new Promise(r=>setImmediate(r));
 assert.equal(ui.element('search-spinner').hidden,true,'background search must be silent');
 resolve({query:'old',files:[{path:'/indexed/old.txt'}],semanticEligible:false});await pending;
 assert.equal(ui.element('search-spinner').hidden,true);
});
test('Vector progress alone does not repeat the keyword search',async()=>{
 const ui=await boot();ui.calls.Search=async()=>({files:[]});await vm.runInContext('refreshStatus()',ui.context);
 let calls=0;ui.calls.Search=async()=>{calls++;return {files:[]};};ui.status.stats.vectors++;
 await vm.runInContext('refreshStatus()',ui.context);assert.equal(calls,0);
});

test('Foreground searches show loading only until completion',async()=>{
 const ui=await boot();let resolve;ui.calls.Search=()=>new Promise(r=>resolve=r);
 const pending=vm.runInContext('search()',ui.context);assert.equal(ui.element('search-spinner').hidden,false);
 resolve({query:'old',files:[],semanticEligible:false});await pending;assert.equal(ui.element('search-spinner').hidden,true);
});

test('Spinner visibility uses an HTML container around the transparent SVG',()=>{
 const html=fs.readFileSync(path.join(__dirname,'../frontend/index.html'),'utf8');
 assert.ok(html.includes('<span id="search-spinner" class="spinner" hidden><svg'));
 assert.ok(!html.includes('<svg id="search-spinner"'));
});

test('Whitespace after a completed query does not clear results or show loading',async()=>{
 const ui=await boot();vm.runInContext("state.activeSearchKey=JSON.stringify(['old','all'])",ui.context);
 const version=vm.runInContext('state.searchVersion',ui.context);ui.element('query').value='old ';ui.handlers.get('query:input')();
 assert.equal(vm.runInContext('state.searchVersion',ui.context),version);assert.equal(vm.runInContext('state.files.length',ui.context),1);assert.equal(ui.element('search-spinner').hidden,true);
});

test('Returning to a completed type restores rows, selection and scroll without a search',async()=>{
 const ui=await boot();const requested=[];
 ui.calls.Search=async(q,kind)=>{requested.push(kind);return {query:q,semanticEligible:false,filenameOnly:true,files:kind==='all'?[{path:'/first.txt',name:'first.txt'},{path:'/second.txt',name:'second.txt'}]:[]};};
 await vm.runInContext('search()',ui.context);vm.runInContext('state.selected=1',ui.context);ui.element('results').scrollTop=83;
 vm.runInContext("changeKind('image')",ui.context);await new Promise(r=>setImmediate(r));
 vm.runInContext("changeKind('all')",ui.context);await new Promise(r=>setImmediate(r));
 assert.deepEqual(requested,['all','image']);assert.equal(vm.runInContext('state.selected',ui.context),1);assert.equal(ui.element('results').scrollTop,83);assert.equal(ui.element('search-spinner').hidden,true);
 vm.runInContext("changeKind('image')",ui.context);assert.equal(vm.runInContext('state.files.length',ui.context),0);assert.equal(requested.length,2,'empty results are also cached');
});
test('Cached tabs reject a late response from the category being left',async()=>{
 const ui=await boot();let resolve;
 ui.calls.Search=async q=>({query:q,semanticEligible:false,files:[{path:'/old.txt',name:'old.txt'}]});await vm.runInContext('search()',ui.context);
 ui.calls.Search=()=>new Promise(r=>resolve=r);vm.runInContext("changeKind('video')",ui.context);
 vm.runInContext("changeKind('all')",ui.context);resolve({query:'old',files:[{path:'/late.mp4',name:'late.mp4'}]});await new Promise(r=>setImmediate(r));
 assert.equal(vm.runInContext('state.kind',ui.context),'all');assert.equal(vm.runInContext('state.files[0].path',ui.context),'/old.txt');
});
test('Changing query or index completion invalidates cached categories',async()=>{
 const ui=await boot();ui.calls.Search=async q=>({query:q,semanticEligible:false,files:[]});await vm.runInContext('search()',ui.context);
 vm.runInContext("changeKind('image')",ui.context);await new Promise(r=>setImmediate(r));assert.equal(vm.runInContext('state.tabCache.size',ui.context),1);
 ui.element('query').value='different';ui.handlers.get('query:input')();assert.equal(vm.runInContext('state.tabCache.size',ui.context),0);
 await vm.runInContext('search()',ui.context);vm.runInContext("changeKind('all')",ui.context);await new Promise(r=>setImmediate(r));
 ui.status.lastScan='16:00:01';await vm.runInContext('refreshStatus()',ui.context);assert.equal(vm.runInContext('state.tabCache.size',ui.context),0);
});

test('An old response cannot be cached as a newer completed index revision',async()=>{
 const ui=await boot();let resolve;vm.runInContext("state.cacheIndexKey='previous'",ui.context);
 ui.calls.Search=()=>new Promise(r=>resolve=r);const pending=vm.runInContext('search()',ui.context);
 ui.status.lastScan='16:10:01';await vm.runInContext('refreshStatus()',ui.context);
 resolve({query:'old',semanticEligible:false,files:[{path:'/old.txt',name:'old.txt'}]});await pending;
 vm.runInContext('rememberTab()',ui.context);assert.equal(vm.runInContext('state.tabCache.size',ui.context),0);
});

test('Model becoming ready does not invalidate completed keyword tabs',async()=>{
 const ui=await boot();let requests=0;ui.calls.Search=async q=>{requests++;return {query:q,semanticEligible:false,files:[]};};
 await vm.runInContext('search()',ui.context);vm.runInContext("changeKind('image')",ui.context);await new Promise(r=>setImmediate(r));
 ui.status.model='ready';await vm.runInContext('refreshStatus()',ui.context);
 const before=requests;vm.runInContext("changeKind('all')",ui.context);await new Promise(r=>setImmediate(r));
 assert.equal(requests,before);
});

test('Image OCR keyword lookup does not require model eligibility',async()=>{
 const ui=await boot();let timer;ui.context.setTimeout=fn=>{timer=fn;return 2;};
 vm.runInContext("state.kind='image'",ui.context);ui.element('query').value='OCR';
 ui.calls.Search=async q=>({query:q,semanticEligible:false,contentEligible:true,files:[]});
 ui.calls.SearchKeywords=async q=>({query:q,files:[{path:'/capture.png',name:'capture.png',kind:'image',match:'正文',snippet:'LOCALENS OCR 4729'}]});
 await vm.runInContext('search()',ui.context);await timer();
 assert.equal(vm.runInContext('state.files[0].path',ui.context),'/capture.png');assert.equal(ui.element('related-toggle').hidden,true);
});

test('Image OCR is opt-in and its toggle is independent of semantic search',async()=>{
 const ui=await boot();ui.status.stats.ocrAvailable=true;ui.status.stats.ocrPending=7;ui.status.semanticEnabled=true;
 await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(ui.element('ocr').checked,false);assert.equal(ui.element('retry-ocr').disabled,true);
 assert.equal(ui.element('ocr-status').textContent,'图片文字识别已关闭');
 let choice;ui.calls.SetOCREnabled=async enabled=>{choice=enabled;ui.status.ocrEnabled=enabled};
 ui.element('ocr').checked=true;await ui.element('ocr').onchange();
 assert.equal(choice,true);assert.equal(ui.status.semanticEnabled,true);
 assert.match(ui.element('ocr-status').textContent,/剩余 7/);
 ui.status.ocrEnabled=false;ui.status.stats.ocrAvailable=false;await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(ui.element('ocr').disabled,true);
});

test('Late OCR status cannot undo a saved toggle',async()=>{
 const ui=await boot();ui.status.stats.ocrAvailable=true;
 await vm.runInContext('refreshStatus()',ui.context);
 let resolveStatus,resolveSave;
 const stale={...ui.status,ocrEnabled:false};
 ui.calls.GetStatus=()=>new Promise(r=>resolveStatus=r);
 const poll=vm.runInContext('refreshStatus()',ui.context);
 ui.calls.SetOCREnabled=()=>new Promise(r=>resolveSave=r);
 ui.element('ocr').checked=true;const save=ui.element('ocr').onchange();
 assert.equal(ui.element('ocr').disabled,true);
 resolveStatus(stale);await poll;
 assert.equal(ui.element('ocr').checked,true);assert.equal(ui.element('ocr').disabled,true);
 ui.status.ocrEnabled=true;ui.calls.GetStatus=async()=>ui.status;resolveSave();await save;
 assert.equal(ui.element('ocr').checked,true);assert.equal(ui.element('ocr').disabled,false);
});

test('Unsupported OCR visibly identifies the macOS-only limitation in both languages',async()=>{
 const ui=await boot();ui.status.stats.ocrAvailable=false;
 await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(ui.element('ocr-platform').hidden,false);
 assert.equal(ui.element('ocr').disabled,true);
 assert.match(ui.element('ocr-status').textContent,/仅支持 macOS/);
 assert.match(ui.element('ocr-hint').textContent,/Windows 和 Linux/);
 ui.status.language='en';await vm.runInContext('refreshStatus()',ui.context);
 assert.match(ui.element('ocr-status').textContent,/macOS only/);
 assert.match(ui.element('ocr-hint').textContent,/Windows or Linux/);
 ui.status.stats.ocrAvailable=true;await vm.runInContext('refreshStatus()',ui.context);
 assert.equal(ui.element('ocr-platform').hidden,true);
 assert.equal(ui.element('ocr').disabled,false);
 assert.match(ui.element('ocr-hint').textContent,/Enable to recognize/);
});
